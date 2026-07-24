package processing

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/invoiceflow/internal/invoices"
)

var ErrDuplicate = errors.New("duplicate document")
var ErrLeaseLost = errors.New("job lease lost")

type IntakeRecord struct {
	DocumentID, ObjectID, ObjectKey, MediaType, Actor string
	SHA256                                            [32]byte
	Size                                              int64
	CreatedAt                                         time.Time
}
type ClaimedJob struct {
	ID, DocumentID, LeaseToken string
	Attempt                    int
	LeasedUntil                time.Time
}
type ProcessDocument struct {
	DocumentID, ObjectKey, MediaType, SHA256 string
	SizeBytes                                int64
}
type ExtractionSnapshot struct {
	Currency, RoundingPolicyVersion                       string
	TotalMinor                                            *int64
	Proposal, Normalized, Warnings, Evidence, Diagnostics json.RawMessage
}

type Repository struct {
	pool                    *pgxpool.Pool
	webhookConfigured       bool
	webhookDestinationRef   string
	webhookDestinationLabel string
}

func (r *Repository) WithWebhookDestination(configured bool, ref, label string) *Repository {
	r.webhookConfigured = configured
	r.webhookDestinationRef = ref
	r.webhookDestinationLabel = label
	return r
}

// WithWebhookURL is retained as a test/configuration convenience. The URL is
// never kept on Repository and is never persisted; production wiring uses the
// opaque destination configuration above.
func (r *Repository) WithWebhookURL(url string) *Repository {
	return r.WithWebhookDestination(url != "", "server:webhook:v1", "Server-configured webhook")
}

type orphanStorage interface {
	ListObjectsOlderThan(context.Context, time.Time) ([]string, error)
	Delete(context.Context, string) error
}

func NewRepository(pool *pgxpool.Pool) *Repository { return &Repository{pool: pool} }

func NewID() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}

func (r *Repository) CreateQueuedDocument(ctx context.Context, record IntakeRecord) error {
	jobID, err := NewID()
	if err != nil {
		return err
	}
	auditID, err := NewID()
	if err != nil {
		return err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return fmt.Errorf("begin intake transaction: %w", err)
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	_, err = tx.Exec(ctx, `INSERT INTO stored_objects (id,storage_key,sha256,size_bytes,media_type) VALUES ($1,$2,$3,$4,$5)`, record.ObjectID, record.ObjectKey, record.SHA256[:], record.Size, record.MediaType)
	if err != nil {
		if isUnique(err) {
			return ErrDuplicate
		}
		return fmt.Errorf("insert stored object: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO documents (id,object_id,sha256,status,created_at,updated_at) VALUES ($1,$2,$3,'queued',$4,$4)`, record.DocumentID, record.ObjectID, record.SHA256[:], record.CreatedAt.UTC())
	if err != nil {
		if isUnique(err) {
			return ErrDuplicate
		}
		return fmt.Errorf("insert document: %w", err)
	}
	var sequence int64
	err = tx.QueryRow(ctx, `UPDATE documents SET audit_sequence = audit_sequence + 1 WHERE id=$1 RETURNING audit_sequence`, record.DocumentID).Scan(&sequence)
	if err != nil {
		return fmt.Errorf("allocate audit sequence: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,document_id,sequence_number,action,actor,payload,occurred_at) VALUES ($1,$2,$3,'document_uploaded',$4,'{}'::jsonb,$5)`, auditID, record.DocumentID, sequence, record.Actor, record.CreatedAt.UTC())
	if err != nil {
		return fmt.Errorf("insert upload audit: %w", err)
	}
	_, err = tx.Exec(ctx, `INSERT INTO jobs (id,document_id,job_type,status,attempts,max_attempts,next_attempt_at,idempotency_key,created_at,updated_at) VALUES ($1,$2,'process_document','ready',0,5,$3,$4,$3,$3)`, jobID, record.DocumentID, record.CreatedAt.UTC(), "process:"+record.DocumentID)
	if err != nil {
		return fmt.Errorf("enqueue process job: %w", err)
	}
	if err = tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit intake transaction: %w", err)
	}
	return nil
}
func isUnique(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}

// ClaimReady claims at most one due process job using SKIP LOCKED.
func (r *Repository) ClaimReady(ctx context.Context, lease time.Duration) (*ClaimedJob, error) {
	if lease <= 0 {
		return nil, fmt.Errorf("lease must be positive")
	}
	token, err := NewID()
	if err != nil {
		return nil, err
	}
	attemptID, err := NewID()
	if err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var j ClaimedJob
	err = tx.QueryRow(ctx, `WITH candidate AS (SELECT id FROM jobs WHERE job_type='process_document' AND status='ready' AND next_attempt_at<=now() ORDER BY next_attempt_at,created_at FOR UPDATE SKIP LOCKED LIMIT 1), claimed AS (UPDATE jobs j SET status='running', attempts=j.attempts+1, lease_token=$1, leased_until=now()+$2::interval, updated_at=now() FROM candidate c WHERE j.id=c.id RETURNING j.id,j.document_id,j.attempts,j.leased_until) SELECT id,document_id,attempts,leased_until FROM claimed`, token, lease.String()).Scan(&j.ID, &j.DocumentID, &j.Attempt, &j.LeasedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j.LeaseToken = token
	_, err = tx.Exec(ctx, `INSERT INTO job_attempts (id,job_id,attempt_number,lease_token) VALUES ($1,$2,$3,$4)`, attemptID, j.ID, j.Attempt, token)
	if err != nil {
		return nil, err
	}
	if err := transitionDocumentWithAudit(ctx, tx, j.DocumentID, "queued", "processing", "processing_started", "system"); err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &j, nil
}
func (r *Repository) Heartbeat(ctx context.Context, jobID, token string, lease time.Duration) error {
	tag, err := r.pool.Exec(ctx, `UPDATE jobs SET leased_until=now()+$3::interval,updated_at=now() WHERE id=$1 AND status='running' AND lease_token=$2 AND leased_until>now()`, jobID, token, lease.String())
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	return nil
}
func (r *Repository) FinishRetry(ctx context.Context, jobID, token, summary string, delay time.Duration) error {
	if len(summary) > 500 {
		summary = summary[:500]
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var attempts, max int
	var documentID string
	err = tx.QueryRow(ctx, `SELECT attempts,max_attempts,document_id FROM jobs WHERE id=$1 AND status='running' AND lease_token=$2 AND leased_until>now() FOR UPDATE`, jobID, token).Scan(&attempts, &max, &documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return err
	}
	status := "ready"
	if attempts >= max {
		status = "dead_letter"
	}
	_, err = tx.Exec(ctx, `UPDATE jobs SET status=$3, lease_token=NULL, leased_until=NULL, next_attempt_at=CASE WHEN $3='ready' THEN now()+$4::interval ELSE NULL END,last_error=$5,updated_at=now() WHERE id=$1 AND lease_token=$2`, jobID, token, status, delay.String(), summary)
	if err != nil {
		return err
	}
	outcome := "retryable_failure"
	if status == "dead_letter" {
		outcome = "permanent_failure"
	}
	_, err = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=now(),outcome=$3,error_summary=$4 WHERE job_id=$1 AND lease_token=$2 AND finished_at IS NULL`, jobID, token, outcome, summary)
	if err != nil {
		return err
	}
	nextDocumentStatus, action := "queued", "processing_retry_scheduled"
	if status == "dead_letter" {
		nextDocumentStatus, action = "failed", "processing_dead_lettered"
	}
	if err := transitionDocumentWithAudit(ctx, tx, documentID, "processing", nextDocumentStatus, action, "system"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// FinishPermanent records one non-retryable processing failure and transitions
// the document to failed immediately. The immutable original and attempt stay
// available for later diagnosis without storing tool output.
func (r *Repository) FinishPermanent(ctx context.Context, jobID, token, summary string) error {
	return r.finishFailure(ctx, jobID, token, summary, 0, true)
}

func (r *Repository) finishFailure(ctx context.Context, jobID, token, summary string, delay time.Duration, permanent bool) error {
	if len(summary) > 500 {
		summary = summary[:500]
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var documentID string
	err = tx.QueryRow(ctx, `SELECT document_id FROM jobs WHERE id=$1 AND status='running' AND lease_token=$2 AND leased_until>now() FOR UPDATE`, jobID, token).Scan(&documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE jobs SET status='dead_letter',lease_token=NULL,leased_until=NULL,next_attempt_at=NULL,last_error=$3,updated_at=now() WHERE id=$1 AND lease_token=$2`, jobID, token, summary); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=now(),outcome='permanent_failure',error_summary=$3 WHERE job_id=$1 AND lease_token=$2 AND finished_at IS NULL`, jobID, token, summary); err != nil {
		return err
	}
	if err = transitionDocumentWithAudit(ctx, tx, documentID, "processing", "failed", "processing_failed", "system"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// LoadProcessDocument reads only server-owned metadata needed to open an
// original. It intentionally does not return a client filename or filesystem
// path.
func (r *Repository) LoadProcessDocument(ctx context.Context, documentID string) (ProcessDocument, error) {
	var record ProcessDocument
	var hash []byte
	err := r.pool.QueryRow(ctx, `SELECT d.id,o.storage_key,o.media_type,o.sha256,o.size_bytes FROM documents d JOIN stored_objects o ON o.id=d.object_id WHERE d.id=$1`, documentID).Scan(&record.DocumentID, &record.ObjectKey, &record.MediaType, &hash, &record.SizeBytes)
	if err != nil {
		return ProcessDocument{}, err
	}
	if len(hash) != 32 {
		return ProcessDocument{}, fmt.Errorf("stored hash has invalid length")
	}
	record.SHA256 = hex.EncodeToString(hash)
	return record, nil
}

// FinishExtraction atomically persists one immutable Stage 3 proposal snapshot,
// completes the leased attempt, and moves the document to needs_review. No
// review UI or approval transition is exposed by this method.
func (r *Repository) FinishExtraction(ctx context.Context, jobID, token string, snapshot ExtractionSnapshot) error {
	if snapshot.RoundingPolicyVersion == "" || !json.Valid(snapshot.Proposal) || !json.Valid(snapshot.Normalized) || !json.Valid(snapshot.Warnings) || !json.Valid(snapshot.Evidence) || !json.Valid(snapshot.Diagnostics) {
		return fmt.Errorf("invalid extraction snapshot")
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var documentID string
	err = tx.QueryRow(ctx, `SELECT document_id FROM jobs WHERE id=$1 AND status='running' AND lease_token=$2 AND leased_until>now() FOR UPDATE`, jobID, token).Scan(&documentID)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return err
	}
	versionID, err := NewID()
	if err != nil {
		return err
	}
	var version int
	if err = tx.QueryRow(ctx, `SELECT COALESCE(MAX(version_number),0)+1 FROM invoice_versions WHERE document_id=$1`, documentID).Scan(&version); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `INSERT INTO invoice_versions (id,document_id,version_number,currency,total_minor,source,proposal,normalized,warnings,evidence,diagnostics,rounding_policy_version) VALUES ($1,$2,$3,NULLIF($4,''),$5,'extraction',$6,$7,$8,$9,$10,$11)`, versionID, documentID, version, snapshot.Currency, snapshot.TotalMinor, snapshot.Proposal, snapshot.Normalized, snapshot.Warnings, snapshot.Evidence, snapshot.Diagnostics, snapshot.RoundingPolicyVersion); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE jobs SET status='succeeded',lease_token=NULL,leased_until=NULL,next_attempt_at=NULL,last_error=NULL,updated_at=now() WHERE id=$1 AND lease_token=$2`, jobID, token); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=now(),outcome='succeeded',error_summary=NULL WHERE job_id=$1 AND lease_token=$2 AND finished_at IS NULL`, jobID, token); err != nil {
		return err
	}
	if err = transitionDocumentWithAudit(ctx, tx, documentID, "processing", "needs_review", "processing_completed", "system"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
func (r *Repository) RecoverExpiredLeases(ctx context.Context) (int64, error) {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return 0, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	rows, err := tx.Query(ctx, `SELECT id,document_id,job_type,lease_token,attempts,max_attempts FROM jobs WHERE status='running' AND leased_until<=now() FOR UPDATE SKIP LOCKED`)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var jobs []struct {
		id, documentID, jobType, token string
		attempts, max                  int
	}
	for rows.Next() {
		var j struct {
			id, documentID, jobType, token string
			attempts, max                  int
		}
		if err := rows.Scan(&j.id, &j.documentID, &j.jobType, &j.token, &j.attempts, &j.max); err != nil {
			return 0, err
		}
		jobs = append(jobs, j)
	}
	if err := rows.Err(); err != nil {
		return 0, err
	}
	for _, j := range jobs {
		if j.jobType == string(JobTypeExport) {
			status := "ready"
			if j.attempts >= j.max {
				status = "dead_letter"
			}
			if _, err := tx.Exec(ctx, `UPDATE jobs SET status=$2,lease_token=NULL,leased_until=NULL,next_attempt_at=CASE WHEN $2='ready' THEN now() ELSE NULL END,updated_at=now(),last_error='lease expired' WHERE id=$1`, j.id, status); err != nil {
				return 0, err
			}
			outcome := "retryable_failure"
			if status == "dead_letter" {
				outcome = "permanent_failure"
			}
			if _, err := tx.Exec(ctx, `UPDATE job_attempts SET finished_at=now(),outcome=$3,error_summary='lease expired' WHERE job_id=$1 AND lease_token=$2 AND finished_at IS NULL`, j.id, j.token, outcome); err != nil {
				return 0, err
			}
			var exportID string
			var versionNumber int
			if err := tx.QueryRow(ctx, `SELECT e.id,v.version_number FROM exports e JOIN invoice_versions v ON v.id=e.version_id WHERE e.job_id=$1`, j.id).Scan(&exportID, &versionNumber); err != nil {
				return 0, err
			}
			if status == "ready" {
				if _, err := tx.Exec(ctx, `UPDATE exports SET status='retrying',attempts=$2,next_attempt_at=now(),error_summary='webhook delivery temporary failure',updated_at=now() WHERE id=$1`, exportID, j.attempts); err != nil {
					return 0, err
				}
				if err := appendAudit(ctx, tx, j.documentID, "export_retry_scheduled", "system", map[string]any{"version_number": versionNumber, "attempt": j.attempts, "reason": "lease expired"}); err != nil {
					return 0, err
				}
			} else {
				if _, err := tx.Exec(ctx, `UPDATE exports SET status='dead_letter',attempts=$2,next_attempt_at=NULL,error_summary='webhook delivery failed (exhausted retries)',updated_at=now() WHERE id=$1`, exportID, j.attempts); err != nil {
					return 0, err
				}
				if err := appendAudit(ctx, tx, j.documentID, "export_dead_lettered", "system", map[string]any{"version_number": versionNumber, "attempt": j.attempts, "reason": "lease expired"}); err != nil {
					return 0, err
				}
			}
			continue
		}
		status, outcome, next, action := "ready", "retryable_failure", "queued", "processing_lease_recovered"
		if j.attempts >= j.max {
			status, outcome, next, action = "dead_letter", "permanent_failure", "failed", "processing_dead_lettered"
		}
		if _, err := tx.Exec(ctx, `UPDATE jobs SET status=$2,lease_token=NULL,leased_until=NULL,next_attempt_at=CASE WHEN $2='ready' THEN now() ELSE NULL END,updated_at=now(),last_error='lease expired' WHERE id=$1`, j.id, status); err != nil {
			return 0, err
		}
		if _, err := tx.Exec(ctx, `UPDATE job_attempts SET finished_at=now(),outcome=$3,error_summary='lease expired' WHERE job_id=$1 AND lease_token=$2 AND finished_at IS NULL`, j.id, j.token, outcome); err != nil {
			return 0, err
		}
		if err := transitionDocumentWithAudit(ctx, tx, j.documentID, "processing", next, action, "system"); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return int64(len(jobs)), nil
}

// ReconcileOrphanedObjects removes only old, server-owned objects for which
// PostgreSQL has no stored-object record. The age grace avoids racing the
// storage-first upload transaction; it never deletes a referenced original.
func (r *Repository) ReconcileOrphanedObjects(ctx context.Context, storage orphanStorage, grace time.Duration) (int, error) {
	if grace <= 0 {
		return 0, fmt.Errorf("orphan grace must be positive")
	}
	keys, err := storage.ListObjectsOlderThan(ctx, time.Now().UTC().Add(-grace))
	if err != nil {
		return 0, err
	}
	removed := 0
	for _, key := range keys {
		var referenced bool
		if err := r.pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM stored_objects WHERE storage_key=$1)`, key).Scan(&referenced); err != nil {
			return removed, err
		}
		if referenced {
			continue
		}
		if err := storage.Delete(ctx, key); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func transitionDocumentWithAudit(ctx context.Context, tx pgx.Tx, id, from, to, action, actor string) error {
	eventID, err := NewID()
	if err != nil {
		return err
	}
	var sequence int64
	err = tx.QueryRow(ctx, `UPDATE documents SET status=$3,updated_at=now(),audit_sequence=audit_sequence+1 WHERE id=$1 AND status=$2 RETURNING audit_sequence`, id, from, to).Scan(&sequence)
	if errors.Is(err, pgx.ErrNoRows) {
		return fmt.Errorf("document state changed during job transition")
	}
	if err != nil {
		return err
	}
	_, err = tx.Exec(ctx, `INSERT INTO audit_events (id,document_id,sequence_number,action,actor,payload) VALUES ($1,$2,$3,$4,$5,'{}'::jsonb)`, eventID, id, sequence, action, actor)
	return err
}

type ExportJobDetails struct {
	ExportID         string
	IdempotencyKey   string
	DestinationRef   string
	DestinationLabel string
	DocumentID       string
	VersionNumber    int
	ApprovedAt       time.Time
	Normalized       invoices.NormalizedProposal
}

func (r *Repository) ClaimExportReady(ctx context.Context, lease time.Duration) (*ClaimedJob, error) {
	if lease <= 0 {
		return nil, fmt.Errorf("lease must be positive")
	}
	token, err := NewID()
	if err != nil {
		return nil, err
	}
	attemptID, err := NewID()
	if err != nil {
		return nil, err
	}
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()
	var j ClaimedJob
	err = tx.QueryRow(ctx, `WITH candidate AS (SELECT id FROM jobs WHERE job_type='export_document' AND status='ready' AND next_attempt_at<=now() ORDER BY next_attempt_at,created_at FOR UPDATE SKIP LOCKED LIMIT 1), claimed AS (UPDATE jobs j SET status='running', attempts=j.attempts+1, lease_token=$1, leased_until=now()+$2::interval, updated_at=now() FROM candidate c WHERE j.id=c.id RETURNING j.id,j.document_id,j.attempts,j.leased_until) SELECT id,document_id,attempts,leased_until FROM claimed`, token, lease.String()).Scan(&j.ID, &j.DocumentID, &j.Attempt, &j.LeasedUntil)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	j.LeaseToken = token
	_, err = tx.Exec(ctx, `INSERT INTO job_attempts (id,job_id,attempt_number,lease_token) VALUES ($1,$2,$3,$4)`, attemptID, j.ID, j.Attempt, token)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &j, nil
}

func (r *Repository) LoadExportJobDetails(ctx context.Context, jobID string) (ExportJobDetails, error) {
	var details ExportJobDetails
	var normalizedJSON []byte

	err := r.pool.QueryRow(ctx, `
		SELECT e.id, e.idempotency_key, e.destination_ref, e.destination_label, d.id, v.version_number, d.approved_at, v.normalized
		FROM jobs j
		JOIN exports e ON e.job_id = j.id
		JOIN documents d ON d.id = j.document_id
		JOIN invoice_versions v ON v.id = e.version_id AND e.version_id = d.approved_version_id
		WHERE j.id = $1
	`, jobID).Scan(&details.ExportID, &details.IdempotencyKey, &details.DestinationRef, &details.DestinationLabel, &details.DocumentID, &details.VersionNumber, &details.ApprovedAt, &normalizedJSON)
	if err != nil {
		return ExportJobDetails{}, err
	}

	if err := json.Unmarshal(normalizedJSON, &details.Normalized); err != nil {
		return ExportJobDetails{}, fmt.Errorf("decode normalized proposal for export: %w", err)
	}

	return details, nil
}

func (r *Repository) FinishExportSuccess(ctx context.Context, jobID, token, exportID, actor string) error {
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var attempts int
	var documentID string
	err = tx.QueryRow(ctx, `SELECT document_id,attempts FROM jobs WHERE id=$1 AND status='running' AND lease_token=$2 AND leased_until>now() FOR UPDATE`, jobID, token).Scan(&documentID, &attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if _, err = tx.Exec(ctx, `UPDATE jobs SET status='succeeded', lease_token=NULL, leased_until=NULL, next_attempt_at=NULL, last_error=NULL, updated_at=$3 WHERE id=$1 AND lease_token=$2`, jobID, token, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=$3, outcome='succeeded' WHERE job_id=$1 AND lease_token=$2 AND finished_at IS NULL`, jobID, token, now); err != nil {
		return err
	}
	if tag, err := tx.Exec(ctx, `UPDATE exports SET status='succeeded', attempts=$2, next_attempt_at=NULL, error_summary=NULL, updated_at=$3 WHERE id=$1 AND job_id=$4`, exportID, attempts, now, jobID); err != nil {
		return err
	} else if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}

	var status string
	var versionNumber int
	if err := tx.QueryRow(ctx, `SELECT d.status, v.version_number FROM documents d JOIN invoice_versions v ON v.id=d.approved_version_id WHERE d.id=$1 FOR UPDATE`, documentID).Scan(&status, &versionNumber); err != nil {
		return err
	}
	if status == string(invoices.StateApproved) {
		if _, err := tx.Exec(ctx, `UPDATE documents SET status='exported', updated_at=$1 WHERE id=$2`, now, documentID); err != nil {
			return err
		}
	}

	if err := appendAudit(ctx, tx, documentID, "webhook_exported", actor, map[string]int{"version_number": versionNumber}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}

func (r *Repository) FinishExportRetry(ctx context.Context, jobID, token, exportID, summary string, delay time.Duration) error {
	safeSummary := "webhook delivery temporary failure"
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var attempts, max int
	var documentID string
	var versionNumber int
	err = tx.QueryRow(ctx, `SELECT j.attempts,j.max_attempts,j.document_id,v.version_number FROM jobs j JOIN exports e ON e.job_id=j.id JOIN invoice_versions v ON v.id=e.version_id WHERE j.id=$1 AND j.status='running' AND j.lease_token=$2 AND j.leased_until>now() FOR UPDATE`, jobID, token).Scan(&attempts, &max, &documentID, &versionNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return err
	}

	status := "ready"
	if attempts >= max {
		status = "dead_letter"
	}

	now := time.Now().UTC()
	_, err = tx.Exec(ctx, `UPDATE jobs SET status=$3, lease_token=NULL, leased_until=NULL, next_attempt_at=CASE WHEN $3='ready' THEN now()+$4::interval ELSE NULL END, last_error=$5, updated_at=$6 WHERE id=$1 AND lease_token=$2`, jobID, token, status, delay.String(), safeSummary, now)
	if err != nil {
		return err
	}

	outcome := "retryable_failure"
	if status == "dead_letter" {
		outcome = "permanent_failure"
	}
	_, err = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=$4, outcome=$3, error_summary=$5 WHERE job_id=$1 AND lease_token=$2 AND finished_at IS NULL`, jobID, token, outcome, now, safeSummary)
	if err != nil {
		return err
	}

	if status == "ready" {
		if err := tx.QueryRow(ctx, `UPDATE exports SET status='retrying',attempts=$2,next_attempt_at=now()+$3::interval,error_summary='webhook delivery temporary failure',updated_at=$4 WHERE id=$1 AND job_id=$5 RETURNING id`, exportID, attempts, delay.String(), now, jobID).Scan(new(string)); err != nil {
			return err
		}
		if err := appendAudit(ctx, tx, documentID, "export_retry_scheduled", "system", map[string]any{"version_number": versionNumber, "attempt": attempts, "next_attempt_in_seconds": int(delay.Seconds())}); err != nil {
			return err
		}
	} else if status == "dead_letter" {
		if tag, err := tx.Exec(ctx, `UPDATE exports SET status='dead_letter',attempts=$2,next_attempt_at=NULL,error_summary='webhook delivery failed (exhausted retries)',updated_at=$3 WHERE id=$1 AND job_id=$4`, exportID, attempts, now, jobID); err != nil {
			return err
		} else if tag.RowsAffected() != 1 {
			return ErrLeaseLost
		}
		if err := appendAudit(ctx, tx, documentID, "export_dead_lettered", "system", map[string]any{"version_number": versionNumber, "attempt": attempts, "reason": "retry limit exhausted"}); err != nil {
			return err
		}
	}

	return tx.Commit(ctx)
}

func (r *Repository) FinishExportPermanent(ctx context.Context, jobID, token, exportID, summary string) error {
	safeSummary := "webhook delivery failed (permanent)"
	tx, err := r.pool.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.Background()) }()

	var attempts int
	var documentID string
	var versionNumber int
	err = tx.QueryRow(ctx, `SELECT j.attempts,j.document_id,v.version_number FROM jobs j JOIN exports e ON e.job_id=j.id JOIN invoice_versions v ON v.id=e.version_id WHERE j.id=$1 AND j.status='running' AND j.lease_token=$2 AND j.leased_until>now() FOR UPDATE`, jobID, token).Scan(&attempts, &documentID, &versionNumber)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLeaseLost
	}
	if err != nil {
		return err
	}

	now := time.Now().UTC()
	if _, err = tx.Exec(ctx, `UPDATE jobs SET status='dead_letter', lease_token=NULL, leased_until=NULL, next_attempt_at=NULL, last_error=$3, updated_at=$4 WHERE id=$1 AND lease_token=$2`, jobID, token, safeSummary, now); err != nil {
		return err
	}
	if _, err = tx.Exec(ctx, `UPDATE job_attempts SET finished_at=$4, outcome='permanent_failure', error_summary=$3 WHERE job_id=$1 AND lease_token=$2 AND finished_at IS NULL`, jobID, token, safeSummary, now); err != nil {
		return err
	}
	if tag, err := tx.Exec(ctx, `UPDATE exports SET status='dead_letter', attempts=$2, next_attempt_at=NULL, error_summary=$3, updated_at=$4 WHERE id=$1 AND job_id=$5`, exportID, attempts, safeSummary, now, jobID); err != nil {
		return err
	} else if tag.RowsAffected() != 1 {
		return ErrLeaseLost
	}
	if err := appendAudit(ctx, tx, documentID, "export_dead_lettered", "system", map[string]any{"version_number": versionNumber, "reason": safeSummary}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
