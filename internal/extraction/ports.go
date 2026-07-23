package extraction

import "context"

// TextExtractor extracts embedded, text-based content from a document. It
// does not select OCR fallback, normalize values, or mutate application state.
type TextExtractor interface {
	ExtractText(ctx context.Context, document DocumentInput, limits Limits) (TextExtractionResult, error)
}

// OCR extracts text from an image or a document page raster. It has the same
// intentionally narrow boundary as TextExtractor.
type OCR interface {
	ExtractOCR(ctx context.Context, document DocumentInput, limits Limits) (OCRResult, error)
}

// StructuredExtractor produces an untrusted proposal from bounded,
// page-labelled reference text. It cannot receive or return workflow control
// data.
type StructuredExtractor interface {
	Extract(ctx context.Context, input StructuredExtractionInput, limits Limits) (Proposal, error)
}
