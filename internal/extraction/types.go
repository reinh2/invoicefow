// Package extraction defines the narrow, proposal-only boundary between
// document-processing adapters and InvoiceFlow's server-owned workflow.
//
// All values crossing this boundary are untrusted reference data. Callers
// must validate, normalize, warn on, and persist proposals before any review
// or workflow state changes are made.
package extraction

// PageText is text associated with one one-based source page. Text is raw
// reference material, not an instruction to the application.
type PageText struct {
	PageNumber int
	Text       string
}

// Evidence is optional source context supplied by an adapter. It is omitted
// when the adapter cannot provide it; callers must never synthesize it.
type Evidence struct {
	Field       string       `json:"field"`
	PageNumber  int          `json:"page_number"`
	Excerpt     string       `json:"excerpt"`
	BoundingBox *BoundingBox `json:"bounding_box,omitempty"`
}

// BoundingBox identifies a source region in adapter-defined page coordinates.
// It deliberately carries no claim about a rendering coordinate system.
type BoundingBox struct {
	Left   int `json:"left"`
	Top    int `json:"top"`
	Right  int `json:"right"`
	Bottom int `json:"bottom"`
}

// Proposal contains only raw candidate invoice values. Empty or nil fields are
// allowed: a proposal is not a validated invoice and has no authority to alter
// document identity, workflow state, approvals, exports, retries, or secrets.
//
// Dates and monetary values remain strings until server-side normalization.
// In particular, this package intentionally has no floating-point values or
// confidence score.
type Proposal struct {
	SupplierName  *string            `json:"supplier_name"`
	SupplierEmail *string            `json:"supplier_email"`
	InvoiceNumber *string            `json:"invoice_number"`
	IssueDate     *string            `json:"issue_date"`
	DueDate       *string            `json:"due_date"`
	Currency      *string            `json:"currency"`
	Subtotal      *string            `json:"subtotal"`
	TaxAmount     *string            `json:"tax_amount"`
	Total         *string            `json:"total"`
	LineItems     []LineItemProposal `json:"line_items"`
	Evidence      []Evidence         `json:"evidence"`
	Diagnostics   []Diagnostic       `json:"diagnostics"`
}

// LineItemProposal is an unvalidated candidate line item.
type LineItemProposal struct {
	Description *string `json:"description"`
	Quantity    *string `json:"quantity"`
	UnitPrice   *string `json:"unit_price"`
	TaxAmount   *string `json:"tax_amount"`
	Total       *string `json:"total"`
}

// Diagnostic is a sanitized, bounded adapter diagnostic. It must not contain
// credentials, filesystem paths, or raw provider responses.
type Diagnostic struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

// DocumentInput identifies server-owned document content supplied to text and
// OCR adapters. SHA256 and SizeBytes must be computed by trusted server code
// before an adapter is called. Reader is intentionally opaque: adapters must
// apply the supplied Limits while reading it.
type DocumentInput struct {
	SHA256    string
	MediaType string
	SizeBytes int64
	Reader    ReadCloser
}

// ReadCloser keeps this package independent from a concrete storage adapter
// while allowing a processing adapter to release a server-owned work stream.
type ReadCloser interface {
	Read(p []byte) (n int, err error)
	Close() error
}

// TextExtractionResult and OCRResult contain raw, page-labelled reference
// text. Their callers select sufficiency and any fallback policy.
type TextExtractionResult struct {
	Pages []PageText
}

type OCRResult struct {
	Pages []PageText
}

// StructuredExtractionInput is the only input available to a structured
// extractor. It exposes neither document paths nor workflow authority.
type StructuredExtractionInput struct {
	DocumentSHA256 string
	ReferenceText  []PageText
}
