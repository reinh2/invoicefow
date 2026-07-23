package processing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/reinhlord/invoiceflow/internal/extraction"
	"github.com/reinhlord/invoiceflow/internal/invoices"
)

// ProcessStorage is intentionally limited to the read operation a worker
// needs. It keeps storage keys and paths under server control.
type ProcessStorage interface {
	Open(context.Context, string) (io.ReadCloser, error)
}

type Worker struct {
	Repository *Repository
	Storage    ProcessStorage
	Text       extraction.TextExtractor
	OCR        extraction.OCR
	Structured extraction.StructuredExtractor
	Limits     extraction.Limits
	Lease      time.Duration
	RetryDelay time.Duration
}

// RunOnce claims at most one durable process job and records either an
// immutable extraction snapshot, a bounded retry, or a permanent failure.
func (w Worker) RunOnce(ctx context.Context) (bool, error) {
	if err := w.valid(); err != nil {
		return false, err
	}
	claimed, err := w.Repository.ClaimReady(ctx, w.Lease)
	if err != nil || claimed == nil {
		return false, err
	}
	workCtx, cancel := context.WithTimeout(ctx, maxDuration(w.Limits.PDFTimeout, w.Limits.OCRTimeout)+5*time.Second)
	defer cancel()
	snapshot, err := w.extract(workCtx, claimed.DocumentID)
	if err == nil {
		return true, w.Repository.FinishExtraction(ctx, claimed.ID, claimed.LeaseToken, snapshot)
	}
	summary := processingErrorSummary(err)
	if permanentProcessingError(err) {
		return true, w.Repository.FinishPermanent(ctx, claimed.ID, claimed.LeaseToken, summary)
	}
	return true, w.Repository.FinishRetry(ctx, claimed.ID, claimed.LeaseToken, summary, w.RetryDelay)
}

func (w Worker) valid() error {
	if w.Repository == nil || w.Storage == nil || w.Text == nil || w.OCR == nil || w.Structured == nil || w.Lease <= 0 || w.RetryDelay < 0 {
		return fmt.Errorf("incomplete process worker")
	}
	return w.Limits.Validate()
}

func (w Worker) extract(ctx context.Context, documentID string) (ExtractionSnapshot, error) {
	document, err := w.Repository.LoadProcessDocument(ctx, documentID)
	if err != nil {
		return ExtractionSnapshot{}, err
	}
	pages, err := w.text(ctx, document)
	if err != nil {
		return ExtractionSnapshot{}, err
	}
	if !hasUsableText(pages) {
		pages, err = w.ocr(ctx, document)
		if err != nil {
			return ExtractionSnapshot{}, err
		}
	}
	if err = w.Limits.ValidatePageText(pages); err != nil {
		return ExtractionSnapshot{}, err
	}
	proposal, err := w.Structured.Extract(ctx, extraction.StructuredExtractionInput{DocumentSHA256: document.SHA256, ReferenceText: pages}, w.Limits)
	if err != nil {
		return ExtractionSnapshot{}, err
	}
	if err = extraction.ValidateEvidence(proposal, pages, w.Limits); err != nil {
		return ExtractionSnapshot{}, err
	}
	proposal.Diagnostics = sanitizeDiagnostics(proposal.Diagnostics)
	normalized, warnings := invoices.NormalizeProposal(proposal)
	proposalJSON, _ := json.Marshal(proposal)
	normalizedJSON, _ := json.Marshal(normalized)
	warningsJSON, _ := json.Marshal(warnings)
	evidenceJSON, _ := json.Marshal(proposal.Evidence)
	diagnosticsJSON, _ := json.Marshal(proposal.Diagnostics)
	return ExtractionSnapshot{Currency: normalized.Currency, TotalMinor: normalized.Total, RoundingPolicyVersion: normalized.RoundingPolicyVersion, Proposal: proposalJSON, Normalized: normalizedJSON, Warnings: warningsJSON, Evidence: evidenceJSON, Diagnostics: diagnosticsJSON}, nil
}

func (w Worker) text(ctx context.Context, document ProcessDocument) ([]extraction.PageText, error) {
	reader, err := w.Storage.Open(ctx, document.ObjectKey)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	result, err := w.Text.ExtractText(ctx, extraction.DocumentInput{SHA256: document.SHA256, MediaType: document.MediaType, SizeBytes: document.SizeBytes, Reader: reader}, w.Limits)
	return result.Pages, err
}
func (w Worker) ocr(ctx context.Context, document ProcessDocument) ([]extraction.PageText, error) {
	reader, err := w.Storage.Open(ctx, document.ObjectKey)
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	result, err := w.OCR.ExtractOCR(ctx, extraction.DocumentInput{SHA256: document.SHA256, MediaType: document.MediaType, SizeBytes: document.SizeBytes, Reader: reader}, w.Limits)
	return result.Pages, err
}
func hasUsableText(pages []extraction.PageText) bool {
	for _, page := range pages {
		if strings.TrimSpace(page.Text) != "" {
			return true
		}
	}
	return false
}
func maxDuration(left, right time.Duration) time.Duration {
	if left > right {
		return left
	}
	return right
}

func permanentProcessingError(err error) bool {
	return errors.Is(err, extraction.ErrMalformedPDF) || errors.Is(err, extraction.ErrEncryptedPDF) || errors.Is(err, extraction.ErrInputTooLarge) || errors.Is(err, extraction.ErrTooManyPages) || errors.Is(err, extraction.ErrReferenceTooLarge) || errors.Is(err, extraction.ErrProcessOutputTooLarge) || errors.Is(err, extraction.ErrProviderOutputTooLarge) || errors.Is(err, extraction.ErrTooManyLineItems) || errors.Is(err, extraction.ErrTooManyEvidence) || errors.Is(err, extraction.ErrTooManyDiagnostics) || errors.Is(err, extraction.ErrInvalidProposalSchema) || errors.Is(err, extraction.ErrInvalidInput) || errors.Is(err, extraction.ErrUnsupportedOCR)
}
func processingErrorSummary(err error) string {
	switch {
	case errors.Is(err, extraction.ErrMalformedPDF):
		return "document PDF could not be parsed"
	case errors.Is(err, extraction.ErrEncryptedPDF):
		return "encrypted PDFs are not supported"
	case errors.Is(err, extraction.ErrTooManyPages):
		return "document exceeds page limit"
	case errors.Is(err, extraction.ErrProcessOutputTooLarge):
		return "extraction exceeded output limit"
	case errors.Is(err, extraction.ErrUnsupportedOCR):
		return "OCR is unavailable for this document type"
	case errors.Is(err, context.DeadlineExceeded):
		return "processing timed out"
	default:
		return "processing failed; retry scheduled"
	}
}
func sanitizeDiagnostics(diagnostics []extraction.Diagnostic) []extraction.Diagnostic {
	result := make([]extraction.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		if diagnostic.Code == "fake_fixture_unmatched" {
			result = append(result, extraction.Diagnostic{Code: diagnostic.Code, Message: "No configured fictional fixture matched this document."})
			continue
		}
		result = append(result, extraction.Diagnostic{Code: "provider_diagnostic", Message: "The extractor reported a diagnostic."})
	}
	return result
}
