// Package documents contains document identity and intake-facing boundaries.
package documents

import "time"

// Document is durable metadata for an immutable original. Actual intake is a
// later stage; this foundation type deliberately does not accept browser input.
type Document struct {
	ID        string
	ObjectKey string
	SHA256    [32]byte
	CreatedAt time.Time
}

// UploadResult is deliberately opaque: neither client filenames nor storage
// locations are part of the API contract.
type UploadResult struct {
	ID        string    `json:"id"`
	Status    string    `json:"status"`
	CreatedAt time.Time `json:"created_at"`
}
