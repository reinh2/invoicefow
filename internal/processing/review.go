package processing

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reinhlord/invoiceflow/internal/export"
	"github.com/reinhlord/invoiceflow/internal/extraction"
	"github.com/reinhlord/invoiceflow/internal/invoices"
)

var (
	ErrDocumentNotFound       = errors.New("document not found")
	ErrInvalidDocumentState   = errors.New("invalid document transition")
	ErrStaleReviewVersion     = errors.New("stale review version")
	ErrInvalidHumanReviewEdit = errors.New("invalid human review edit")
	ErrInvalidApproval        = errors.New("invalid approval request")
	ErrInvalidExport          = errors.New("invalid export request")
	ErrWebhookNotConfigured   = errors.New("webhook destination URL is not configured on server")
)

type ExportRecord struct {
	ID               string     `json:"id"`
	DocumentID       string     `json:"document_id"`
	VersionNumber    int        `json:"version_number"`
	ExportType       string     `json:"export_type"`
	Status           string     `json:"status"`
	IdempotencyKey   string     `json:"idempotency_key"`
	DestinationRef   string     `json:"destination_ref"`
	DestinationLabel string     `json:"destination_label"`
	Attempts         int        `json:"attempts"`
	NextAttemptAt    *time.Time `json:"next_attempt_at,omitempty"`
	ErrorSummary     *string    `json:"error_summary,omitempty"`
	CreatedAt        time.Time  `json:"created_at"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

const maxReviewDetailItems = 100

// HumanReviewInput is the narrow candidate shape accepted from a human review
// form. It deliberately has no evidence, diagnostics, state, actor, or other
// authority-bearing fields.
type HumanReviewInput struct {
	SupplierName  *string         `json:"supplier_name"`
	SupplierEmail *string         `json:"supplier_email"`
	InvoiceNumber *string         `json:"invoice_number"`
	IssueDate     *string         `json:"issue_date"`
	DueDate       *string         `json:"due_date"`
	Currency      *string         `json:"currency"`
	Subtotal      *string         `json:"subtotal"`
	TaxAmount     *string         `json:"tax_amount"`
	Total         *string         `json:"total"`
	LineItems     []HumanLineItem `json:"line_items"`
}

type HumanLineItem struct {
	Description *string `json:"description"`
	Quantity    *string `json:"quantity"`
	UnitPrice   *string `json:"unit_price"`
	TaxAmount   *string `json:"tax_amount"`
	Total       *string `json:"total"`
}

func DecodeHumanReviewInput(raw []byte) (HumanReviewInput, error) {
	if len(raw) == 0 || len(raw) > extraction.DefaultLimits().MaxProviderOutputBytes {
		return HumanReviewInput{}, ErrInvalidHumanReviewEdit
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var input HumanReviewInput
	if err := decoder.Decode(&input); err != nil {
		return HumanReviewInput{}, ErrInvalidHumanReviewEdit
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return HumanReviewInput{}, ErrInvalidHumanReviewEdit
	}
	proposal := input.proposal()
	if err := extraction.DefaultLimits().ValidateProposal(proposal); err != nil {
		return HumanReviewInput{}, ErrInvalidHumanReviewEdit
	}
	return input, nil
}

func (input HumanReviewInput) proposal() extraction.Proposal {
	items := make([]extraction.LineItemProposal, len(input.LineItems))
	for index, item := range input.LineItems {
		items[index] = extraction.LineItemProposal{Description: item.Description, Quantity: item.Quantity, UnitPrice: item.UnitPrice, TaxAmount: item.TaxAmount, Total: item.Total}
	}
	return extraction.Proposal{SupplierName: input.SupplierName, SupplierEmail: input.SupplierEmail, InvoiceNumber: input.InvoiceNumber, IssueDate: input.IssueDate, DueDate: input.DueDate, Currency: input.Currency, Subtotal: input.Subtotal, TaxAmount: input.TaxAmount, Total: input.Total, LineItems: items}
}

type ReviewVersion struct {
	VersionNumber         int              `json:"version_number"`
	Source                string           `json:"source"`
	CreatedAt             time.Time        `json:"created_at"`
	Proposal              json.RawMessage  `json:"proposal"`
	Normalized            json.RawMessage  `json:"normalized"`
	Warnings              json.RawMessage  `json:"warnings"`
	Evidence              json.RawMessage  `json:"evidence"`
	Diagnostics           json.RawMessage  `json:"diagnostics"`
	RoundingPolicyVersion string           `json:"rounding_policy_version"`
	Editable              EditableProposal `json:"editable"`
}

type EditableProposal struct {
	SupplierName  string             `json:"supplier_name"`
	SupplierEmail string             `json:"supplier_email"`
	InvoiceNumber string             `json:"invoice_number"`
	IssueDate     string             `json:"issue_date"`
	DueDate       string             `json:"due_date"`
	Currency      string             `json:"currency"`
	Subtotal      string             `json:"subtotal"`
	TaxAmount     string             `json:"tax_amount"`
	Total         string             `json:"total"`
	LineItems     []EditableLineItem `json:"line_items"`
}

type EditableLineItem struct {
	Description string `json:"description"`
	Quantity    string `json:"quantity"`
	UnitPrice   string `json:"unit_price"`
	TaxAmount   string `json:"tax_amount"`
	Total       string `json:"total"`
}

type ReviewAuditEvent struct {
	Sequence   int64           `json:"sequence"`
	Action     string          `json:"action"`
	Actor      string          `json:"actor"`
	Payload    json.RawMessage `json:"payload"`
	OccurredAt time.Time       `json:"occurred_at"`
}

type ReviewDocument struct {
	ID                    string             `json:"id"`
	Status                string             `json:"status"`
	CreatedAt             time.Time          `json:"created_at"`
	UpdatedAt             time.Time          `json:"updated_at"`
	MediaType             string             `json:"media_type"`
	ApprovedVersionNumber *int               `json:"approved_version_number,omitempty"`
	ApprovedAt            *time.Time         `json:"approved_at,omitempty"`
	Versions              []ReviewVersion    `json:"versions"`
	Audit                 []ReviewAuditEvent `json:"audit"`
	Exports               []ExportRecord     `json:"exports,omitempty"`
}

type SourceDocument struct{ ObjectKey, MediaType string }

// DocumentSummary is the presentation-safe row of a document list. It carries
// no storage key, filesystem path, SHA-256, object id, or internal version id —
// only what an operator needs to recognize a document and open it.
type DocumentSummary struct {
	ID            string    `json:"id"`
	Status        string    `json:"status"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	SupplierName  string    `json:"supplier_name,omitempty"`
	InvoiceNumber string    `json:"invoice_number,omitempty"`
	Currency      string    `json:"currency,omitempty"`
	TotalMinor    *int64    `json:"total_minor,omitempty"`
	VersionNumber *int      `json:"version_number,omitempty"`
}

// DocumentPage is one bounded page of the document list. NextCursor is empty
// when the last page has been reached.
type DocumentPage struct {
	Documents  []DocumentSummary `json:"documents"`
	NextCursor string            `json:"next_cursor,omitempty"`
}

const (
	defaultDocumentPageSize = 20
	maxDocumentPageSize     = 100
)

// ErrInvalidCursor means a list cursor was not one this server issued.
var ErrInvalidCursor = errors.New("invalid pagination cursor")

// ListDocuments returns one page of documents, newest first.
//
// Pagination is keyset rather than offset: the cursor carries the (created_at,
// id) of the last row returned, so inserting a document while an operator pages
// through the list cannot make a row repeat or disappear. The page size is
// clamped server-side; a client cannot ask for an unbounded scan.
//
// The summary values come from the document's newest version, which is a
// proposal under review, not authoritative financial data.
func (r *Repository) ListDocuments(ctx context.Context, pageSize int, cursor string) (DocumentPage, error) {
	if pageSize <= 0 {
		pageSize = defaultDocumentPageSize
	}
	if pageSize > maxDocumentPageSize {
		pageSize = maxDocumentPageSize
	}
	cursorTime, cursorID, err := decodeDocumentCursor(cursor)
	if err != nil {
		return DocumentPage{}, err
	}

	// One extra row is requested purely to learn whether another page exists,
	// and is never returned to the client.
	rows, err := r.pool.Query(ctx, `
SELECT d.id, d.status, d.created_at, d.updated_at,
       v.version_number, v.currency, v.total_minor,
       v.normalized->>'supplier_name', v.normalized->>'invoice_number'
FROM documents d
LEFT JOIN LATERAL (
    SELECT version_number, currency, total_minor, normalized
    FROM invoice_versions
    WHERE document_id = d.id
    ORDER BY version_number DESC
    LIMIT 1
) v ON true
WHERE ($1::timestamptz IS NULL OR (d.created_at, d.id) < ($1::timestamptz, $2::uuid))
ORDER BY d.created_at DESC, d.id DESC
LIMIT $3`, cursorTime, cursorID, pageSize+1)
	if err != nil {
		return DocumentPage{}, err
	}
	defer rows.Close()

	page := DocumentPage{Documents: make([]DocumentSummary, 0, pageSize)}
	for rows.Next() {
		var summary DocumentSummary
		var versionNumber *int
		var currency, supplier, invoiceNumber *string
		var totalMinor *int64
		if err := rows.Scan(&summary.ID, &summary.Status, &summary.CreatedAt, &summary.UpdatedAt,
			&versionNumber, &currency, &totalMinor, &supplier, &invoiceNumber); err != nil {
			return DocumentPage{}, err
		}
		summary.VersionNumber = versionNumber
		summary.TotalMinor = totalMinor
		summary.Currency = stringValue(currency)
		summary.SupplierName = stringValue(supplier)
		summary.InvoiceNumber = stringValue(invoiceNumber)
		page.Documents = append(page.Documents, summary)
	}
	if err := rows.Err(); err != nil {
		return DocumentPage{}, err
	}

	if len(page.Documents) > pageSize {
		last := page.Documents[pageSize-1]
		page.Documents = page.Documents[:pageSize]
		page.NextCursor = encodeDocumentCursor(last.CreatedAt, last.ID)
	}
	return page, nil
}

// validUUID accepts only the canonical lowercase UUID text form, so a cursor
// cannot smuggle arbitrary text into a parameterized comparison.
func validUUID(value string) bool {
	if len(value) != 36 {
		return false
	}
	for index, r := range value {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if r != '-' {
				return false
			}
			continue
		}
		if !(r >= '0' && r <= '9') && !(r >= 'a' && r <= 'f') {
			return false
		}
	}
	return true
}

func stringValue(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

// encodeDocumentCursor produces an opaque cursor. It is deliberately not a
// client-meaningful value: callers must pass back exactly what they received.
func encodeDocumentCursor(createdAt time.Time, id string) string {
	return base64.RawURLEncoding.EncodeToString([]byte(createdAt.UTC().Format(time.RFC3339Nano) + "|" + id))
}

// decodeDocumentCursor accepts only a cursor this server could have issued. A
// malformed cursor is a client error, never a silent fall back to the first
// page, which would otherwise loop a paging client forever.
func decodeDocumentCursor(cursor string) (*time.Time, *string, error) {
	if cursor == "" {
		return nil, nil, nil
	}
	raw, err := base64.RawURLEncoding.DecodeString(cursor)
	if err != nil {
		return nil, nil, ErrInvalidCursor
	}
	timestamp, id, found := strings.Cut(string(raw), "|")
	if !found || !validUUID(id) {
		return nil, nil, ErrInvalidCursor
	}
	parsed, err := time.Parse(time.RFC3339Nano, timestamp)
	if err != nil {
		return nil, nil, ErrInvalidCursor
	}
	return &parsed, &id, nil
}

// GetReviewDocument returns a bounded, presentation-safe view. Server storage
// keys and hashes remain internal and source bytes require a separate stream.
func (r *Repository) GetReviewDocument(ctx context.Context, documentID string) (ReviewDocument, error) {
	var detail ReviewDocument
	err := r.pool.QueryRow(ctx, `SELECT d.id,d.status,d.created_at,d.updated_at,o.media_type,d.approved_at,v.version_number FROM documents d JOIN stored_objects o ON o.id=d.object_id LEFT JOIN invoice_versions v ON v.id=d.approved_version_id WHERE d.id=$1`, documentID).Scan(&detail.ID, &detail.Status, &detail.CreatedAt, &detail.UpdatedAt, &detail.MediaType, &detail.ApprovedAt, &detail.ApprovedVersionNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return ReviewDocument{}, ErrDocumentNotFound
	}
	if err != nil {
		return ReviewDocument{}, err
	}
	rows, err := r.pool.Query(ctx, `SELECT version_number,source,created_at,proposal,normalized,warnings,evidence,diagnostics,rounding_policy_version FROM invoice_versions WHERE document_id=$1 ORDER BY version_number DESC LIMIT $2`, documentID, maxReviewDetailItems)
	if err != nil {
		return ReviewDocument{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var version ReviewVersion
		if err := rows.Scan(&version.VersionNumber, &version.Source, &version.CreatedAt, &version.Proposal, &version.Normalized, &version.Warnings, &version.Evidence, &version.Diagnostics, &version.RoundingPolicyVersion); err != nil {
			return ReviewDocument{}, err
		}
		editable, err := editableFromNormalized(version.Normalized)
		if err != nil {
			return ReviewDocument{}, fmt.Errorf("decode normalized review snapshot: %w", err)
		}
		version.Editable = editable
		detail.Versions = append(detail.Versions, version)
	}
	if err := rows.Err(); err != nil {
		return ReviewDocument{}, err
	}
	audits, err := r.pool.Query(ctx, `SELECT sequence_number,action,actor,payload,occurred_at FROM audit_events WHERE document_id=$1 ORDER BY sequence_number DESC LIMIT $2`, documentID, maxReviewDetailItems)
	if err != nil {
		return ReviewDocument{}, err
	}
	defer audits.Close()
	for audits.Next() {
		var event ReviewAuditEvent
		if err := audits.Scan(&event.Sequence, &event.Action, &event.Actor, &event.Payload, &event.OccurredAt); err != nil {
			return ReviewDocument{}, err
		}
		detail.Audit = append(detail.Audit, event)
	}
	if err := audits.Err(); err != nil {
		return ReviewDocument{}, err
	}

	exportRows, err := r.pool.Query(ctx, `SELECT e.id,e.document_id,v.version_number,e.export_type,e.status,e.idempotency_key,e.destination_ref,e.destination_label,e.error_summary,e.created_at,e.updated_at,e.attempts,e.next_attempt_at FROM exports e JOIN invoice_versions v ON v.id=e.version_id WHERE e.document_id=$1 ORDER BY e.created_at DESC LIMIT $2`, documentID, maxReviewDetailItems)
	if err != nil {
		return ReviewDocument{}, fmt.Errorf("load export history: %w", err)
	}
	defer exportRows.Close()
	for exportRows.Next() {
		var rec ExportRecord
		if err := exportRows.Scan(&rec.ID, &rec.DocumentID, &rec.VersionNumber, &rec.ExportType, &rec.Status, &rec.IdempotencyKey, &rec.DestinationRef, &rec.DestinationLabel, &rec.ErrorSummary, &rec.CreatedAt, &rec.UpdatedAt, &rec.Attempts, &rec.NextAttemptAt); err != nil {
			return ReviewDocument{}, fmt.Errorf("scan export history: %w", err)
		}
		detail.Exports = append(detail.Exports, rec)
	}
	if err := exportRows.Err(); err != nil {
		return ReviewDocument{}, fmt.Errorf("read export history: %w", err)
	}

	return detail, nil
}

func (r *Repository) LoadReviewSource(ctx context.Context, documentID string) (SourceDocument, error) {
	var source SourceDocument
	err := r.pool.QueryRow(ctx, `SELECT o.storage_key,o.media_type FROM documents d JOIN stored_objects o ON o.id=d.object_id WHERE d.id=$1`, documentID).Scan(&source.ObjectKey, &source.MediaType)
	if errors.Is(err, pgx.ErrNoRows) {
		return SourceDocument{}, ErrDocumentNotFound
	}
	return source, err
}

// SaveHumanReview inserts a new immutable version from untrusted user
// candidates. The document row lock serializes version allocation and state
// validation with the audit event in one transaction.
func (r *Repository) SaveHumanReview(ctx context.Context, documentID string, expectedVersion int, input HumanReviewInput, actor string) (int, error) {
	if expectedVersion < 1 || strings.TrimSpace(actor) == "" {
		return 0, ErrInvalidHumanReviewEdit
	}
	proposal := input.proposal()
	if err := extraction.DefaultLimits().ValidateProposal(proposal); err != nil {
		return 0, ErrInvalidHumanReviewEdit
	}
	normalized, warnings := invoices.NormalizeProposal(proposal)
	proposalJSON, err := json.Marshal(proposal)
	if err != nil {
		return 0, err
	}
	normalizedJSON, err := json.Marshal(normalized)
	if err != nil {
		return 0, err
	}
	warningsJSON, err := json.Marshal(warnings)
	if err != nil {
		return 0, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM documents WHERE id=$1 FOR UPDATE`, documentID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrDocumentNotFound
	} else if err != nil {
		return 0, err
	}
	if status != string(invoices.StateNeedsReview) {
		return 0, ErrInvalidDocumentState
	}
	var latest int
	var evidence, diagnostics json.RawMessage
	if err := tx.QueryRow(ctx, `SELECT version_number,evidence,diagnostics FROM invoice_versions WHERE document_id=$1 ORDER BY version_number DESC LIMIT 1`, documentID).Scan(&latest, &evidence, &diagnostics); err != nil {
		return 0, err
	}
	if latest != expectedVersion {
		return 0, ErrStaleReviewVersion
	}
	versionID, err := NewID()
	if err != nil {
		return 0, err
	}
	version := latest + 1
	if _, err = tx.Exec(ctx, `INSERT INTO invoice_versions (id,document_id,version_number,currency,total_minor,source,proposal,normalized,warnings,evidence,diagnostics,rounding_policy_version) VALUES ($1,$2,$3,NULLIF($4,''),$5,'human_review',$6,$7,$8,$9,$10,$11)`, versionID, documentID, version, normalized.Currency, normalized.Total, proposalJSON, normalizedJSON, warningsJSON, evidence, diagnostics, invoices.RoundingPolicyV1); err != nil {
		return 0, err
	}
	if err = appendAudit(ctx, tx, documentID, "human_review_saved", actor, map[string]int{"base_version": expectedVersion, "version": version}); err != nil {
		return 0, err
	}
	if err = tx.Commit(ctx); err != nil {
		return 0, err
	}
	return version, nil
}

func (r *Repository) RejectDocument(ctx context.Context, documentID, actor string) error {
	if strings.TrimSpace(actor) == "" {
		return ErrInvalidHumanReviewEdit
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM documents WHERE id=$1 FOR UPDATE`, documentID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return ErrDocumentNotFound
	} else if err != nil {
		return err
	}
	if status != string(invoices.StateNeedsReview) {
		return ErrInvalidDocumentState
	}
	if _, err = tx.Exec(ctx, `UPDATE documents SET status='rejected',updated_at=now() WHERE id=$1`, documentID); err != nil {
		return err
	}
	if err = appendAudit(ctx, tx, documentID, "document_rejected", actor, map[string]string{}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func (r *Repository) ApproveDocument(ctx context.Context, documentID string, versionNumber int, actor string) (int, error) {
	if versionNumber < 1 || strings.TrimSpace(actor) == "" {
		return 0, ErrInvalidApproval
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var status string
	if err := tx.QueryRow(ctx, `SELECT status FROM documents WHERE id=$1 FOR UPDATE`, documentID).Scan(&status); errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrDocumentNotFound
	} else if err != nil {
		return 0, err
	}
	if status != string(invoices.StateNeedsReview) {
		return 0, ErrInvalidDocumentState
	}

	var latestVer int
	var versionID string
	if err := tx.QueryRow(ctx, `SELECT id, version_number FROM invoice_versions WHERE document_id=$1 ORDER BY version_number DESC LIMIT 1`, documentID).Scan(&versionID, &latestVer); errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrStaleReviewVersion
	} else if err != nil {
		return 0, err
	}

	if versionNumber != latestVer {
		return 0, ErrStaleReviewVersion
	}

	now := time.Now().UTC()
	if _, err := tx.Exec(ctx, `UPDATE documents SET status='approved', approved_version_id=$1, approved_at=$2, updated_at=$2 WHERE id=$3`, versionID, now, documentID); err != nil {
		return 0, err
	}

	if err := appendAudit(ctx, tx, documentID, "document_approved", actor, map[string]int{"version_number": versionNumber}); err != nil {
		return 0, err
	}

	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return versionNumber, nil
}

func (r *Repository) ExportCSV(ctx context.Context, documentID string, actor string) ([]byte, error) {
	if strings.TrimSpace(actor) == "" {
		return nil, ErrInvalidExport
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var status string
	var approvedVersionID *string
	if err := tx.QueryRow(ctx, `SELECT status, approved_version_id FROM documents WHERE id=$1 FOR UPDATE`, documentID).Scan(&status, &approvedVersionID); errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrDocumentNotFound
	} else if err != nil {
		return nil, err
	}
	if (status != string(invoices.StateApproved) && status != string(invoices.StateExported)) || approvedVersionID == nil {
		return nil, ErrInvalidDocumentState
	}

	var normalizedJSON []byte
	var versionNumber int
	if err := tx.QueryRow(ctx, `SELECT version_number, normalized FROM invoice_versions WHERE id=$1`, *approvedVersionID).Scan(&versionNumber, &normalizedJSON); err != nil {
		return nil, err
	}

	var normalized invoices.NormalizedProposal
	if err := json.Unmarshal(normalizedJSON, &normalized); err != nil {
		return nil, fmt.Errorf("decode approved normalized proposal: %w", err)
	}

	csvBytes, err := export.GenerateCSV(normalized)
	if err != nil {
		return nil, fmt.Errorf("generate csv: %w", err)
	}

	idempotencyKey := fmt.Sprintf("csv_export:%s:%s", documentID, *approvedVersionID)
	exportID, err := NewID()
	if err != nil {
		return nil, err
	}
	now := time.Now().UTC()

	var existing string
	err = tx.QueryRow(ctx, `SELECT id FROM exports WHERE idempotency_key=$1`, idempotencyKey).Scan(&existing)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = tx.Exec(ctx, `INSERT INTO exports (id, document_id, version_id, export_type, status, idempotency_key, destination_ref, destination_label, created_at, updated_at) VALUES ($1,$2,$3,'csv','succeeded',$4,'local:csv-download','CSV download',$5,$5)`, exportID, documentID, *approvedVersionID, idempotencyKey, now)
		if err != nil {
			return nil, fmt.Errorf("insert csv export record: %w", err)
		}
		if status == string(invoices.StateApproved) {
			if _, err := tx.Exec(ctx, `UPDATE documents SET status='exported', updated_at=$1 WHERE id=$2`, now, documentID); err != nil {
				return nil, err
			}
		}
		if err := appendAudit(ctx, tx, documentID, "csv_exported", actor, map[string]int{"version_number": versionNumber}); err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return csvBytes, nil
}

func (r *Repository) ApprovedVersionNumber(ctx context.Context, documentID string) (int, error) {
	var version int
	err := r.pool.QueryRow(ctx, `SELECT v.version_number FROM documents d JOIN invoice_versions v ON v.id=d.approved_version_id WHERE d.id=$1 AND d.status IN ('approved','exported')`, documentID).Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, ErrInvalidDocumentState
	}
	return version, err
}

func (r *Repository) EnqueueWebhookExport(ctx context.Context, documentID string, actor string) (ExportRecord, error) {
	if strings.TrimSpace(actor) == "" {
		return ExportRecord{}, ErrInvalidExport
	}
	if !r.webhookConfigured {
		return ExportRecord{}, ErrWebhookNotConfigured
	}
	destinationRef := r.webhookDestinationRef
	destinationLabel := r.webhookDestinationLabel
	if destinationRef == "" || destinationLabel == "" {
		return ExportRecord{}, ErrWebhookNotConfigured
	}

	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return ExportRecord{}, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var status string
	var approvedVersionID *string
	if err := tx.QueryRow(ctx, `SELECT status, approved_version_id FROM documents WHERE id=$1 FOR UPDATE`, documentID).Scan(&status, &approvedVersionID); errors.Is(err, pgx.ErrNoRows) {
		return ExportRecord{}, ErrDocumentNotFound
	} else if err != nil {
		return ExportRecord{}, err
	}
	if (status != string(invoices.StateApproved) && status != string(invoices.StateExported)) || approvedVersionID == nil {
		return ExportRecord{}, ErrInvalidDocumentState
	}

	var versionNumber int
	if err := tx.QueryRow(ctx, `SELECT version_number FROM invoice_versions WHERE id=$1`, *approvedVersionID).Scan(&versionNumber); err != nil {
		return ExportRecord{}, err
	}

	idempotencyKey := fmt.Sprintf("webhook_export:%s:%s", documentID, *approvedVersionID)
	now := time.Now().UTC()

	var record ExportRecord
	var versionID string
	var existingJobID *string
	err = tx.QueryRow(ctx, `SELECT e.id, e.document_id, e.version_id, e.export_type, e.status, e.idempotency_key, e.destination_ref, e.destination_label, e.error_summary, e.job_id, e.created_at, e.updated_at, e.attempts, e.next_attempt_at FROM exports e WHERE e.idempotency_key=$1`, idempotencyKey).Scan(&record.ID, &record.DocumentID, &versionID, &record.ExportType, &record.Status, &record.IdempotencyKey, &record.DestinationRef, &record.DestinationLabel, &record.ErrorSummary, &existingJobID, &record.CreatedAt, &record.UpdatedAt, &record.Attempts, &record.NextAttemptAt)
	if errors.Is(err, pgx.ErrNoRows) {
		jobID, err := NewID()
		if err != nil {
			return ExportRecord{}, err
		}
		exportID, err := NewID()
		if err != nil {
			return ExportRecord{}, err
		}
		_, err = tx.Exec(ctx, `INSERT INTO jobs (id, document_id, job_type, status, attempts, max_attempts, next_attempt_at, idempotency_key, created_at, updated_at) VALUES ($1,$2,'export_document','ready',0,5,$3,$4,$3,$3)`, jobID, documentID, now, idempotencyKey)
		if err != nil {
			return ExportRecord{}, fmt.Errorf("enqueue export job: %w", err)
		}

		_, err = tx.Exec(ctx, `INSERT INTO exports (id, document_id, version_id, export_type, status, idempotency_key, destination_ref, destination_label, job_id, created_at, updated_at) VALUES ($1,$2,$3,'webhook','pending',$4,$5,$6,$7,$8,$8)`, exportID, documentID, *approvedVersionID, idempotencyKey, destinationRef, destinationLabel, jobID, now)
		if err != nil {
			return ExportRecord{}, fmt.Errorf("insert webhook export record: %w", err)
		}

		if err := appendAudit(ctx, tx, documentID, "export_enqueued", actor, map[string]any{"export_type": "webhook", "version_number": versionNumber, "destination_ref": destinationRef, "destination_label": destinationLabel}); err != nil {
			return ExportRecord{}, err
		}

		record = ExportRecord{
			ID:               exportID,
			DocumentID:       documentID,
			VersionNumber:    versionNumber,
			ExportType:       "webhook",
			Status:           "pending",
			IdempotencyKey:   idempotencyKey,
			DestinationRef:   destinationRef,
			DestinationLabel: destinationLabel,
			CreatedAt:        now,
			UpdatedAt:        now,
		}
	} else if err != nil {
		return ExportRecord{}, err
	}
	if versionID != "" {
		var existingVersionNumber int
		if err := tx.QueryRow(ctx, `SELECT version_number FROM invoice_versions WHERE id=$1`, versionID).Scan(&existingVersionNumber); err != nil {
			return ExportRecord{}, err
		}
		record.VersionNumber = existingVersionNumber
	}

	if err := tx.Commit(ctx); err != nil {
		return ExportRecord{}, err
	}
	return record, nil
}

func appendAudit(ctx context.Context, tx pgx.Tx, documentID, action, actor string, payload any) error {
	eventID, err := NewID()
	if err != nil {
		return err
	}
	payloadJSON, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	var sequence int64
	if err = tx.QueryRow(ctx, `UPDATE documents SET audit_sequence=audit_sequence+1 WHERE id=$1 RETURNING audit_sequence`, documentID).Scan(&sequence); err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,document_id,sequence_number,action,actor,payload) VALUES ($1,$2,$3,$4,$5,$6)`, eventID, documentID, sequence, action, actor, payloadJSON)
	return err
}

func editableFromNormalized(raw []byte) (EditableProposal, error) {
	var normalized invoices.NormalizedProposal
	if err := json.Unmarshal(raw, &normalized); err != nil {
		return EditableProposal{}, err
	}
	exponent, validCurrency := invoices.CurrencyExponent(normalized.Currency)
	format := func(value *int64) string {
		if value == nil || !validCurrency {
			return ""
		}
		return invoices.MinorToDecimalV1(*value, exponent)
	}
	editable := EditableProposal{SupplierName: normalized.SupplierName, SupplierEmail: normalized.SupplierEmail, InvoiceNumber: normalized.InvoiceNumber, IssueDate: normalized.IssueDate, DueDate: normalized.DueDate, Currency: normalized.Currency, Subtotal: format(normalized.Subtotal), TaxAmount: format(normalized.TaxAmount), Total: format(normalized.Total), LineItems: make([]EditableLineItem, len(normalized.LineItems))}
	for index, line := range normalized.LineItems {
		editable.LineItems[index] = EditableLineItem{Description: line.Description, Quantity: line.Quantity, UnitPrice: format(line.UnitPrice), TaxAmount: format(line.TaxAmount), Total: format(line.Total)}
	}
	return editable, nil
}
