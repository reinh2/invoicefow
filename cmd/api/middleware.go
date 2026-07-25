package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/reinhlord/invoiceflow/internal/processing"
)

// RequestIDHeader carries the id the server assigned to a request. The same
// value appears in the JSON error envelope, so an operator can join what a
// client reports to a server log line.
const RequestIDHeader = "X-Request-Id"

type requestIDContextKey struct{}

// maxLoggedPathBytes bounds the request path a log line may contain. The path
// is client-supplied, so it is both truncated and stripped of anything that is
// not printable ASCII before it reaches a log record.
const maxLoggedPathBytes = 200

// withRequestContext assigns a request id at the edge, publishes it in the
// response header and request context, and emits exactly one structured access
// log line per request.
//
// Only server-derived or sanitized values are logged: method, a bounded
// printable-ASCII path with the query string dropped, response status, and
// duration. Request bodies, uploaded file names, storage keys, document text,
// and configuration secrets never reach this log (AGENTS.md log-sanitization
// rule).
func withRequestContext(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := newRequestID()
		w.Header().Set(RequestIDHeader, id)
		recorder := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		started := time.Now()
		next.ServeHTTP(recorder, r.WithContext(context.WithValue(r.Context(), requestIDContextKey{}, id)))
		if logger != nil {
			logger.Info("api request",
				"method", r.Method,
				"path", safeLogPath(r.URL.Path),
				"status", recorder.status,
				"duration_ms", time.Since(started).Milliseconds(),
				"request_id", id,
			)
		}
	})
}

// requestID returns the id assigned by withRequestContext. When the middleware
// is not installed — a handler constructed directly in a test — it falls back to
// a freshly generated id so an error envelope always carries one.
func requestID(r *http.Request) string {
	if r != nil {
		if id, ok := r.Context().Value(requestIDContextKey{}).(string); ok && id != "" {
			return id
		}
	}
	return newRequestID()
}

func newRequestID() string {
	id, err := processing.NewID()
	if err != nil || id == "" {
		return "request-failed"
	}
	return id
}

// safeLogPath reduces a client-supplied path to bounded printable ASCII, so a
// crafted request cannot inject control characters or unbounded text into the
// log stream.
func safeLogPath(path string) string {
	if len(path) > maxLoggedPathBytes {
		path = path[:maxLoggedPathBytes]
	}
	out := make([]rune, 0, len(path))
	for _, r := range path {
		if r < 0x20 || r > 0x7e {
			r = '?'
		}
		out = append(out, r)
	}
	return string(out)
}

// statusRecorder observes the status code without altering the response. It
// exposes the wrapped writer through Unwrap so http.ResponseController keeps
// working for handlers that stream.
type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (s *statusRecorder) WriteHeader(status int) {
	if !s.written {
		s.status, s.written = status, true
	}
	s.ResponseWriter.WriteHeader(status)
}

func (s *statusRecorder) Write(b []byte) (int, error) {
	s.written = true
	return s.ResponseWriter.Write(b)
}

func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }
