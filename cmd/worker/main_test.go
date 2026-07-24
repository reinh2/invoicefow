package main

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/reinhlord/invoiceflow/internal/invoices"
)

// fixtureFiles maps each committed fictional document to the SHA-256 the offline
// extractor is keyed to. Regenerating the files (scripts/gen-fixtures.py) changes
// the bytes; this test then fails and prints the new digest to paste into
// defaultFakeFixtures.
var fixtureFiles = map[string]string{
	"fixture-aurora-stationery.pdf":  "f76e1b0c0a972a83d57528f1ca0810d94d633bfe812899ae0c093d3d9d94ec99",
	"fixture-meridian-supplies.png":  "f4f911b595ba897de5f4c1d8dd969f9eda53c1f1522ccb45219a03350a975ffd",
	"fixture-cedarline-services.pdf": "6767ba11afd4b7e926196d268491c761e08128c1eed3c9a349dfe4b77a5dd945",
	"stage2-fictional-compose.pdf":   "86ab48c217acdd9f083e8f2d24fc8f547ec8c80a10cd958121a79ffb3f229e99",
}

// expectedWarnings is the exact server warning-code set the real normalizer must
// produce for each fixture's configured proposal, verified rather than asserted
// by hand.
var expectedWarnings = map[string][]string{
	"OFFICE-001":    {},
	"AURORA-1042":   {},
	"MERIDIAN-2087": {},
	"CEDAR-3390":    {"subtotal_tax_total_mismatch"},
}

func TestDefaultFakeFixturesMatchCommittedFiles(t *testing.T) {
	registered := make(map[string]bool)
	for _, fixture := range defaultFakeFixtures() {
		registered[fixture.DocumentSHA256] = true
	}
	for name, want := range fixtureFiles {
		path := filepath.Join("..", "..", "testdata", name)
		bytes, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", name, err)
		}
		got := hex.EncodeToString(sha256Sum(bytes))
		if got != want {
			t.Fatalf("%s: sha256 = %s, want %s (regenerated? update fixtureFiles and defaultFakeFixtures)", name, got, want)
		}
		if !registered[want] {
			t.Fatalf("%s: sha256 %s is not registered in defaultFakeFixtures", name, want)
		}
	}
}

func TestDefaultFakeFixturesNormalizeToExpectedWarnings(t *testing.T) {
	seen := make(map[string]bool)
	for _, fixture := range defaultFakeFixtures() {
		number := deref(fixture.Proposal.InvoiceNumber)
		want, ok := expectedWarnings[number]
		if !ok {
			t.Fatalf("fixture %q has no expected-warning entry", number)
		}
		seen[number] = true
		_, warnings := invoices.NormalizeProposal(fixture.Proposal)
		got := warningCodes(warnings)
		if !equalStringSets(got, want) {
			t.Fatalf("%s: warnings = %v, want %v", number, got, want)
		}
	}
	for number := range expectedWarnings {
		if !seen[number] {
			t.Fatalf("expected fixture %q was not registered", number)
		}
	}
}

func sha256Sum(b []byte) []byte {
	sum := sha256.Sum256(b)
	return sum[:]
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func warningCodes(warnings []invoices.Warning) []string {
	codes := make([]string, 0, len(warnings))
	for _, w := range warnings {
		codes = append(codes, w.Code)
	}
	return codes
}

func equalStringSets(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	ac := append([]string(nil), a...)
	bc := append([]string(nil), b...)
	sort.Strings(ac)
	sort.Strings(bc)
	for i := range ac {
		if ac[i] != bc[i] {
			return false
		}
	}
	return true
}
