package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reinhlord/invoiceflow/internal/app"
	"github.com/reinhlord/invoiceflow/internal/platform"
	"github.com/reinhlord/invoiceflow/internal/processing"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	config, err := app.LoadConfig()
	if err != nil {
		logger.Error("configuration error", "error", err)
		os.Exit(1)
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	startupCtx, cancel := context.WithTimeout(ctx, config.DBTimeout)
	defer cancel()
	pool, err := platform.OpenPool(startupCtx, config.DatabaseURL)
	if err != nil {
		logger.Error("database startup failed", "error", err)
		os.Exit(1)
	}
	defer pool.Close()
	if err := platform.Migrate(startupCtx, pool, config.MigrationDir); err != nil {
		logger.Error("migration failed", "error", err)
		os.Exit(1)
	}
	storage, err := platform.NewFileStorage(config.StorageDir)
	if err != nil {
		logger.Error("storage startup failed", "error", err)
		os.Exit(1)
	}
	repository := processing.NewRepository(pool)

	server := &http.Server{Addr: config.APIAddress, Handler: newHandlerWithDependencies(apiDependencies{db: pool, intake: repository, review: repository, storage: storage, actor: config.DemoActor, tempDir: storage.TemporaryDirectory()}), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = server.Shutdown(shutdownCtx)
	}()
	logger.Info("api listening", "address", config.APIAddress, "actor", config.DemoActor)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("api stopped", "error", err)
		os.Exit(1)
	}
}
