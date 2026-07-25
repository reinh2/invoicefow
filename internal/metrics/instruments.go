package metrics

import "context"

// The outcome label carries only the server-owned constants declared by
// internal/processing (success, retry, dead_letter), so its cardinality is fixed
// by the code rather than by data. This package deliberately does not import
// that one: instrumentation depends on the domain, never the reverse.
//
// extractionDurationBounds are seconds. The upper bounds bracket the configured
// PDF (15 s) and OCR (30 s) timeouts, so a run that times out is visible as a
// distinct bucket rather than being lost in +Inf.
var extractionDurationBounds = []float64{0.1, 0.25, 0.5, 1, 2, 5, 10, 15, 30, 60}

// StatusCounts is the database-sourced snapshot a Worker instruments as gauges.
type StatusCounts struct {
	// DocumentsByStatus counts documents per document state.
	DocumentsByStatus []LabeledValue
	// JobsByStatus counts durable jobs per job status, which is what "queue
	// depth" means here: the number of ready jobs waiting to be claimed.
	JobsByStatus []LabeledValue
}

// StatusSource reads the current counts. It is the repository in production and
// a stub in tests, so the instrument set stays testable without PostgreSQL.
type StatusSource interface {
	StatusCounts(context.Context) (StatusCounts, error)
}

// Worker is the instrument set the worker process reports.
type Worker struct {
	Registry *Registry
	// ProcessJobs counts finished process jobs by terminal outcome.
	ProcessJobs *Counter
	// ExportJobs counts finished webhook export jobs by terminal outcome.
	ExportJobs *Counter
	// ExtractionDuration measures how long one document's extraction took,
	// whether it succeeded or failed.
	ExtractionDuration *Histogram
}

// NewWorker registers the worker instrument set. source may be nil, in which
// case the database-backed gauges are omitted rather than reporting zeroes that
// would read as an empty, healthy system.
func NewWorker(source StatusSource) *Worker {
	registry := NewRegistry()
	worker := &Worker{
		Registry: registry,
		ProcessJobs: registry.Counter("invoiceflow_process_jobs_total",
			"Finished document processing jobs by terminal outcome.", "outcome"),
		ExportJobs: registry.Counter("invoiceflow_export_jobs_total",
			"Finished webhook export jobs by terminal outcome.", "outcome"),
		ExtractionDuration: registry.Histogram("invoiceflow_extraction_duration_seconds",
			"Wall-clock duration of one document extraction attempt, in seconds.", extractionDurationBounds),
	}
	if source != nil {
		registry.GaugeFunc("invoiceflow_documents", "Documents by state.", "status",
			func(ctx context.Context) ([]LabeledValue, error) {
				counts, err := source.StatusCounts(ctx)
				return counts.DocumentsByStatus, err
			})
		registry.GaugeFunc("invoiceflow_jobs", "Durable jobs by status; queue depth is the ready series.", "status",
			func(ctx context.Context) ([]LabeledValue, error) {
				counts, err := source.StatusCounts(ctx)
				return counts.JobsByStatus, err
			})
	}
	return worker
}
