package extraction

import (
	"context"
	"errors"
	"testing"
)

type stubExtractor struct {
	proposal Proposal
	err      error
	calls    int
}

func (s *stubExtractor) Extract(context.Context, StructuredExtractionInput, Limits) (Proposal, error) {
	s.calls++
	if s.err != nil {
		return Proposal{}, s.err
	}
	return s.proposal, nil
}

func fallbackInput() StructuredExtractionInput {
	return StructuredExtractionInput{
		DocumentSHA256: heuristicTestHash,
		ReferenceText:  []PageText{{PageNumber: 1, Text: "Invoice No: A-1\n"}},
	}
}

func TestFallbackKeepsPrimaryResultWhenItRecognizedSomething(t *testing.T) {
	matched := ptrTo("Fixture Supplier")
	primary := &stubExtractor{proposal: Proposal{SupplierName: matched}}
	fallback := &stubExtractor{proposal: Proposal{InvoiceNumber: ptrTo("HEURISTIC")}}
	chain, err := NewFallbackStructuredExtractor(primary, fallback)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}

	proposal, err := chain.Extract(context.Background(), fallbackInput(), DefaultLimits())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if proposal.SupplierName == nil || *proposal.SupplierName != "Fixture Supplier" {
		t.Errorf("primary result must win, got %+v", proposal)
	}
	if fallback.calls != 0 {
		t.Errorf("fallback ran %d times, want 0", fallback.calls)
	}
}

func TestFallbackRunsOnlyWhenPrimaryFoundNothing(t *testing.T) {
	primary := &stubExtractor{proposal: Proposal{Diagnostics: []Diagnostic{{Code: "fake_fixture_unmatched", Message: "no match"}}}}
	fallback := &stubExtractor{proposal: Proposal{
		InvoiceNumber: ptrTo("HEURISTIC-1"),
		Diagnostics:   []Diagnostic{{Code: HeuristicDiagnosticCode, Message: "heuristic"}},
	}}
	chain, err := NewFallbackStructuredExtractor(primary, fallback)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}

	proposal, err := chain.Extract(context.Background(), fallbackInput(), DefaultLimits())
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if proposal.InvoiceNumber == nil || *proposal.InvoiceNumber != "HEURISTIC-1" {
		t.Errorf("fallback result expected, got %+v", proposal)
	}
	if fallback.calls != 1 {
		t.Errorf("fallback ran %d times, want 1", fallback.calls)
	}
	// The reviewer should still be able to see that the fixture registry was
	// consulted first and did not match.
	if len(proposal.Diagnostics) != 2 ||
		proposal.Diagnostics[0].Code != "fake_fixture_unmatched" ||
		proposal.Diagnostics[1].Code != HeuristicDiagnosticCode {
		t.Errorf("diagnostics = %+v, want both adapters represented in order", proposal.Diagnostics)
	}
}

func TestFallbackDoesNotMaskPrimaryError(t *testing.T) {
	primary := &stubExtractor{err: ErrInputTooLarge}
	fallback := &stubExtractor{proposal: Proposal{InvoiceNumber: ptrTo("HEURISTIC")}}
	chain, err := NewFallbackStructuredExtractor(primary, fallback)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}

	if _, err := chain.Extract(context.Background(), fallbackInput(), DefaultLimits()); !errors.Is(err, ErrInputTooLarge) {
		t.Fatalf("error = %v, want the primary classification preserved", err)
	}
	if fallback.calls != 0 {
		t.Errorf("fallback ran %d times after a primary error, want 0", fallback.calls)
	}
}

func TestFallbackBoundsCombinedDiagnostics(t *testing.T) {
	limits := DefaultLimits()
	limits.MaxDiagnostics = 1
	primary := &stubExtractor{proposal: Proposal{Diagnostics: []Diagnostic{{Code: "fake_fixture_unmatched", Message: "no match"}}}}
	fallback := &stubExtractor{proposal: Proposal{
		InvoiceNumber: ptrTo("HEURISTIC-1"),
		Diagnostics:   []Diagnostic{{Code: HeuristicDiagnosticCode, Message: "heuristic"}},
	}}
	chain, err := NewFallbackStructuredExtractor(primary, fallback)
	if err != nil {
		t.Fatalf("chain: %v", err)
	}

	proposal, err := chain.Extract(context.Background(), fallbackInput(), limits)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if len(proposal.Diagnostics) > limits.MaxDiagnostics {
		t.Errorf("diagnostics = %d, want at most %d", len(proposal.Diagnostics), limits.MaxDiagnostics)
	}
}

func TestNewFallbackStructuredExtractorRejectsIncompleteChain(t *testing.T) {
	if _, err := NewFallbackStructuredExtractor(nil, HeuristicStructuredExtractor{}); !errors.Is(err, ErrInvalidExtractorChain) {
		t.Errorf("missing primary: err = %v", err)
	}
	if _, err := NewFallbackStructuredExtractor(HeuristicStructuredExtractor{}, nil); !errors.Is(err, ErrInvalidExtractorChain) {
		t.Errorf("missing fallback: err = %v", err)
	}
}

func TestHasCandidatesIgnoresDiagnosticsAndEvidence(t *testing.T) {
	empty := Proposal{
		Diagnostics: []Diagnostic{{Code: "x", Message: "y"}},
		Evidence:    []Evidence{{Field: "total", PageNumber: 1, Excerpt: "Total 1.00"}},
	}
	if empty.HasCandidates() {
		t.Error("diagnostics and evidence alone must not count as candidates")
	}
	if !(Proposal{LineItems: []LineItemProposal{{Description: ptrTo("item")}}}).HasCandidates() {
		t.Error("a line item is a candidate")
	}
	if (Proposal{SupplierName: ptrTo("")}).HasCandidates() {
		t.Error("an empty string is not a candidate")
	}
}

func ptrTo(value string) *string { return &value }
