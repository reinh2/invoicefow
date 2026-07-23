package extraction

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidLimits          = errors.New("invalid extraction limits")
	ErrInvalidInput           = errors.New("invalid extraction input")
	ErrInputTooLarge          = errors.New("extraction input exceeds configured limit")
	ErrTooManyPages           = errors.New("extraction input has too many pages")
	ErrReferenceTooLarge      = errors.New("extraction reference text exceeds configured limit")
	ErrProviderOutputTooLarge = errors.New("provider proposal exceeds configured limit")
	ErrTooManyLineItems       = errors.New("provider proposal has too many line items")
	ErrTooManyEvidence        = errors.New("provider proposal has too many evidence entries")
	ErrTooManyDiagnostics     = errors.New("provider proposal has too many diagnostics")
	ErrProcessOutputTooLarge  = errors.New("extraction process output exceeds configured limit")
)

// Limits bounds data supplied to and returned from extraction adapters. Values
// are configured by trusted server code; zero is never interpreted as
// unlimited.
type Limits struct {
	MaxDocumentBytes        int64
	MaxPages                int
	MaxRasterDimension      int
	MaxRasterPixels         int64
	MaxReferenceTextBytes   int
	MaxProviderOutputBytes  int
	MaxEvidenceExcerptBytes int
	MaxLineItems            int
	MaxEvidence             int
	MaxDiagnostics          int
	MaxProcessOutputBytes   int
	PDFTimeout              time.Duration
	OCRTimeout              time.Duration
}

// Validate rejects incomplete or internally inconsistent server limits.
func (l Limits) Validate() error {
	if l.MaxDocumentBytes <= 0 || l.MaxPages <= 0 || l.MaxRasterDimension <= 0 || l.MaxRasterPixels <= 0 || l.MaxReferenceTextBytes <= 0 || l.MaxProviderOutputBytes <= 0 || l.MaxEvidenceExcerptBytes <= 0 || l.MaxLineItems <= 0 || l.MaxEvidence <= 0 || l.MaxDiagnostics <= 0 || l.MaxProcessOutputBytes <= 0 || l.PDFTimeout <= 0 || l.OCRTimeout <= 0 {
		return ErrInvalidLimits
	}
	if l.MaxEvidenceExcerptBytes > l.MaxReferenceTextBytes {
		return fmt.Errorf("%w: evidence excerpt limit exceeds reference text limit", ErrInvalidLimits)
	}
	return nil
}

// DefaultLimits are deliberately finite Stage 3 server limits. They are not
// request-controlled and are documented in ADR-006.
func DefaultLimits() Limits {
	return Limits{
		MaxDocumentBytes:        20 << 20,
		MaxPages:                50,
		MaxRasterDimension:      10_000,
		MaxRasterPixels:         40_000_000,
		MaxReferenceTextBytes:   256 << 10,
		MaxProviderOutputBytes:  64 << 10,
		MaxEvidenceExcerptBytes: 512,
		MaxLineItems:            200,
		MaxEvidence:             100,
		MaxDiagnostics:          20,
		MaxProcessOutputBytes:   512 << 10,
		PDFTimeout:              15 * time.Second,
		OCRTimeout:              30 * time.Second,
	}
}

// ValidateDocumentInput validates trusted metadata before a text/OCR adapter
// consumes its stream. It deliberately does not read the stream.
func (l Limits) ValidateDocumentInput(input DocumentInput) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if !validSHA256(input.SHA256) || input.MediaType == "" || input.SizeBytes < 0 || input.Reader == nil {
		return ErrInvalidInput
	}
	if input.SizeBytes > l.MaxDocumentBytes {
		return ErrInputTooLarge
	}
	return nil
}

// ValidateStructuredInput bounds and labels the hostile reference text passed
// to a structured extractor. Pages must be strictly increasing and one-based
// so evidence can later refer back to a stable source page.
func (l Limits) ValidateStructuredInput(input StructuredExtractionInput) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if !validSHA256(input.DocumentSHA256) {
		return ErrInvalidInput
	}
	return l.ValidatePageText(input.ReferenceText)
}

// ValidatePageText applies source page and reference-text bounds to text or
// OCR adapter output before it is considered for a structured request.
func (l Limits) ValidatePageText(pages []PageText) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if len(pages) > l.MaxPages {
		return ErrTooManyPages
	}

	textBytes := 0
	previousPage := 0
	for _, page := range pages {
		if page.PageNumber <= previousPage {
			return ErrInvalidInput
		}
		previousPage = page.PageNumber
		textBytes += len(page.Text)
		if textBytes > l.MaxReferenceTextBytes {
			return ErrReferenceTooLarge
		}
	}
	return nil
}

// ValidateProposal applies byte and evidence bounds to an adapter result. It
// does not make business decisions about dates, currency, tax, totals, or
// completeness; those are server-side normalization responsibilities.
func (l Limits) ValidateProposal(proposal Proposal) error {
	if err := l.Validate(); err != nil {
		return err
	}
	if len(proposal.LineItems) > l.MaxLineItems {
		return ErrTooManyLineItems
	}
	if len(proposal.Evidence) > l.MaxEvidence {
		return ErrTooManyEvidence
	}
	if len(proposal.Diagnostics) > l.MaxDiagnostics {
		return ErrTooManyDiagnostics
	}

	outputBytes := 0
	add := func(value *string) bool {
		if value != nil {
			outputBytes += len(*value)
		}
		return outputBytes <= l.MaxProviderOutputBytes
	}
	for _, value := range []*string{
		proposal.SupplierName,
		proposal.SupplierEmail,
		proposal.InvoiceNumber,
		proposal.IssueDate,
		proposal.DueDate,
		proposal.Currency,
		proposal.Subtotal,
		proposal.TaxAmount,
		proposal.Total,
	} {
		if !add(value) {
			return ErrProviderOutputTooLarge
		}
	}
	for _, item := range proposal.LineItems {
		for _, value := range []*string{item.Description, item.Quantity, item.UnitPrice, item.TaxAmount, item.Total} {
			if !add(value) {
				return ErrProviderOutputTooLarge
			}
		}
	}
	for _, evidence := range proposal.Evidence {
		if evidence.Field == "" || evidence.PageNumber <= 0 || len(evidence.Excerpt) > l.MaxEvidenceExcerptBytes {
			return ErrInvalidInput
		}
		if evidence.BoundingBox != nil && (evidence.BoundingBox.Left > evidence.BoundingBox.Right || evidence.BoundingBox.Top > evidence.BoundingBox.Bottom) {
			return ErrInvalidInput
		}
		outputBytes += len(evidence.Excerpt)
		if outputBytes > l.MaxProviderOutputBytes {
			return ErrProviderOutputTooLarge
		}
	}
	for _, diagnostic := range proposal.Diagnostics {
		outputBytes += len(diagnostic.Code) + len(diagnostic.Message)
		if outputBytes > l.MaxProviderOutputBytes {
			return ErrProviderOutputTooLarge
		}
	}
	return nil
}

func validSHA256(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, r := range value {
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}
