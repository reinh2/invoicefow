package processing

import (
	"encoding/base64"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/reinhlord/invoiceflow/internal/extraction"
)

func TestDecodeHumanReviewInputRejectsAuthorityAndNormalizesCandidates(t *testing.T) {
	input, err := DecodeHumanReviewInput([]byte(`{"supplier_name":"  Fictional Vendor ","currency":" usd ","total":"24.005","line_items":[{"description":"Service","quantity":"1","unit_price":"24.005","tax_amount":"","total":"24.00"}]}`))
	if err != nil {
		t.Fatalf("DecodeHumanReviewInput() error = %v", err)
	}
	proposal := input.proposal()
	if proposal.Currency == nil || *proposal.Currency != " usd " || len(proposal.LineItems) != 1 {
		t.Fatalf("proposal = %+v", proposal)
	}
	if _, err := DecodeHumanReviewInput([]byte(`{"currency":"USD","status":"approved"}`)); err != ErrInvalidHumanReviewEdit {
		t.Fatalf("unknown authority field error = %v, want ErrInvalidHumanReviewEdit", err)
	}
	if _, err := DecodeHumanReviewInput([]byte(`{"currency":1}`)); err != ErrInvalidHumanReviewEdit {
		t.Fatalf("numeric candidate error = %v, want ErrInvalidHumanReviewEdit", err)
	}
}

func TestEditableFromNormalizedUsesExactMoneyStrings(t *testing.T) {
	raw := []byte(`{"rounding_policy_version":"money-v1","currency":"USD","total_minor":2400,"line_items":[{"quantity":"2","unit_price_minor":1000,"total_minor":2000}]}`)
	editable, err := editableFromNormalized(raw)
	if err != nil {
		t.Fatal(err)
	}
	if editable.Total != "24.00" || editable.LineItems[0].UnitPrice != "10.00" {
		t.Fatalf("editable = %+v", editable)
	}
	if _, err := editableFromNormalized([]byte(`not JSON`)); err == nil || !strings.Contains(err.Error(), "invalid character") {
		t.Fatalf("bad JSON error = %v", err)
	}
}

// The page size is the only list parameter a client controls, so both ends of
// the clamp matter: an unspecified size must not become zero rows, and a huge
// size must not become an unbounded scan.
func TestClampDocumentPageSizeBoundsBothEnds(t *testing.T) {
	for _, tc := range []struct{ in, want int }{
		{0, defaultDocumentPageSize},
		{-1, defaultDocumentPageSize},
		{1, 1},
		{maxDocumentPageSize, maxDocumentPageSize},
		{maxDocumentPageSize + 1, maxDocumentPageSize},
		{1 << 30, maxDocumentPageSize},
	} {
		if got := clampDocumentPageSize(tc.in); got != tc.want {
			t.Fatalf("clampDocumentPageSize(%d) = %d, want %d", tc.in, got, tc.want)
		}
	}
}

// A cursor must survive a round trip exactly, and anything the server did not
// issue must be a client error rather than a silent reset to the first page —
// which would loop a paging client forever.
func TestDocumentCursorRoundTripAndRejection(t *testing.T) {
	issued := time.Date(2026, 7, 25, 10, 30, 0, 123456789, time.UTC)
	id := "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
	encoded := encodeDocumentCursor(issued, id)

	gotTime, gotID, err := decodeDocumentCursor(encoded)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if gotID == nil || *gotID != id || gotTime == nil || !gotTime.Equal(issued) {
		t.Fatalf("round trip lost data: %v %v", gotTime, gotID)
	}

	if gotTime, gotID, err := decodeDocumentCursor(""); err != nil || gotTime != nil || gotID != nil {
		t.Fatalf("empty cursor must mean first page, got %v %v %v", gotTime, gotID, err)
	}

	for name, cursor := range map[string]string{
		"not base64":       "!!!!",
		"no separator":     base64.RawURLEncoding.EncodeToString([]byte("2026-07-25T10:30:00Z")),
		"bad uuid":         base64.RawURLEncoding.EncodeToString([]byte("2026-07-25T10:30:00Z|../../etc")),
		"uppercase uuid":   base64.RawURLEncoding.EncodeToString([]byte("2026-07-25T10:30:00Z|3F2504E0-4F89-11D3-9A0C-0305E82C3301")),
		"bad timestamp":    base64.RawURLEncoding.EncodeToString([]byte("yesterday|" + id)),
		"sql in timestamp": base64.RawURLEncoding.EncodeToString([]byte("' OR 1=1--|" + id)),
	} {
		if _, _, err := decodeDocumentCursor(cursor); !errors.Is(err, ErrInvalidCursor) {
			t.Fatalf("%s: err = %v, want ErrInvalidCursor", name, err)
		}
	}
}

func TestValidUUIDAcceptsOnlyCanonicalLowercaseForm(t *testing.T) {
	if !validUUID("3f2504e0-4f89-11d3-9a0c-0305e82c3301") {
		t.Fatal("canonical uuid rejected")
	}
	for _, value := range []string{
		"", "3f2504e0-4f89-11d3-9a0c-0305e82c330", "3f2504e0-4f89-11d3-9a0c-0305e82c33011",
		"3F2504E0-4F89-11D3-9A0C-0305E82C3301", "3f2504e04f8911d39a0c0305e82c3301xxxx",
		"3f2504e0_4f89-11d3-9a0c-0305e82c3301", "3f2504e0-4f89-11d3-9a0c-0305e82c330g",
	} {
		if validUUID(value) {
			t.Fatalf("accepted %q", value)
		}
	}
}

// Every review-mutating action is allowed only from needs_review. A document
// that was already approved, exported, rejected, or failed must not be editable.
func TestRequireNeedsReviewRejectsEveryOtherState(t *testing.T) {
	if err := requireNeedsReview("needs_review"); err != nil {
		t.Fatalf("needs_review rejected: %v", err)
	}
	for _, status := range []string{"uploaded", "queued", "processing", "approved", "rejected", "exported", "failed", ""} {
		if err := requireNeedsReview(status); !errors.Is(err, ErrInvalidDocumentState) {
			t.Fatalf("%q: err = %v, want ErrInvalidDocumentState", status, err)
		}
	}
}

// Export must target the exact approved version. A state without the immutable
// reference is not exportable even when the status looks right — that pairing is
// the invariant, not the status alone.
func TestRequireExportableVersionNeedsBothStatusAndApprovedVersion(t *testing.T) {
	approved := "9c3f0a2e-4f89-11d3-9a0c-0305e82c3301"
	for _, status := range []string{"approved", "exported"} {
		got, err := requireExportableVersion(status, &approved)
		if err != nil || got != approved {
			t.Fatalf("%s: got %q err %v", status, got, err)
		}
	}
	empty := ""
	for name, tc := range map[string]struct {
		status  string
		version *string
	}{
		"approved but no version": {"approved", nil},
		"approved but empty id":   {"approved", &empty},
		"needs_review":            {"needs_review", &approved},
		"rejected":                {"rejected", &approved},
		"failed":                  {"failed", &approved},
	} {
		if _, err := requireExportableVersion(tc.status, tc.version); !errors.Is(err, ErrInvalidDocumentState) {
			t.Fatalf("%s: err = %v, want ErrInvalidDocumentState", name, err)
		}
	}
}

// The idempotency key is the sole delivery key (ADR-012): it must depend on the
// exact version and must not collide across export types.
func TestExportIdempotencyKeysAreVersionScopedAndDistinct(t *testing.T) {
	document := "3f2504e0-4f89-11d3-9a0c-0305e82c3301"
	v1, v2 := "aaaaaaaa-4f89-11d3-9a0c-0305e82c3301", "bbbbbbbb-4f89-11d3-9a0c-0305e82c3301"

	if csvExportIdempotencyKey(document, v1) != csvExportIdempotencyKey(document, v1) {
		t.Fatal("csv key is not stable")
	}
	if csvExportIdempotencyKey(document, v1) == csvExportIdempotencyKey(document, v2) {
		t.Fatal("csv key does not depend on the version")
	}
	if webhookExportIdempotencyKey(document, v1) == csvExportIdempotencyKey(document, v1) {
		t.Fatal("csv and webhook keys collide")
	}
	if !strings.HasPrefix(webhookExportIdempotencyKey(document, v1), "webhook_export:") {
		t.Fatalf("unexpected webhook key %q", webhookExportIdempotencyKey(document, v1))
	}
}

// A review edit is untrusted input, so the size bound must hold before any
// parsing work happens.
func TestDecodeHumanReviewInputRejectsEmptyAndOversizedBodies(t *testing.T) {
	if _, err := DecodeHumanReviewInput(nil); !errors.Is(err, ErrInvalidHumanReviewEdit) {
		t.Fatalf("empty body err = %v", err)
	}
	oversized := []byte(`{"supplier_name":"` + strings.Repeat("a", extraction.DefaultLimits().MaxProviderOutputBytes) + `"}`)
	if _, err := DecodeHumanReviewInput(oversized); !errors.Is(err, ErrInvalidHumanReviewEdit) {
		t.Fatalf("oversized body err = %v", err)
	}
	if _, err := DecodeHumanReviewInput([]byte(`{"currency":"USD"} {"currency":"EUR"}`)); !errors.Is(err, ErrInvalidHumanReviewEdit) {
		t.Fatal("trailing JSON was accepted")
	}
}

// A version whose currency the server does not recognize must render as empty
// strings rather than as a number divided by a guessed exponent.
func TestEditableFromNormalizedLeavesUnknownCurrencyAmountsEmpty(t *testing.T) {
	editable, err := editableFromNormalized([]byte(`{"rounding_policy_version":"money-v1","currency":"","total_minor":2400,"line_items":[{"description":"Service","total_minor":2400}]}`))
	if err != nil {
		t.Fatal(err)
	}
	if editable.Total != "" || editable.LineItems[0].Total != "" {
		t.Fatalf("amounts were rendered without a currency: %+v", editable)
	}
	if editable.LineItems[0].Description != "Service" {
		t.Fatalf("description lost: %+v", editable.LineItems[0])
	}
}

func TestStringValueTreatsNilAsEmpty(t *testing.T) {
	value := "supplier"
	if stringValue(nil) != "" || stringValue(&value) != "supplier" {
		t.Fatal("stringValue mismatch")
	}
}
