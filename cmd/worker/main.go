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
	structured, err := buildStructuredExtractor(config)
	if err != nil {
		logger.Error("structured extractor configuration failed", "error", err)
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
		Structured: structured, Limits: limits, Lease: 45 * time.Second, RetryDelay: 15 * time.Second,
		WebhookSender: sender,
		OnProviderError: func(documentID string, err error) {
			logger.Error("structured extraction provider failed", "document_id", documentID, "error", err.Error())
		},
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
	logger.Info("worker ready; Stage 3 extraction and Stage 5 webhook export execution enabled", "extractor", config.Extractor)
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

// buildStructuredExtractor selects the structured-extraction provider from
// server configuration. "fake" is the deterministic offline default; "openai"
// is an opt-in live provider. The API key never leaves configuration here and
// is not logged.
func buildStructuredExtractor(config app.Config) (extraction.StructuredExtractor, error) {
	if config.Extractor == "openai" {
		return extraction.NewOpenAIStructuredExtractor(extraction.OpenAIOptions{
			APIKey:  config.OpenAIAPIKey,
			Model:   config.OpenAIModel,
			BaseURL: config.OpenAIBaseURL,
		})
	}
	return extraction.NewFakeStructuredExtractor(defaultFakeFixtures())
}

func ptr(value string) *string { return &value }

// fakeLine builds one line-item proposal candidate. Line tax is "0.00" so the
// per-line arithmetic check (quantity*unit + tax == total) holds for the
// consistent fixtures and is not the source of any warning.
func fakeLine(description, quantity, unitPrice, total string) extraction.LineItemProposal {
	return extraction.LineItemProposal{
		Description: ptr(description), Quantity: ptr(quantity),
		UnitPrice: ptr(unitPrice), TaxAmount: ptr("0.00"), Total: ptr(total),
	}
}

// defaultFakeFixtures configures the offline deterministic extractor. Each entry
// pairs the committed SHA-256 of a fictional document in testdata/ with the
// marker embedded in that document and the proposal the extractor returns for
// it. cmd/worker's TestDefaultFakeFixtures verifies the hashes still match the
// committed files and that each proposal normalizes to its expected warnings.
//
// The synthetic OFFICE-001 file predates the realistic fixtures and is retained
// only because the landing page illustrates it by name; the demo, smoke test,
// and captured media use the three realistic documents below.
func defaultFakeFixtures() []extraction.FakeFixture {
	office := extraction.FakeFixture{
		DocumentSHA256: "86ab48c217acdd9f083e8f2d24fc8f547ec8c80a10cd958121a79ffb3f229e99",
		Marker:         "INVOICEFLOW_FIXTURE:OFFICE-001",
		Proposal: extraction.Proposal{
			SupplierName: ptr("Fictional Office Goods"), SupplierEmail: ptr("billing@example.test"),
			InvoiceNumber: ptr("OFFICE-001"), IssueDate: ptr("2026-07-01"), DueDate: ptr("2026-07-31"),
			Currency: ptr("USD"), Subtotal: ptr("20.00"), TaxAmount: ptr("4.00"), Total: ptr("24.00"),
			LineItems: []extraction.LineItemProposal{fakeLine("Fictional paper", "2", "10.00", "20.00")},
		},
	}

	// Realistic text PDF, clean text extraction, no server warnings.
	aurora := extraction.FakeFixture{
		DocumentSHA256: "f76e1b0c0a972a83d57528f1ca0810d94d633bfe812899ae0c093d3d9d94ec99",
		Marker:         "INVOICEFLOW_FIXTURE:AURORA-1042",
		Proposal: extraction.Proposal{
			SupplierName: ptr("Aurora Stationery Co."), SupplierEmail: ptr("billing@aurora-stationery.example"),
			InvoiceNumber: ptr("AURORA-1042"), IssueDate: ptr("2026-06-15"), DueDate: ptr("2026-07-15"),
			Currency: ptr("USD"), Subtotal: ptr("80.00"), TaxAmount: ptr("6.40"), Total: ptr("86.40"),
			LineItems: []extraction.LineItemProposal{
				fakeLine("A4 copy paper, 80 gsm (5 reams)", "5", "6.00", "30.00"),
				fakeLine("Gel ink pens, box of 12", "3", "8.00", "24.00"),
				fakeLine("Mesh desk organizer", "2", "13.00", "26.00"),
			},
		},
	}

	// Realistic raster image, exercised through the Tesseract OCR path. The
	// marker is a bare letters/digits/hyphen token so a pinned Tesseract reads
	// it verbatim.
	meridian := extraction.FakeFixture{
		DocumentSHA256: "f4f911b595ba897de5f4c1d8dd969f9eda53c1f1522ccb45219a03350a975ffd",
		Marker:         "MERIDIAN-2087",
		Proposal: extraction.Proposal{
			SupplierName: ptr("Meridian Office Supplies"), SupplierEmail: ptr("accounts@meridian-supplies.example"),
			InvoiceNumber: ptr("MERIDIAN-2087"), IssueDate: ptr("2026-06-18"), DueDate: ptr("2026-07-18"),
			Currency: ptr("USD"), Subtotal: ptr("63.00"), TaxAmount: ptr("5.04"), Total: ptr("68.04"),
			LineItems: []extraction.LineItemProposal{
				fakeLine("Ballpoint pens, box of 50", "4", "9.00", "36.00"),
				fakeLine("Sticky notes, pack of 12", "6", "4.50", "27.00"),
			},
		},
	}

	// Realistic text PDF whose subtotal + tax (250.00 + 47.50 = 297.50) does not
	// equal its total (290.00), so the normalizer emits exactly one
	// subtotal_tax_total_mismatch warning for the human to reconcile.
	cedarline := extraction.FakeFixture{
		DocumentSHA256: "6767ba11afd4b7e926196d268491c761e08128c1eed3c9a349dfe4b77a5dd945",
		Marker:         "INVOICEFLOW_FIXTURE:CEDAR-3390",
		Proposal: extraction.Proposal{
			SupplierName: ptr("Cedarline Services LLC"), SupplierEmail: ptr("ar@cedarline.example"),
			InvoiceNumber: ptr("CEDAR-3390"), IssueDate: ptr("2026-06-20"), DueDate: ptr("2026-07-20"),
			Currency: ptr("USD"), Subtotal: ptr("250.00"), TaxAmount: ptr("47.50"), Total: ptr("290.00"),
			LineItems: []extraction.LineItemProposal{
				fakeLine("Managed hosting, monthly", "10", "15.00", "150.00"),
				fakeLine("On-site support, hours", "4", "25.00", "100.00"),
			},
		},
	}

	return []extraction.FakeFixture{office, aurora, meridian, cedarline}
}
