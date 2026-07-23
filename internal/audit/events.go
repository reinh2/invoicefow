// Package audit defines append-only audit vocabulary.
package audit

import "time"

type Event struct {
	DocumentID string
	Sequence   int64
	Action     string
	Actor      string
	OccurredAt time.Time
}
