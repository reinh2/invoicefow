package export

import (
	"bytes"
	"crypto/sha256"
	"encoding/json"
	"io"
	"net/http"
	"sync"
	"time"
)

// ControlledReceiver is the deliberately narrow Compose demo adapter. It
// validates the same public webhook contract as a receiver should: canonical
// JSON, HMAC, replay window, and idempotency-key consistency.
type ControlledReceiver struct {
	Secret string
	Now    func() time.Time

	mu         sync.Mutex
	Bodies     map[string][32]byte
	Validated  int
	Duplicates int
	LastKey    string
}

func (r *ControlledReceiver) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	if req.Method != http.MethodPost || req.URL.Path != "/webhook" {
		http.NotFound(w, req)
		return
	}
	body, err := io.ReadAll(io.LimitReader(req.Body, maxWebhookBodyBytes+1))
	if err != nil || len(body) > maxWebhookBodyBytes {
		http.Error(w, "invalid body", http.StatusRequestEntityTooLarge)
		return
	}
	timestamp := req.Header.Get("X-InvoiceFlow-Timestamp")
	if err := VerifySignature(r.Secret, req.Header.Get("X-InvoiceFlow-Signature"), timestamp, body, r.now(), WebhookReplayWindow); err != nil {
		http.Error(w, "invalid signature", http.StatusUnauthorized)
		return
	}
	var payload WebhookPayload
	if json.Unmarshal(body, &payload) != nil || payload.IdempotencyKey == "" || payload.Event != "invoice.exported" || req.Header.Get("X-InvoiceFlow-Idempotency-Key") != payload.IdempotencyKey {
		http.Error(w, "invalid payload", http.StatusBadRequest)
		return
	}
	canonical, err := CanonicalPayload(payload)
	if err != nil || !bytes.Equal(canonical, body) {
		http.Error(w, "non-canonical payload", http.StatusBadRequest)
		return
	}
	hash := sha256.Sum256(body)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.Bodies == nil {
		r.Bodies = make(map[string][32]byte)
	}
	if previous, exists := r.Bodies[payload.IdempotencyKey]; exists {
		if previous != hash {
			http.Error(w, "idempotency key reused", http.StatusConflict)
			return
		}
		r.Duplicates++
		writeReceiverJSON(w, http.StatusOK, map[string]any{"status": "duplicate", "idempotency_key": payload.IdempotencyKey})
		return
	}
	r.Bodies[payload.IdempotencyKey] = hash
	r.LastKey = payload.IdempotencyKey
	r.Validated++
	writeReceiverJSON(w, http.StatusOK, map[string]any{"status": "accepted", "idempotency_key": payload.IdempotencyKey})
}

func (r *ControlledReceiver) Stats(w http.ResponseWriter, _ *http.Request) {
	r.mu.Lock()
	defer r.mu.Unlock()
	writeReceiverJSON(w, http.StatusOK, map[string]any{"validated_count": r.Validated, "duplicate_count": r.Duplicates, "idempotency_count": len(r.Bodies), "last_idempotency_key": r.LastKey})
}

func (r *ControlledReceiver) now() time.Time {
	if r.Now != nil {
		return r.Now()
	}
	return time.Now().UTC()
}

func writeReceiverJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
