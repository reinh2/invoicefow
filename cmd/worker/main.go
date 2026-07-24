package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/reinhlord/invoiceflow/internal/app"
	"github.com/reinhlord/invoiceflow/internal/export"
	"github.com/reinhlord/invoiceflow/internal/extraction"
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
	limits := extraction.DefaultLimits()
	fixtures := defaultFakeFixtures()
	fake, err := extraction.NewFakeStructuredExtractor(fixtures)
	if err != nil {
		logger.Error("fake extractor configuration failed", "error", err)
		os.Exit(1)
	}
	var sender *export.WebhookSender
	if config.WebhookMode == "controlled" {
		sender = export.NewControlledWebhookSender(config.WebhookSecret, config.WebhookURL)
	} else {
		sender = export.NewStrictWebhookSender(config.WebhookSecret, config.WebhookURL)
	}
	worker := processing.Worker{
		Repository: repository, Storage: storage,
		Text:       extraction.PDFTextExtractor{TemporaryDir: storage.TemporaryDirectory()},
		OCR:        extraction.TesseractOCR{TemporaryDir: storage.TemporaryDirectory()},
		Structured: fake, Limits: limits, Lease: 45 * time.Second, RetryDelay: 15 * time.Second,
		WebhookSender: sender,
	}
	maintain := func() {
		maintenanceCtx, maintenanceCancel := context.WithTimeout(ctx, config.DBTimeout)
		defer maintenanceCancel()
		if removed, err := repository.ReconcileOrphanedObjects(maintenanceCtx, storage, 24*time.Hour); err != nil {
			logger.Error("orphan reconciliation failed", "error", err)
		} else if removed > 0 {
			logger.Info("reconciled orphaned storage objects", "count", removed)
		}
		if removed, err := storage.CleanupTemporaryFilesOlderThan(maintenanceCtx, time.Now().UTC().Add(-24*time.Hour)); err != nil {
			logger.Error("temporary intake cleanup failed", "error", err)
		} else if removed > 0 {
			logger.Info("reconciled temporary intake files", "count", removed)
		}
		if recovered, err := repository.RecoverExpiredLeases(maintenanceCtx); err != nil {
			logger.Error("expired lease recovery failed", "error", err)
		} else if recovered > 0 {
			logger.Info("recovered expired job leases", "count", recovered)
		}
	}
	maintain()
	logger.Info("worker ready; Stage 3 extraction and Stage 5 webhook export execution enabled")
	maintenanceTicker := time.NewTicker(5 * time.Minute)
	defer maintenanceTicker.Stop()
	jobTicker := time.NewTicker(time.Second)
	defer jobTicker.Stop()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-maintenanceTicker.C:
				maintain()
			case <-jobTicker.C:
				processed, err := worker.RunOnce(ctx)
				if err != nil {
					logger.Error("process job failed", "error", err)
				} else if processed {
					logger.Info("processed document job")
				}
				exported, err := worker.RunExportOnce(ctx)
				if err != nil {
					logger.Error("export job failed", "error", err)
				} else if exported {
					logger.Info("processed export job")
				}
			}
		}
	}()
	<-ctx.Done()
}

func defaultFakeFixtures() []extraction.FakeFixture {
	supplier, email, number, issueDate, dueDate, currency := "Fictional Office Goods", "billing@example.test", "OFFICE-001", "2026-07-01", "2026-07-31", "USD"
	subtotal, tax, total, description, quantity, price := "20.00", "4.00", "24.00", "Fictional paper", "2", "10.00"
	f1 := extraction.FakeFixture{DocumentSHA256: "86ab48c217acdd9f083e8f2d24fc8f547ec8c80a10cd958121a79ffb3f229e99", Marker: "INVOICEFLOW_FIXTURE:OFFICE-001", Proposal: extraction.Proposal{
		SupplierName: &supplier, SupplierEmail: &email, InvoiceNumber: &number, IssueDate: &issueDate, DueDate: &dueDate, Currency: &currency, Subtotal: &subtotal, TaxAmount: &tax, Total: &total,
		LineItems: []extraction.LineItemProposal{{Description: &description, Quantity: &quantity, UnitPrice: &price, Total: &subtotal}},
	}}
	f2 := f1
	f2.DocumentSHA256 = "3672ec274f27b1716f58b1517c4a4ac3ee66ab8b07100ed5cd6ffe0a8f68ecdc"
	return []extraction.FakeFixture{f1, f2}
}
