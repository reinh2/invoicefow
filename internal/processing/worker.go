package processing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/reinhlord/invoiceflow/internal/export"
	"github.com/reinhlord/invoiceflow/internal/extraction"
	"github.com/reinhlord/invoiceflow/internal/invoices"
)

// ProcessStorage is intentionally limited to the read operation a worker
// needs. It keeps storage keys and paths under server control.
type ProcessStorage interface {
	Open(context.Context, string) (io.ReadCloser, error)
}

// WebhookDeliverySender is the narrow worker boundary for a server-owned
// delivery adapter. It keeps the worker independent from URL/secret handling
// and makes durable retry behavior testable without persisting network data.
type WebhookDeliverySender interface {
	Send(context.Context, export.WebhookPayload) export.DeliveryResult
}

type Worker struct {
	Repository    *Repository
	Storage       ProcessStorage
	Text          extraction.TextExtractor
	OCR           extraction.OCR
	Structured    extraction.StructuredExtractor
	Limits        extraction.Limits
	Lease         time.Duration
	RetryDelay    time.Duration
	WebhookSender WebhookDeliverySender

	// OnProviderError optionally reports a structured-extraction provider
	// failure to the operator. The persisted job summary is deliberately
	// generic, which leaves an operator with no way to tell a provider
	// rejection from a quota or transport problem. Only provider errors whose
	// messages are bounded and secret-free by construction (see ADR-014) are
	// passed here; tool output, storage paths, document text, and the API key
	// never reach this hook.
	OnProviderError func(documentID string, err error)

	// OnProcessFinished and OnExportFinished optionally report one finished
	// durable job to an observer. outcome is one of OutcomeSuccess,
	// OutcomeRetry, or OutcomeDeadLetter — server-owned constants, never
	// derived from document content. The duration covers the whole extraction
	// attempt, successful or not. Both hooks are nil in the default wiring, so
	// nothing about job execution depends on an observer being installed.
	OnProcessFinished func(outcome string, duration time.Duration)
	OnExportFinished  func(outcome string)
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
	started := time.Now()
	snapshot, err := w.extract(workCtx, claimed.DocumentID)
	elapsed := time.Since(started)
	if err == nil {
		finishErr := w.Repository.FinishExtraction(ctx, claimed.ID, claimed.LeaseToken, snapshot)
		if finishErr == nil {
			w.reportProcessOutcome(OutcomeSuccess, elapsed)
		}
		return true, finishErr
	}
	w.reportProviderError(claimed.DocumentID, err)
	summary := processingErrorSummary(err)
	if permanentProcessingError(err) {
		finishErr := w.Repository.FinishPermanent(ctx, claimed.ID, claimed.LeaseToken, summary)
		if finishErr == nil {
			// A non-retryable failure dead-letters the job immediately.
			w.reportProcessOutcome(OutcomeDeadLetter, elapsed)
		}
		return true, finishErr
	}
	outcome, finishErr := w.Repository.FinishRetry(ctx, claimed.ID, claimed.LeaseToken, summary, w.RetryDelay)
	if finishErr == nil {
		w.reportProcessOutcome(outcome, elapsed)
	}
	return true, finishErr
}

// reportProcessOutcome and reportExportOutcome report only after the durable
// transition committed, so a counter never claims an outcome the database did
// not record.
func (w Worker) reportProcessOutcome(outcome string, duration time.Duration) {
	if w.OnProcessFinished != nil {
		w.OnProcessFinished(outcome, duration)
	}
}

func (w Worker) reportExportOutcome(outcome string) {
	if w.OnExportFinished != nil {
		w.OnExportFinished(outcome)
	}
}

func (w Worker) RunExportOnce(ctx context.Context) (bool, error) {
	if w.Repository == nil || w.WebhookSender == nil || w.Lease <= 0 {
		return false, fmt.Errorf("incomplete export worker")
	}
	claimed, err := w.Repository.ClaimExportReady(ctx, w.Lease)
	if err != nil || claimed == nil {
		return false, err
	}
	details, err := w.Repository.LoadExportJobDetails(ctx, claimed.ID)
	if err != nil {
		return true, w.Repository.FinishExportPermanent(ctx, claimed.ID, claimed.LeaseToken, "", err.Error())
	}

	payload := buildWebhookPayload(details)

	res := w.WebhookSender.Send(ctx, payload)
	if res.Error == nil {
		finishErr := w.Repository.FinishExportSuccess(ctx, claimed.ID, claimed.LeaseToken, details.ExportID, "system")
		if finishErr == nil {
			w.reportExportOutcome(OutcomeSuccess)
		}
		return true, finishErr
	}

	summary := "webhook delivery failed"
	if res.Retryable {
		outcome, finishErr := w.Repository.FinishExportRetry(ctx, claimed.ID, claimed.LeaseToken, details.ExportID, summary, w.RetryDelay)
		if finishErr == nil {
			w.reportExportOutcome(outcome)
		}
		return true, finishErr
	}
	finishErr := w.Repository.FinishExportPermanent(ctx, claimed.ID, claimed.LeaseToken, details.ExportID, summary)
	if finishErr == nil {
		// A non-retryable delivery failure dead-letters the export immediately.
		w.reportExportOutcome(OutcomeDeadLetter)
	}
	return true, finishErr
}

func buildWebhookPayload(details ExportJobDetails) export.WebhookPayload {
	return export.WebhookPayload{
		Event:          "invoice.exported",
		DocumentID:     details.DocumentID,
		VersionNumber:  details.VersionNumber,
		ApprovedAt:     details.ApprovedAt,
		IdempotencyKey: details.IdempotencyKey,
		Normalized:     details.Normalized,
	}
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
	// The snapshot columns are constrained to JSON arrays. An adapter that
	// asserts no evidence (or a document with no warnings) leaves a nil slice,
	// which would marshal to `null` and violate that constraint, so always
	// encode an empty array.
	warningsJSON, _ := json.Marshal(jsonArray(warnings))
	evidenceJSON, _ := json.Marshal(jsonArray(proposal.Evidence))
	diagnosticsJSON, _ := json.Marshal(jsonArray(proposal.Diagnostics))
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

// jsonArray returns a non-nil slice so encoding yields `[]` rather than `null`.
func jsonArray[T any](values []T) []T {
	if values == nil {
		return []T{}
	}
	return values
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

// reportProviderError forwards only the provider-exchange errors that carry no
// secret by design. Every other failure keeps its generic summary.
func (w Worker) reportProviderError(documentID string, err error) {
	if w.OnProviderError == nil {
		return
	}
	if !errors.Is(err, extraction.ErrOpenAIRequest) && !errors.Is(err, extraction.ErrOpenAIConfiguration) {
		return
	}
	w.OnProviderError(documentID, err)
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

// sanitizeDiagnostics replaces every adapter diagnostic with a server-owned
// code and message. Only codes on this allowlist keep their identity, and even
// those never keep the adapter's own message text.
func sanitizeDiagnostics(diagnostics []extraction.Diagnostic) []extraction.Diagnostic {
	result := make([]extraction.Diagnostic, 0, len(diagnostics))
	for _, diagnostic := range diagnostics {
		switch diagnostic.Code {
		case "fake_fixture_unmatched":
			result = append(result, extraction.Diagnostic{Code: diagnostic.Code, Message: "No configured fictional fixture matched this document."})
		case extraction.HeuristicDiagnosticCode:
			result = append(result, extraction.Diagnostic{Code: diagnostic.Code, Message: "Values were read by the offline heuristic reader. Verify every field against the original."})
		default:
			result = append(result, extraction.Diagnostic{Code: "provider_diagnostic", Message: "The extractor reported a diagnostic."})
		}
	}
	return result
}
