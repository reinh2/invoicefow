package processing

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/reinhlord/invoiceflow/internal/extraction"
	"github.com/reinhlord/invoiceflow/internal/invoices"
)

var (
	ErrDocumentNotFound       = errors.New("document not found")
	ErrInvalidDocumentState   = errors.New("invalid document transition")
	ErrStaleReviewVersion     = errors.New("stale review version")
	ErrInvalidHumanReviewEdit = errors.New("invalid human review edit")
)

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
	ID        string             `json:"id"`
	Status    string             `json:"status"`
	CreatedAt time.Time          `json:"created_at"`
	UpdatedAt time.Time          `json:"updated_at"`
	MediaType string             `json:"media_type"`
	Versions  []ReviewVersion    `json:"versions"`
	Audit     []ReviewAuditEvent `json:"audit"`
}

type SourceDocument struct{ ObjectKey, MediaType string }

// GetReviewDocument returns a bounded, presentation-safe view. Server storage
// keys and hashes remain internal and source bytes require a separate stream.
func (r *Repository) GetReviewDocument(ctx context.Context, documentID string) (ReviewDocument, error) {
	var detail ReviewDocument
	err := r.pool.QueryRow(ctx, `SELECT d.id,d.status,d.created_at,d.updated_at,o.media_type FROM documents d JOIN stored_objects o ON o.id=d.object_id WHERE d.id=$1`, documentID).Scan(&detail.ID, &detail.Status, &detail.CreatedAt, &detail.UpdatedAt, &detail.MediaType)
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
