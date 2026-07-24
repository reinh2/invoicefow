package processing

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/reinhlord/invoiceflow/internal/export"
	"github.com/reinhlord/invoiceflow/internal/extraction"
	"github.com/reinhlord/invoiceflow/internal/invoices"
)

func TestWebhookPayloadUsesPersistedIdempotencyKeyAndStableBytes(t *testing.T) {
	details := ExportJobDetails{
		DocumentID: "doc-1", VersionNumber: 2, ApprovedAt: time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC),
		IdempotencyKey: "webhook_export:doc-1:version-2", Normalized: invoices.NormalizedProposal{Currency: "JPY"},
	}
	payload := buildWebhookPayload(details)
	if payload.IdempotencyKey != details.IdempotencyKey {
		t.Fatalf("payload key=%q, persisted key=%q", payload.IdempotencyKey, details.IdempotencyKey)
	}
	first, err := export.CanonicalPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := export.CanonicalPayload(buildWebhookPayload(details))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("retry payload bytes differ: %q vs %q", first, second)
	}
}

// A provider that asserts no evidence (ADR-014) leaves nil slices. The snapshot
// columns are constrained to JSON arrays, so nil must encode as `[]`, never
// `null`.
func TestSnapshotArraysEncodeEmptyRatherThanNull(t *testing.T) {
	for name, encoded := range map[string][]byte{
		"evidence":    mustMarshal(t, jsonArray[extraction.Evidence](nil)),
		"diagnostics": mustMarshal(t, jsonArray[extraction.Diagnostic](nil)),
		"warnings":    mustMarshal(t, jsonArray[invoices.Warning](nil)),
	} {
		if string(encoded) != "[]" {
			t.Fatalf("%s encoded as %q, want []", name, encoded)
		}
	}
}

func mustMarshal(t *testing.T, value any) []byte {
	t.Helper()
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return encoded
}

// The provider-error hook must stay narrow: only errors that are secret-free by
// construction reach the operator log.
func TestReportProviderErrorOnlyForwardsProviderErrors(t *testing.T) {
	var forwarded []string
	worker := Worker{OnProviderError: func(_ string, err error) { forwarded = append(forwarded, err.Error()) }}

	worker.reportProviderError("doc-1", fmt.Errorf("%w: provider status 429", extraction.ErrOpenAIRequest))
	worker.reportProviderError("doc-1", extraction.ErrMalformedPDF)
	worker.reportProviderError("doc-1", errors.New("/private/storage/tmp/abc: permission denied"))

	if len(forwarded) != 1 || !strings.Contains(forwarded[0], "provider status 429") {
		t.Fatalf("forwarded=%v, want only the provider error", forwarded)
	}
}
