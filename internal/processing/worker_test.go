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

// The observer hooks are optional, so job execution must not depend on one
// being installed.
func TestOutcomeHooksAreOptionalAndForwardVerbatim(t *testing.T) {
	Worker{}.reportProcessOutcome(OutcomeSuccess, time.Second)
	Worker{}.reportExportOutcome(OutcomeRetry)

	var outcomes []string
	var durations []time.Duration
	worker := Worker{
		OnProcessFinished: func(outcome string, d time.Duration) {
			outcomes = append(outcomes, outcome)
			durations = append(durations, d)
		},
		OnExportFinished: func(outcome string) { outcomes = append(outcomes, outcome) },
	}
	worker.reportProcessOutcome(OutcomeDeadLetter, 250*time.Millisecond)
	worker.reportExportOutcome(OutcomeSuccess)

	if len(outcomes) != 2 || outcomes[0] != OutcomeDeadLetter || outcomes[1] != OutcomeSuccess {
		t.Fatalf("outcomes=%v", outcomes)
	}
	if len(durations) != 1 || durations[0] != 250*time.Millisecond {
		t.Fatalf("durations=%v", durations)
	}
}

// Every error the worker classifies must map to a summary that describes the
// failure without carrying tool output, a path, or document text.
func TestProcessingErrorSummaryIsSafeAndClassified(t *testing.T) {
	hostile := errors.New("/private/storage/tmp/abc.pdf: Syntax Error: Couldn't read xref")
	if got := processingErrorSummary(hostile); got != "processing failed; retry scheduled" {
		t.Fatalf("unclassified error leaked detail: %q", got)
	}
	if permanentProcessingError(hostile) {
		t.Fatal("an unclassified error must stay retryable")
	}

	for _, err := range []error{
		extraction.ErrMalformedPDF, extraction.ErrEncryptedPDF, extraction.ErrTooManyPages,
		extraction.ErrInputTooLarge, extraction.ErrInvalidProposalSchema, extraction.ErrUnsupportedOCR,
	} {
		if !permanentProcessingError(err) {
			t.Fatalf("%v should be permanent", err)
		}
	}
}

// Only allowlisted diagnostic codes keep their identity, and no adapter message
// text survives sanitization.
func TestSanitizeDiagnosticsRewritesEveryMessage(t *testing.T) {
	sanitized := sanitizeDiagnostics([]extraction.Diagnostic{
		{Code: "fake_fixture_unmatched", Message: "no fixture for /srv/storage/objects/ab.pdf"},
		{Code: extraction.HeuristicDiagnosticCode, Message: "read 7 fields from page 1"},
		{Code: "provider_rate_limited", Message: "key sk-live-123 exceeded quota"},
	})
	if len(sanitized) != 3 {
		t.Fatalf("got %d diagnostics", len(sanitized))
	}
	if sanitized[2].Code != "provider_diagnostic" {
		t.Fatalf("unknown code kept its identity: %q", sanitized[2].Code)
	}
	for _, diagnostic := range sanitized {
		if strings.Contains(diagnostic.Message, "/srv/") || strings.Contains(diagnostic.Message, "sk-live") ||
			strings.Contains(diagnostic.Message, "page 1") {
			t.Fatalf("adapter text survived: %q", diagnostic.Message)
		}
	}
}

func TestHasUsableTextIgnoresWhitespaceOnlyPages(t *testing.T) {
	blank := []extraction.PageText{{PageNumber: 1, Text: "  \n\t "}, {PageNumber: 2, Text: ""}}
	if hasUsableText(blank) {
		t.Fatal("whitespace-only pages must trigger the OCR fallback")
	}
	if !hasUsableText([]extraction.PageText{{PageNumber: 1, Text: " "}, {PageNumber: 2, Text: "Invoice"}}) {
		t.Fatal("a page with text must not trigger OCR")
	}
}
