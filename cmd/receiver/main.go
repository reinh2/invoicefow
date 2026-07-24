package main

import (
	"log/slog"
	"net/http"
	"os"

	"github.com/reinhlord/invoiceflow/internal/export"
)

func main() {
	receiver := &export.ControlledReceiver{Secret: os.Getenv("WEBHOOK_SECRET")}
	mux := http.NewServeMux()
	mux.HandleFunc("/webhook", receiver.ServeHTTP)
	mux.HandleFunc("/stats", receiver.Stats)
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) })
	slog.New(slog.NewJSONHandler(os.Stdout, nil)).Info("controlled webhook receiver listening", "address", ":8090")
	if err := http.ListenAndServe(":8090", mux); err != nil {
		os.Exit(1)
	}
}
