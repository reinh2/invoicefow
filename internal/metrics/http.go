package metrics

import (
	"bytes"
	"context"
	"net/http"
	"time"
)

func contextWithTimeout(req *http.Request) (context.Context, context.CancelFunc) {
	return context.WithTimeout(req.Context(), ScrapeTimeout)
}

// ScrapeTimeout bounds the database work one scrape may cause. A slow or stuck
// gauge query must not hold a connection indefinitely.
const ScrapeTimeout = 5 * time.Second

// Handler serves the registry at /metrics and nothing else.
//
// The exposition is rendered into a buffer first, so a collection error cannot
// leave a partially written response with a 200 status. The endpoint is served
// on its own listener (ADR-017) and carries no authentication of its own: it
// must be bound to an address that is not publicly reachable.
func (r *Registry) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, req *http.Request) {
		ctx, cancel := contextWithTimeout(req)
		defer cancel()
		var buffer bytes.Buffer
		if err := r.WriteText(ctx, &buffer); err != nil {
			http.Error(w, "metrics unavailable", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(buffer.Bytes())
	})
	return mux
}
