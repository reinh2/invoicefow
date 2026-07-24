package processing

import (
	"bytes"
	"testing"
	"time"

	"github.com/reinhlord/invoiceflow/internal/export"
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
