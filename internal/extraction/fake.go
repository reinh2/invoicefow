package extraction

import (
	"context"
	"errors"
	"strings"
)

var ErrInvalidFakeFixture = errors.New("invalid fake extraction fixture")

// FakeFixture is server configuration for one fictional offline demo input.
// A match requires both the server-computed content hash and the configured
// marker in the bounded reference text. Markers are not credentials; requiring
// both makes the fake deterministic without treating arbitrary hostile text as
// an invoice fixture.
type FakeFixture struct {
	DocumentSHA256 string
	Marker         string
	Proposal       Proposal
}

// FakeStructuredExtractor is an offline deterministic adapter for the default
// demo. It never makes a network call and never parses, normalizes, or
// validates invoice values. Unknown input deterministically returns an empty
// partial proposal with a safe diagnostic for human review.
type FakeStructuredExtractor struct {
	fixtures map[string]fakeFixture
}

type fakeFixture struct {
	marker   string
	proposal Proposal
}

// NewFakeStructuredExtractor builds an immutable-copy fixture registry. Each
// fixture hash may be registered once so the fake's behavior is deterministic.
func NewFakeStructuredExtractor(fixtures []FakeFixture) (*FakeStructuredExtractor, error) {
	configured := make(map[string]fakeFixture, len(fixtures))
	for _, fixture := range fixtures {
		if !validSHA256(fixture.DocumentSHA256) || strings.TrimSpace(fixture.Marker) == "" {
			return nil, ErrInvalidFakeFixture
		}
		if _, exists := configured[fixture.DocumentSHA256]; exists {
			return nil, ErrInvalidFakeFixture
		}
		configured[fixture.DocumentSHA256] = fakeFixture{
			marker:   fixture.Marker,
			proposal: cloneProposal(fixture.Proposal),
		}
	}
	return &FakeStructuredExtractor{fixtures: configured}, nil
}

// Extract returns only a raw proposal. It is safe to use in a no-key demo, but
// it deliberately has no workflow side effects or external authority.
func (f *FakeStructuredExtractor) Extract(ctx context.Context, input StructuredExtractionInput, limits Limits) (Proposal, error) {
	if err := ctx.Err(); err != nil {
		return Proposal{}, err
	}
	if err := limits.ValidateStructuredInput(input); err != nil {
		return Proposal{}, err
	}

	fixture, exists := f.fixtures[input.DocumentSHA256]
	if exists && containsMarker(input.ReferenceText, fixture.marker) {
		proposal := cloneProposal(fixture.proposal)
		if err := limits.ValidateProposal(proposal); err != nil {
			return Proposal{}, err
		}
		return proposal, nil
	}
	proposal := Proposal{Diagnostics: []Diagnostic{{
		Code:    "fake_fixture_unmatched",
		Message: "No configured fictional fixture matched this document.",
	}}}
	if err := limits.ValidateProposal(proposal); err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

func containsMarker(pages []PageText, marker string) bool {
	for _, page := range pages {
		if strings.Contains(page.Text, marker) {
			return true
		}
	}
	return false
}

func cloneProposal(proposal Proposal) Proposal {
	clone := proposal
	clone.SupplierName = cloneString(proposal.SupplierName)
	clone.SupplierEmail = cloneString(proposal.SupplierEmail)
	clone.InvoiceNumber = cloneString(proposal.InvoiceNumber)
	clone.IssueDate = cloneString(proposal.IssueDate)
	clone.DueDate = cloneString(proposal.DueDate)
	clone.Currency = cloneString(proposal.Currency)
	clone.Subtotal = cloneString(proposal.Subtotal)
	clone.TaxAmount = cloneString(proposal.TaxAmount)
	clone.Total = cloneString(proposal.Total)
	clone.LineItems = make([]LineItemProposal, len(proposal.LineItems))
	for i, item := range proposal.LineItems {
		clone.LineItems[i] = LineItemProposal{
			Description: cloneString(item.Description),
			Quantity:    cloneString(item.Quantity),
			UnitPrice:   cloneString(item.UnitPrice),
			TaxAmount:   cloneString(item.TaxAmount),
			Total:       cloneString(item.Total),
		}
	}
	clone.Evidence = make([]Evidence, len(proposal.Evidence))
	for i, evidence := range proposal.Evidence {
		clone.Evidence[i] = evidence
		if evidence.BoundingBox != nil {
			box := *evidence.BoundingBox
			clone.Evidence[i].BoundingBox = &box
		}
	}
	clone.Diagnostics = append([]Diagnostic(nil), proposal.Diagnostics...)
	return clone
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
