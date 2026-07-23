package extraction

import (
	"errors"
	"testing"
)

func TestDecodeProposalJSONRejectsUnknownFields(t *testing.T) {
	_, err := DecodeProposalJSON([]byte(`{"supplier_name":"Fictional","approval":"yes"}`), testLimits())
	if !errors.Is(err, ErrInvalidProposalSchema) {
		t.Fatalf("DecodeProposalJSON() error = %v, want ErrInvalidProposalSchema", err)
	}
	proposal, err := DecodeProposalJSON([]byte(`{"supplier_name":"Fictional","line_items":[]}`), testLimits())
	if err != nil || proposal.SupplierName == nil {
		t.Fatalf("DecodeProposalJSON() = %#v, %v", proposal, err)
	}
}

func TestValidateEvidenceRequiresExactSourceExcerptAndKnownField(t *testing.T) {
	proposal := Proposal{Evidence: []Evidence{{Field: "total", PageNumber: 1, Excerpt: "TOTAL 24.00"}}}
	pages := []PageText{{PageNumber: 1, Text: "Fictional\nTOTAL 24.00"}}
	if err := ValidateEvidence(proposal, pages, testLimits()); err != nil {
		t.Fatalf("ValidateEvidence() error = %v", err)
	}
	proposal.Evidence[0].Excerpt = "invented"
	if !errors.Is(ValidateEvidence(proposal, pages, testLimits()), ErrInvalidInput) {
		t.Fatal("invented excerpt was accepted")
	}
}

func TestParsePDFInfoAndSplitPageText(t *testing.T) {
	pages, encrypted, ok := parsePDFInfo("Pages:          2\nEncrypted:      no\n")
	if !ok || encrypted || pages != 2 {
		t.Fatalf("parsePDFInfo = %d, %v, %v", pages, encrypted, ok)
	}
	pages, encrypted, ok = parsePDFInfo("Pages: 1\nEncrypted: yes (print:no)\n")
	if !ok || !encrypted || pages != 1 {
		t.Fatalf("encrypted parse = %d, %v, %v", pages, encrypted, ok)
	}
	text := splitPageText("one\ftwo\f", 2)
	if len(text) != 2 || text[0].PageNumber != 1 || text[1].Text != "two" {
		t.Fatalf("splitPageText = %#v", text)
	}
}
