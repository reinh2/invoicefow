// Package processing owns durable job vocabulary, not worker-specific adapters.
package processing

type JobType string
type JobStatus string

const (
	JobTypeProcess JobType = "process_document"
	JobTypeExport  JobType = "export_document"

	JobReady      JobStatus = "ready"
	JobRunning    JobStatus = "running"
	JobSucceeded  JobStatus = "succeeded"
	JobDeadLetter JobStatus = "dead_letter"
)

// Terminal outcomes a finished durable job reports to an observer. These are
// the only values a Worker hook emits, which is what keeps the metric label
// they feed bounded by code rather than by data.
const (
	OutcomeSuccess    = "success"
	OutcomeRetry      = "retry"
	OutcomeDeadLetter = "dead_letter"
)
