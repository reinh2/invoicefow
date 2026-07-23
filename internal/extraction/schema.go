package extraction

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
)

// ErrInvalidProposalSchema means a provider did not return the narrow Stage 3
// proposal shape. It is intentionally not a retryable provider detail.
var ErrInvalidProposalSchema = errors.New("invalid provider proposal schema")

// DecodeProposalJSON accepts one strict proposal object. It rejects unknown
// fields, trailing JSON, and non-string candidate values before the normalizer
// sees them. Limits remain a separate trusted-server check.
func DecodeProposalJSON(raw []byte, limits Limits) (Proposal, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var proposal Proposal
	if err := decoder.Decode(&proposal); err != nil {
		return Proposal{}, ErrInvalidProposalSchema
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return Proposal{}, ErrInvalidProposalSchema
	}
	if err := limits.ValidateProposal(proposal); err != nil {
		return Proposal{}, err
	}
	return proposal, nil
}

// ValidateEvidence checks the meaningful part of evidence semantics after
// source extraction: the adapter cannot point at a nonexistent page or invent
// an excerpt that does not occur in that page's bounded source text.
func ValidateEvidence(proposal Proposal, pages []PageText, limits Limits) error {
	if err := limits.ValidateProposal(proposal); err != nil {
		return err
	}
	byPage := make(map[int]string, len(pages))
	for _, page := range pages {
		byPage[page.PageNumber] = page.Text
	}
	for _, evidence := range proposal.Evidence {
		if !validEvidenceField(evidence.Field) || evidence.Excerpt == "" || !containsPageExcerpt(byPage[evidence.PageNumber], evidence.Excerpt) {
			return ErrInvalidInput
		}
	}
	return nil
}

func containsPageExcerpt(page, excerpt string) bool {
	return len(page) > 0 && len(excerpt) > 0 && bytes.Contains([]byte(page), []byte(excerpt))
}

func validEvidenceField(field string) bool {
	switch field {
	case "supplier_name", "supplier_email", "invoice_number", "issue_date", "due_date", "currency", "subtotal", "tax_amount", "total", "line_items":
		return true
	default:
		return false
	}
}
