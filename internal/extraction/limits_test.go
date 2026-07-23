package extraction

import (
	"errors"
	"io"
	"strings"
	"testing"
	"time"
)

func TestLimitsValidate(t *testing.T) {
	t.Parallel()

	if err := testLimits().Validate(); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}

	invalid := testLimits()
	invalid.MaxPages = 0
	if !errors.Is(invalid.Validate(), ErrInvalidLimits) {
		t.Fatalf("Validate() error = %v, want ErrInvalidLimits", invalid.Validate())
	}

	invalid = testLimits()
	invalid.MaxEvidenceExcerptBytes = invalid.MaxReferenceTextBytes + 1
	if !errors.Is(invalid.Validate(), ErrInvalidLimits) {
		t.Fatalf("Validate() error = %v, want ErrInvalidLimits", invalid.Validate())
	}

	invalid = testLimits()
	invalid.MaxDiagnostics = 0
	if !errors.Is(invalid.Validate(), ErrInvalidLimits) {
		t.Fatalf("Validate() error = %v, want ErrInvalidLimits", invalid.Validate())
	}
}

func TestLimitsValidateDocumentInput(t *testing.T) {
	t.Parallel()

	input := DocumentInput{
		SHA256:    testHash,
		MediaType: "application/pdf",
		SizeBytes: 12,
		Reader:    io.NopCloser(strings.NewReader("fictional")),
	}
	if err := testLimits().ValidateDocumentInput(input); err != nil {
		t.Fatalf("ValidateDocumentInput() error = %v", err)
	}

	input.SizeBytes = 101
	if !errors.Is(testLimits().ValidateDocumentInput(input), ErrInputTooLarge) {
		t.Fatalf("ValidateDocumentInput() error = %v, want ErrInputTooLarge", testLimits().ValidateDocumentInput(input))
	}

	input = DocumentInput{SHA256: "not-a-hash", MediaType: "application/pdf", Reader: io.NopCloser(strings.NewReader("x"))}
	if !errors.Is(testLimits().ValidateDocumentInput(input), ErrInvalidInput) {
		t.Fatalf("ValidateDocumentInput() error = %v, want ErrInvalidInput", testLimits().ValidateDocumentInput(input))
	}
}

func TestLimitsValidateStructuredInput(t *testing.T) {
	t.Parallel()

	input := StructuredExtractionInput{
		DocumentSHA256: testHash,
		ReferenceText:  []PageText{{PageNumber: 1, Text: "one"}, {PageNumber: 2, Text: "two"}},
	}
	if err := testLimits().ValidateStructuredInput(input); err != nil {
		t.Fatalf("ValidateStructuredInput() error = %v", err)
	}

	input.ReferenceText[1].PageNumber = 1
	if !errors.Is(testLimits().ValidateStructuredInput(input), ErrInvalidInput) {
		t.Fatalf("duplicate page error = %v, want ErrInvalidInput", testLimits().ValidateStructuredInput(input))
	}

	input = StructuredExtractionInput{DocumentSHA256: testHash, ReferenceText: []PageText{{PageNumber: 1, Text: strings.Repeat("x", 101)}}}
	if !errors.Is(testLimits().ValidateStructuredInput(input), ErrReferenceTooLarge) {
		t.Fatalf("oversized reference error = %v, want ErrReferenceTooLarge", testLimits().ValidateStructuredInput(input))
	}

	input = StructuredExtractionInput{DocumentSHA256: strings.Repeat("A", 64)}
	if !errors.Is(testLimits().ValidateStructuredInput(input), ErrInvalidInput) {
		t.Fatalf("non-canonical hash error = %v, want ErrInvalidInput", testLimits().ValidateStructuredInput(input))
	}
}

func TestLimitsValidateProposal(t *testing.T) {
	t.Parallel()

	name := "Fictional Supplies"
	if err := testLimits().ValidateProposal(Proposal{SupplierName: &name, Evidence: []Evidence{{Field: "supplier_name", PageNumber: 1, Excerpt: "source"}}}); err != nil {
		t.Fatalf("ValidateProposal() error = %v", err)
	}

	overlong := strings.Repeat("x", 101)
	if !errors.Is(testLimits().ValidateProposal(Proposal{SupplierName: &overlong}), ErrProviderOutputTooLarge) {
		t.Fatalf("oversized proposal error = %v, want ErrProviderOutputTooLarge", testLimits().ValidateProposal(Proposal{SupplierName: &overlong}))
	}

	if !errors.Is(testLimits().ValidateProposal(Proposal{Evidence: []Evidence{{PageNumber: 0, Excerpt: "source"}}}), ErrInvalidInput) {
		t.Fatalf("invalid evidence error = %v, want ErrInvalidInput", testLimits().ValidateProposal(Proposal{Evidence: []Evidence{{PageNumber: 0, Excerpt: "source"}}}))
	}

	if !errors.Is(testLimits().ValidateProposal(Proposal{LineItems: make([]LineItemProposal, 3)}), ErrTooManyLineItems) {
		t.Fatalf("too many line items error = %v, want ErrTooManyLineItems", testLimits().ValidateProposal(Proposal{LineItems: make([]LineItemProposal, 3)}))
	}
}

const testHash = "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"

func testLimits() Limits {
	return Limits{
		MaxDocumentBytes:        100,
		MaxPages:                2,
		MaxRasterDimension:      10,
		MaxRasterPixels:         100,
		MaxReferenceTextBytes:   100,
		MaxProviderOutputBytes:  100,
		MaxEvidenceExcerptBytes: 20,
		MaxLineItems:            2,
		MaxEvidence:             2,
		MaxDiagnostics:          2,
		MaxProcessOutputBytes:   100,
		PDFTimeout:              time.Second,
		OCRTimeout:              time.Second,
	}
}
