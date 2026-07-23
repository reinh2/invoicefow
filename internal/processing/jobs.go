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
