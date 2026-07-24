package extraction

import (
	"context"
	"errors"
)

// ErrInvalidExtractorChain means a fallback chain was built without both of its
// adapters. It is a configuration fault, not a document fault.
var ErrInvalidExtractorChain = errors.New("invalid structured extractor chain")

// HasCandidates reports whether a proposal carries any invoice value at all.
// Diagnostics and evidence alone do not count: an adapter that recognized
// nothing still returns them.
func (p Proposal) HasCandidates() bool {
	for _, value := range []*string{
		p.SupplierName, p.SupplierEmail, p.InvoiceNumber,
		p.IssueDate, p.DueDate, p.Currency,
		p.Subtotal, p.TaxAmount, p.Total,
	} {
		if value != nil && *value != "" {
			return true
		}
	}
	return len(p.LineItems) > 0
}

// FallbackStructuredExtractor tries Primary and, only when Primary recognized
// nothing at all, asks Fallback for a second opinion.
//
// The trigger is the shape of the result — a proposal with no candidate values
// — not a provider-specific diagnostic code, so the chain stays provider
// neutral. A Primary error is returned as is and never masked by the fallback:
// a bounded, classified extraction failure must keep flowing into the worker's
// existing retry and dead-letter path.
type FallbackStructuredExtractor struct {
	Primary  StructuredExtractor
	Fallback StructuredExtractor
}

// NewFallbackStructuredExtractor rejects an incomplete chain at startup rather
// than degrading silently at the first document.
func NewFallbackStructuredExtractor(primary, fallback StructuredExtractor) (*FallbackStructuredExtractor, error) {
	if primary == nil || fallback == nil {
		return nil, ErrInvalidExtractorChain
	}
	return &FallbackStructuredExtractor{Primary: primary, Fallback: fallback}, nil
}

func (f *FallbackStructuredExtractor) Extract(ctx context.Context, input StructuredExtractionInput, limits Limits) (Proposal, error) {
	proposal, err := f.Primary.Extract(ctx, input, limits)
	if err != nil {
		return Proposal{}, err
	}
	if proposal.HasCandidates() {
		return proposal, nil
	}
	// The primary adapter's diagnostics are preserved so the review UI can still
	// show that the fixture registry was consulted and did not match.
	fallbackProposal, err := f.Fallback.Extract(ctx, input, limits)
	if err != nil {
		return Proposal{}, err
	}
	fallbackProposal.Diagnostics = boundedDiagnostics(append(proposal.Diagnostics, fallbackProposal.Diagnostics...), limits.MaxDiagnostics)
	if err := limits.ValidateProposal(fallbackProposal); err != nil {
		return Proposal{}, err
	}
	return fallbackProposal, nil
}

func boundedDiagnostics(diagnostics []Diagnostic, max int) []Diagnostic {
	if max < 0 {
		max = 0
	}
	if len(diagnostics) > max {
		return diagnostics[:max]
	}
	return diagnostics
}
