package export

import (
	"bytes"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/reinhlord/invoiceflow/internal/invoices"
)

func TestComputeSignatureKnownVector(t *testing.T) {
	got := ComputeSignature("test-secret", "2026-07-24T12:00:00Z", []byte(`{"event":"invoice.exported"}`))
	want := "f9044174feba93928b698f4ee44dc3b9944b5a70c7e92b96e6b7de6c59fe8b46"
	if got != want {
		t.Fatalf("signature=%s, want known vector %s", got, want)
	}
	if err := VerifySignature("test-secret", "t=2026-07-24T12:00:00Z,v1="+got, "2026-07-24T12:00:00Z", []byte(`{"event":"invoice.exported"}`), time.Date(2026, 7, 24, 12, 4, 0, 0, time.UTC), WebhookReplayWindow); err != nil {
		t.Fatalf("known vector did not verify: %v", err)
	}
}

func TestWebhookIPValidationMatrix(t *testing.T) {
	for _, value := range []string{"10.0.0.1", "172.16.0.1", "192.168.1.1", "169.254.1.1", "127.0.0.1", "100.64.0.1", "192.0.2.1", "::1", "fc00::1", "fe80::1", "::ffff:192.168.1.1"} {
		if err := ValidateIP(net.ParseIP(value)); err == nil {
			t.Errorf("private/reserved IP %s was accepted", value)
		}
	}
	for _, value := range []string{"8.8.8.8", "1.1.1.1", "93.184.216.34"} {
		if err := ValidateIP(net.ParseIP(value)); err != nil {
			t.Errorf("public IP %s rejected: %v", value, err)
		}
	}
}

func TestWebhookURLValidationIsStrictByDefault(t *testing.T) {
	sender := NewStrictWebhookSender("secret", "https://example.test/webhook")
	for _, target := range []string{"http://example.com/webhook", "https://example.com:8443/webhook", "ftp://example.com/webhook", "https://user:password@example.com/webhook", "https://example.com/webhook?token=secret"} {
		if _, err := sender.ValidateURL(target); err == nil {
			t.Errorf("unsafe target %q was accepted", target)
		}
	}
	controlled := NewControlledWebhookSender("secret", "http://receiver:8090/webhook")
	if _, err := controlled.ValidateURL("http://receiver:8090/webhook"); err != nil {
		t.Fatalf("controlled receiver target rejected: %v", err)
	}
	if _, err := controlled.ValidateURL("http://127.0.0.1:8080/webhook"); err == nil {
		t.Fatal("controlled mode accepted a non-configured destination")
	}
	if _, err := NewControlledWebhookSender("secret", "http://receiver:8090/webhook?token=secret").ValidateURL("http://receiver:8090/webhook?token=secret"); err == nil {
		t.Fatal("controlled mode accepted a destination with a query secret")
	}
	if result := NewStrictWebhookSender("", "https://example.test/webhook").Send(t.Context(), WebhookPayload{Event: "invoice.exported"}); result.Error == nil {
		t.Fatal("strict sender without a secret attempted delivery")
	}
}

func TestCanonicalWebhookPayloadIsByteStableAcrossRetry(t *testing.T) {
	payload := WebhookPayload{Event: "invoice.exported", DocumentID: "doc-1", VersionNumber: 2, ApprovedAt: time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC), IdempotencyKey: "webhook_export:doc-1:version-2", Normalized: invoices.NormalizedProposal{Currency: "JPY"}}
	first, err := CanonicalPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CanonicalPayload(payload)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("canonical payload changed between retry attempts: %q vs %q", first, second)
	}
}

func TestVerifySignatureReplayAndConstantTimePath(t *testing.T) {
	body := []byte(`{"event":"invoice.exported","idempotency_key":"key-1"}`)
	now := time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC)
	timestamp := now.Format(time.RFC3339)
	signature := "t=" + timestamp + ",v1=" + ComputeSignature("secret", timestamp, body)
	if err := VerifySignature("secret", signature, timestamp, body, now.Add(4*time.Minute), WebhookReplayWindow); err != nil {
		t.Fatalf("fresh signature rejected: %v", err)
	}
	if err := VerifySignature("secret", signature, timestamp, body, now.Add(6*time.Minute), WebhookReplayWindow); err == nil {
		t.Fatal("stale signature accepted")
	}
	if err := VerifySignature("wrong", signature, timestamp, body, now, WebhookReplayWindow); err == nil {
		t.Fatal("signature with wrong secret accepted")
	}
}

func TestControlledReceiverValidatesCanonicalPayloadAndIdempotency(t *testing.T) {
	receiver := &ControlledReceiver{Secret: "secret", Now: func() time.Time { return time.Date(2026, 7, 24, 12, 0, 0, 0, time.UTC) }}
	body := []byte(`{"event":"invoice.exported","document_id":"doc-1","version_number":2,"approved_at":"2026-07-24T11:00:00Z","idempotency_key":"key-1","normalized":{}}`)
	// Canonicalize the fixture through the public payload type.
	canonical, err := CanonicalPayload(WebhookPayload{Event: "invoice.exported", DocumentID: "doc-1", VersionNumber: 2, ApprovedAt: time.Date(2026, 7, 24, 11, 0, 0, 0, time.UTC), IdempotencyKey: "key-1"})
	if err != nil {
		t.Fatal(err)
	}
	body = canonical
	timestamp := "2026-07-24T12:00:00Z"
	makeRequest := func() *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "http://receiver:8090/webhook", bytes.NewReader(body))
		req.Header.Set("X-InvoiceFlow-Timestamp", timestamp)
		req.Header.Set("X-InvoiceFlow-Idempotency-Key", "key-1")
		req.Header.Set("X-InvoiceFlow-Signature", "t="+timestamp+",v1="+ComputeSignature("secret", timestamp, body))
		out := httptest.NewRecorder()
		receiver.ServeHTTP(out, req)
		return out
	}
	if response := makeRequest(); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"accepted"`)) {
		t.Fatalf("first receiver response=%d %s", response.Code, response.Body.String())
	}
	if response := makeRequest(); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"duplicate"`)) {
		t.Fatalf("duplicate receiver response=%d %s", response.Code, response.Body.String())
	}
	if receiver.Validated != 1 || receiver.Duplicates != 1 {
		t.Fatalf("receiver counters=%d/%d, want 1/1", receiver.Validated, receiver.Duplicates)
	}
}
