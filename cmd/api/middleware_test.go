package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// countingHandler records every slog record so a test can assert that exactly
// one access line is emitted per request and inspect its attributes.
type countingHandler struct {
	records []map[string]any
}

func (h *countingHandler) Enabled(context.Context, slog.Level) bool { return true }
func (h *countingHandler) Handle(_ context.Context, record slog.Record) error {
	attrs := map[string]any{"msg": record.Message}
	record.Attrs(func(a slog.Attr) bool {
		attrs[a.Key] = a.Value.Any()
		return true
	})
	h.records = append(h.records, attrs)
	return nil
}
func (h *countingHandler) WithAttrs([]slog.Attr) slog.Handler { return h }
func (h *countingHandler) WithGroup(string) slog.Handler      { return h }

// reviewHandler builds a handler whose review dependency is present, so an
// unknown document id reaches the 404 envelope rather than the missing-service
// 500.
func reviewHandler() http.Handler {
	return newHandlerWithDependencies(apiDependencies{db: fakePinger{}, review: &fakeReview{}, actor: "test"})
}

func TestRequestIDInErrorEnvelopeMatchesResponseHeader(t *testing.T) {
	handler := withRequestContext(reviewHandler(), slog.New(&countingHandler{}))
	recorder := httptest.NewRecorder()
	// An unknown /api path answers with the JSON error envelope.
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/documents/not-a-uuid", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("got status %d, want 404", recorder.Code)
	}
	var body apiError
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	header := recorder.Header().Get(RequestIDHeader)
	if header == "" {
		t.Fatal("response carried no request id header")
	}
	if body.Error.RequestID != header {
		t.Fatalf("envelope id %q != header id %q", body.Error.RequestID, header)
	}
}

func TestEveryRequestIsLoggedExactlyOnce(t *testing.T) {
	sink := &countingHandler{}
	handler := withRequestContext(reviewHandler(), slog.New(sink))

	for _, path := range []string{"/healthz", "/readyz", "/api/v1/documents/not-a-uuid"} {
		handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, path, nil))
	}
	if len(sink.records) != 3 {
		t.Fatalf("got %d log records for 3 requests", len(sink.records))
	}

	last := sink.records[2]
	for _, key := range []string{"method", "path", "status", "duration_ms", "request_id"} {
		if _, ok := last[key]; !ok {
			t.Fatalf("access log line is missing %q: %v", key, last)
		}
	}
	if last["status"] != int64(http.StatusNotFound) {
		t.Fatalf("logged status %v, want 404", last["status"])
	}
	if last["path"] != "/api/v1/documents/not-a-uuid" {
		t.Fatalf("logged path %v", last["path"])
	}
}

// A successful response must be logged with its real status, not the 200 the
// recorder is seeded with, and healthy paths must still carry the header.
func TestAccessLogRecordsExplicitStatusAndHeaderOnSuccess(t *testing.T) {
	sink := &countingHandler{}
	handler := withRequestContext(reviewHandler(), slog.New(sink))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	if recorder.Header().Get(RequestIDHeader) == "" {
		t.Fatal("successful response carried no request id header")
	}
	if sink.records[0]["status"] != int64(http.StatusOK) {
		t.Fatalf("logged status %v, want 200", sink.records[0]["status"])
	}
}

// The path is client-supplied, so control characters and unbounded length must
// not reach a log record.
func TestSafeLogPathStripsControlBytesAndBounds(t *testing.T) {
	if got := safeLogPath("/api/\n\x00v1"); got != "/api/??v1" {
		t.Fatalf("got %q", got)
	}
	long := "/" + strings.Repeat("a", 500)
	if got := safeLogPath(long); len(got) != maxLoggedPathBytes {
		t.Fatalf("got length %d, want %d", len(got), maxLoggedPathBytes)
	}
}

// Without the middleware a handler must still produce an id rather than an
// empty field in the envelope.
func TestRequestIDFallsBackWhenMiddlewareIsAbsent(t *testing.T) {
	recorder := httptest.NewRecorder()
	reviewHandler().ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/documents/not-a-uuid", nil))
	var body apiError
	if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if body.Error.RequestID == "" {
		t.Fatal("envelope carried no request id")
	}
}
