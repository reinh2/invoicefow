package invoices

import "testing"

func TestDocumentTransitions(t *testing.T) {
	if err := StateUploaded.ValidateTransition(StateQueued); err != nil {
		t.Fatal(err)
	}
	if err := StateApproved.ValidateTransition(StateQueued); err == nil {
		t.Fatal("approved document must not return to queued")
	}
	if err := DocumentState("unknown").ValidateTransition(StateQueued); err == nil {
		t.Fatal("unknown state must fail")
	}
}
