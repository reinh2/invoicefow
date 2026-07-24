package invoices

import "fmt"

type DocumentState string

const (
	StateUploaded    DocumentState = "uploaded"
	StateQueued      DocumentState = "queued"
	StateProcessing  DocumentState = "processing"
	StateNeedsReview DocumentState = "needs_review"
	StateApproved    DocumentState = "approved"
	StateRejected    DocumentState = "rejected"
	StateExported    DocumentState = "exported"
	StateFailed      DocumentState = "failed"
)

func (s DocumentState) Valid() bool {
	switch s {
	case StateUploaded, StateQueued, StateProcessing, StateNeedsReview, StateApproved, StateRejected, StateExported, StateFailed:
		return true
	default:
		return false
	}
}

// CanTransition encodes the state machine. Retrying moves failed work to queued.
func (s DocumentState) CanTransition(next DocumentState) bool {
	switch s {
	case StateUploaded:
		return next == StateQueued || next == StateFailed
	case StateQueued:
		return next == StateProcessing || next == StateFailed
	case StateProcessing:
		return next == StateNeedsReview || next == StateFailed || next == StateQueued
	case StateNeedsReview:
		return next == StateApproved || next == StateRejected
	case StateApproved:
		return next == StateExported
	case StateExported:
		return next == StateExported
	case StateFailed:
		return next == StateQueued
	default:
		return false
	}
}

func (s DocumentState) ValidateTransition(next DocumentState) error {
	if !s.Valid() || !next.Valid() || !s.CanTransition(next) {
		return fmt.Errorf("invalid document transition: %s -> %s", s, next)
	}
	return nil
}
