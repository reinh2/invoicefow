//go:build integration

package processing

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/invoiceflow/internal/extraction"
	"github.com/reinhlord/invoiceflow/internal/platform"
)

func integrationRepository(t *testing.T) (*Repository, *pgxpool.Pool, context.Context) {
	t.Helper()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		t.Fatal("DATABASE_URL is required for PostgreSQL integration tests; start Compose PostgreSQL and set DATABASE_URL")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)
	admin, err := platform.OpenPool(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	schemaID, err := NewID()
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	schema := "invoiceflow_it_" + strings.ReplaceAll(schemaID, "-", "")
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		admin.Close()
		t.Fatal(err)
	}
	poolConfig, err := pgxpool.ParseConfig(url)
	if err != nil {
		admin.Close()
		t.Fatal(err)
	}
	poolConfig.ConnConfig.RuntimeParams["search_path"] = schema
	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
		t.Fatal(err)
	}
	t.Cleanup(func() {
		pool.Close()
		_, _ = admin.Exec(context.Background(), "DROP SCHEMA "+schema+" CASCADE")
		admin.Close()
	})
	if err := platform.Migrate(ctx, pool, filepath.Join("..", "..", "db", "migrations")); err != nil {
		t.Fatal(err)
	}
	return NewRepository(pool), pool, ctx
}

func integrationRecord(t *testing.T, label string) IntakeRecord {
	t.Helper()
	documentID, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	objectID, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	hash := sha256.Sum256([]byte(t.Name() + ":" + label))
	return IntakeRecord{
		DocumentID: documentID,
		ObjectID:   objectID,
		ObjectKey:  "integration/" + objectID + ".pdf",
		MediaType:  "application/pdf",
		Actor:      "integration-test",
		SHA256:     hash,
		Size:       17,
		CreatedAt:  time.Now().UTC(),
	}
}

func TestCreateQueuedDocumentAtomicallyPersistsOriginalAuditAndJob(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record := integrationRecord(t, "atomic")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}

	var objects, documents, audits, jobs int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM stored_objects WHERE id=$1),
		(SELECT count(*) FROM documents WHERE id=$2 AND status='queued'),
		(SELECT count(*) FROM audit_events WHERE document_id=$2 AND action='document_uploaded' AND sequence_number=1),
		(SELECT count(*) FROM jobs WHERE document_id=$2 AND job_type='process_document' AND status='ready' AND attempts=0 AND idempotency_key='process:' || $2)`, record.ObjectID, record.DocumentID).Scan(&objects, &documents, &audits, &jobs); err != nil {
		t.Fatal(err)
	}
	if objects != 1 || documents != 1 || audits != 1 || jobs != 1 {
		t.Fatalf("atomic intake rows objects=%d documents=%d audits=%d jobs=%d, want all 1", objects, documents, audits, jobs)
	}
	// Do not leave this independent fixture eligible for the global worker queue.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status='succeeded' WHERE document_id=$1`, record.DocumentID); err != nil {
		t.Fatal(err)
	}
}

func TestMigrationEnforcesStage2JobInvariants(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record := integrationRecord(t, "constraints")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	var jobID string
	if err := pool.QueryRow(ctx, `SELECT id FROM jobs WHERE document_id=$1`, record.DocumentID).Scan(&jobID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET attempts=max_attempts+1 WHERE id=$1`, jobID); err == nil || !strings.Contains(err.Error(), "jobs_attempts_bounded") {
		t.Fatalf("unbounded job attempts error=%v, want jobs_attempts_bounded", err)
	}
	claimed, err := repo.ClaimReady(ctx, time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("ClaimReady() job=%+v err=%v", claimed, err)
	}
	duplicateAttemptID, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	duplicateToken, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO job_attempts (id,job_id,attempt_number,lease_token) VALUES ($1,$2,2,$3)`, duplicateAttemptID, claimed.ID, duplicateToken); err == nil || !strings.Contains(err.Error(), "job_attempts_one_open_per_job") {
		t.Fatalf("second open attempt error=%v, want job_attempts_one_open_per_job", err)
	}
}

func TestCreateQueuedDocumentConcurrentDuplicateHasOneWinner(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	first := integrationRecord(t, "race")
	second := integrationRecord(t, "other-ids-same-hash")
	second.SHA256 = first.SHA256

	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for _, record := range []IntakeRecord{first, second} {
		wg.Add(1)
		go func(record IntakeRecord) {
			defer wg.Done()
			<-start
			errs <- repo.CreateQueuedDocument(ctx, record)
		}(record)
	}
	close(start)
	wg.Wait()
	close(errs)
	var successes, duplicates int
	for err := range errs {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, ErrDuplicate):
			duplicates++
		default:
			t.Fatalf("concurrent intake error: %v", err)
		}
	}
	if successes != 1 || duplicates != 1 {
		t.Fatalf("concurrent intake successes=%d duplicates=%d, want 1 each", successes, duplicates)
	}
	var documents, audits, jobs int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM documents WHERE sha256=$1),
		(SELECT count(*) FROM audit_events a JOIN documents d ON d.id=a.document_id WHERE d.sha256=$1),
		(SELECT count(*) FROM jobs j JOIN documents d ON d.id=j.document_id WHERE d.sha256=$1)`, first.SHA256[:]).Scan(&documents, &audits, &jobs); err != nil {
		t.Fatal(err)
	}
	if documents != 1 || audits != 1 || jobs != 1 {
		t.Fatalf("duplicate race left documents=%d audits=%d jobs=%d, want one accepted set", documents, audits, jobs)
	}
	// ClaimReady is deliberately global, so retire the race fixture before later
	// tests exercise its one-winner behavior.
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status='succeeded' WHERE document_id IN (SELECT id FROM documents WHERE sha256=$1)`, first.SHA256[:]); err != nil {
		t.Fatal(err)
	}
}

func TestClaimReadyHasOneWinnerAndRecordsAttempt(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record := integrationRecord(t, "claim")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}

	start := make(chan struct{})
	claims := make(chan *ClaimedJob, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			job, err := repo.ClaimReady(ctx, time.Minute)
			claims <- job
			errs <- err
		}()
	}
	close(start)
	wg.Wait()
	close(claims)
	close(errs)
	var winners int
	for job := range claims {
		if job != nil {
			winners++
			if job.DocumentID != record.DocumentID || job.Attempt != 1 || job.LeaseToken == "" {
				t.Fatalf("unexpected claim: %+v", job)
			}
		}
	}
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if winners != 1 {
		t.Fatalf("claim winners=%d, want 1", winners)
	}
	var attempts, openAttempts, processingAudits int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT attempts FROM jobs WHERE document_id=$1),
		(SELECT count(*) FROM job_attempts ja JOIN jobs j ON j.id=ja.job_id WHERE j.document_id=$1 AND ja.finished_at IS NULL),
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='processing_started')`, record.DocumentID).Scan(&attempts, &openAttempts, &processingAudits); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || openAttempts != 1 || processingAudits != 1 {
		t.Fatalf("claim persistence attempts=%d open=%d audits=%d, want 1 each", attempts, openAttempts, processingAudits)
	}
}

func TestHeartbeatAndLeaseRecoveryPreserveAttemptHistory(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record := integrationRecord(t, "recovery")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimReady(ctx, time.Second)
	if err != nil || claimed == nil {
		t.Fatalf("ClaimReady() job=%+v err=%v", claimed, err)
	}
	if err := repo.Heartbeat(ctx, claimed.ID, claimed.LeaseToken, 2*time.Second); err != nil {
		t.Fatalf("Heartbeat() error = %v", err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET leased_until=now()-interval '1 second' WHERE id=$1`, claimed.ID); err != nil {
		t.Fatal(err)
	}
	if err := repo.Heartbeat(ctx, claimed.ID, claimed.LeaseToken, time.Second); !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("expired Heartbeat() error = %v, want ErrLeaseLost", err)
	}
	recovered, err := repo.RecoverExpiredLeases(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if recovered != 1 {
		t.Fatalf("RecoverExpiredLeases()=%d, want 1", recovered)
	}
	var status, documentStatus, outcome, summary string
	var finished int
	if err := pool.QueryRow(ctx, `SELECT j.status,d.status,ja.outcome,ja.error_summary,CASE WHEN ja.finished_at IS NULL THEN 0 ELSE 1 END
		FROM jobs j JOIN documents d ON d.id=j.document_id JOIN job_attempts ja ON ja.job_id=j.id
		WHERE j.id=$1 AND ja.attempt_number=1`, claimed.ID).Scan(&status, &documentStatus, &outcome, &summary, &finished); err != nil {
		t.Fatal(err)
	}
	if status != "ready" || documentStatus != "queued" || outcome != "retryable_failure" || summary != "lease expired" || finished != 1 {
		t.Fatalf("recovery status=%q document=%q outcome=%q summary=%q finished=%d", status, documentStatus, outcome, summary, finished)
	}
	if job, err := repo.ClaimReady(ctx, time.Second); err != nil || job == nil || job.Attempt != 2 {
		t.Fatalf("reclaimed job=%+v err=%v, want attempt 2", job, err)
	}
}

func TestFinishRetryDeadLettersAtMaximumAttempts(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record := integrationRecord(t, "dead-letter")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= 5; attempt++ {
		job, err := repo.ClaimReady(ctx, time.Second)
		if err != nil || job == nil {
			t.Fatalf("claim %d: job=%+v err=%v", attempt, job, err)
		}
		summary := fmt.Sprintf("temporary failure %d", attempt)
		if err := repo.FinishRetry(ctx, job.ID, job.LeaseToken, summary, 0); err != nil {
			t.Fatalf("FinishRetry %d: %v", attempt, err)
		}
	}
	var jobStatus, documentStatus, lastError string
	var attempts, closedAttempts, permanentFailures int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT status FROM jobs WHERE document_id=$1),
		(SELECT status FROM documents WHERE id=$1),
		(SELECT attempts FROM jobs WHERE document_id=$1),
		(SELECT last_error FROM jobs WHERE document_id=$1),
		(SELECT count(*) FROM job_attempts ja JOIN jobs j ON j.id=ja.job_id WHERE j.document_id=$1 AND ja.finished_at IS NOT NULL),
		(SELECT count(*) FROM job_attempts ja JOIN jobs j ON j.id=ja.job_id WHERE j.document_id=$1 AND ja.outcome='permanent_failure')`, record.DocumentID).Scan(&jobStatus, &documentStatus, &attempts, &lastError, &closedAttempts, &permanentFailures); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "dead_letter" || documentStatus != "failed" || attempts != 5 || lastError != "temporary failure 5" || closedAttempts != 5 || permanentFailures != 1 {
		t.Fatalf("dead letter job=%q document=%q attempts=%d error=%q closed=%d permanent=%d", jobStatus, documentStatus, attempts, lastError, closedAttempts, permanentFailures)
	}
	if job, err := repo.ClaimReady(ctx, time.Second); err != nil || job != nil {
		t.Fatalf("dead-lettered job was claimable: job=%+v err=%v", job, err)
	}
}

type integrationStorage struct{ payload []byte }

func (s integrationStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader(string(s.payload))), nil
}

type integrationText struct{}

func (integrationText) ExtractText(_ context.Context, _ extraction.DocumentInput, _ extraction.Limits) (extraction.TextExtractionResult, error) {
	return extraction.TextExtractionResult{Pages: []extraction.PageText{{PageNumber: 1, Text: "INVOICEFLOW_FIXTURE:WORKER-001"}}}, nil
}

type integrationOCR struct{}

func (integrationOCR) ExtractOCR(context.Context, extraction.DocumentInput, extraction.Limits) (extraction.OCRResult, error) {
	return extraction.OCRResult{}, errors.New("OCR should not be called")
}

func TestWorkerPersistsNormalizedExtractionAndCompletesLease(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record := integrationRecord(t, "worker")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	supplier, currency, subtotal, tax, total, quantity, price, lineTotal := "Fictional Vendor", "USD", "20.00", "4.00", "24.00", "2", "10.00", "20.00"
	fake, err := extraction.NewFakeStructuredExtractor([]extraction.FakeFixture{{DocumentSHA256: fmt.Sprintf("%x", record.SHA256), Marker: "INVOICEFLOW_FIXTURE:WORKER-001", Proposal: extraction.Proposal{SupplierName: &supplier, Currency: &currency, Subtotal: &subtotal, TaxAmount: &tax, Total: &total, LineItems: []extraction.LineItemProposal{{Quantity: &quantity, UnitPrice: &price, Total: &lineTotal}}}}})
	if err != nil {
		t.Fatal(err)
	}
	worker := Worker{Repository: repo, Storage: integrationStorage{payload: []byte("fictional")}, Text: integrationText{}, OCR: integrationOCR{}, Structured: fake, Limits: extraction.DefaultLimits(), Lease: time.Minute, RetryDelay: 0}
	processed, err := worker.RunOnce(ctx)
	if err != nil || !processed {
		t.Fatalf("RunOnce() = %v, %v", processed, err)
	}
	var jobStatus, documentStatus, source, policy string
	var totalMinor int64
	var warnings, diagnostics []byte
	if err := pool.QueryRow(ctx, `SELECT j.status,d.status,v.source,v.rounding_policy_version,v.total_minor,v.warnings,v.diagnostics FROM jobs j JOIN documents d ON d.id=j.document_id JOIN invoice_versions v ON v.document_id=d.id WHERE d.id=$1`, record.DocumentID).Scan(&jobStatus, &documentStatus, &source, &policy, &totalMinor, &warnings, &diagnostics); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "succeeded" || documentStatus != "needs_review" || source != "extraction" || policy != "money-v1" || totalMinor != 2400 || string(warnings) != "[]" || string(diagnostics) != "[]" {
		t.Fatalf("snapshot job=%q document=%q source=%q policy=%q total=%d warnings=%s diagnostics=%s", jobStatus, documentStatus, source, policy, totalMinor, warnings, diagnostics)
	}
}

func TestHumanReviewVersionsAndRejectionAreImmutableAndAtomic(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record := integrationRecord(t, "human-review")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimReady(ctx, time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("ClaimReady() = %+v, %v", claimed, err)
	}
	snapshot := ExtractionSnapshot{
		Currency: "USD", RoundingPolicyVersion: "money-v1", TotalMinor: ptrInt64(2400),
		Proposal:   []byte(`{"supplier_name":"Fictional Vendor","currency":"USD","total":"24.00","line_items":[]}`),
		Normalized: []byte(`{"rounding_policy_version":"money-v1","supplier_name":"Fictional Vendor","currency":"USD","total_minor":2400,"line_items":[]}`),
		Warnings:   []byte(`[{"code":"missing_due_date","field":"due_date","message":"Due date is missing."}]`),
		Evidence:   []byte(`[{"field":"total","page_number":1,"excerpt":"24.00"}]`), Diagnostics: []byte(`[{"code":"fake_fixture","message":"Fixture proposal returned."}]`),
	}
	if err := repo.FinishExtraction(ctx, claimed.ID, claimed.LeaseToken, snapshot); err != nil {
		t.Fatal(err)
	}
	currency, total := "USD", "25.00"
	input := HumanReviewInput{Currency: &currency, Total: &total, LineItems: []HumanLineItem{}}
	version, err := repo.SaveHumanReview(ctx, record.DocumentID, 1, input, "integration-test")
	if err != nil || version != 2 {
		t.Fatalf("SaveHumanReview() = %d, %v", version, err)
	}
	if _, err := repo.SaveHumanReview(ctx, record.DocumentID, 1, input, "integration-test"); !errors.Is(err, ErrStaleReviewVersion) {
		t.Fatalf("stale SaveHumanReview() error = %v", err)
	}
	var extractionProposal, extractionWarnings, humanEvidence, humanDiagnostics []byte
	var versions, savedAudits int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT proposal FROM invoice_versions WHERE document_id=$1 AND version_number=1),
		(SELECT warnings FROM invoice_versions WHERE document_id=$1 AND version_number=1),
		(SELECT evidence FROM invoice_versions WHERE document_id=$1 AND version_number=2),
		(SELECT diagnostics FROM invoice_versions WHERE document_id=$1 AND version_number=2),
		(SELECT count(*) FROM invoice_versions WHERE document_id=$1),
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='human_review_saved')`, record.DocumentID).Scan(&extractionProposal, &extractionWarnings, &humanEvidence, &humanDiagnostics, &versions, &savedAudits); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(extractionProposal), "Fictional Vendor") || !strings.Contains(string(extractionWarnings), "missing_due_date") || !strings.Contains(string(humanEvidence), "24.00") || !strings.Contains(string(humanDiagnostics), "fake_fixture") || versions != 2 || savedAudits != 1 {
		t.Fatalf("snapshots extraction=%s warnings=%s human evidence=%s diagnostics=%s versions=%d audits=%d", extractionProposal, extractionWarnings, humanEvidence, humanDiagnostics, versions, savedAudits)
	}
	if err := repo.RejectDocument(ctx, record.DocumentID, "integration-test"); err != nil {
		t.Fatal(err)
	}
	if err := repo.RejectDocument(ctx, record.DocumentID, "integration-test"); !errors.Is(err, ErrInvalidDocumentState) {
		t.Fatalf("second RejectDocument() error = %v", err)
	}
	var status string
	var rejectedAudits int
	if err := pool.QueryRow(ctx, `SELECT d.status,(SELECT count(*) FROM audit_events WHERE document_id=d.id AND action='document_rejected') FROM documents d WHERE d.id=$1`, record.DocumentID).Scan(&status, &rejectedAudits); err != nil {
		t.Fatal(err)
	}
	if status != "rejected" || rejectedAudits != 1 {
		t.Fatalf("rejection status=%q audit=%d", status, rejectedAudits)
	}
	if _, err := repo.SaveHumanReview(ctx, record.DocumentID, 2, input, "integration-test"); !errors.Is(err, ErrInvalidDocumentState) {
		t.Fatalf("SaveHumanReview after rejection error = %v", err)
	}
	detail, err := repo.GetReviewDocument(ctx, record.DocumentID)
	if err != nil || len(detail.Versions) != 2 || detail.Versions[0].VersionNumber != 2 || detail.Versions[0].Editable.Total != "25.00" || len(detail.Audit) != 5 {
		t.Fatalf("GetReviewDocument() = %+v, %v", detail, err)
	}
}

func ptrInt64(value int64) *int64 { return &value }

type integrationOrphanStorage struct {
	keys    []string
	deleted []string
}

func (s *integrationOrphanStorage) ListObjectsOlderThan(context.Context, time.Time) ([]string, error) {
	return s.keys, nil
}
func (s *integrationOrphanStorage) Delete(_ context.Context, key string) error {
	s.deleted = append(s.deleted, key)
	return nil
}

func TestReconcileOrphanedObjectsKeepsReferencedOriginals(t *testing.T) {
	repo, _, ctx := integrationRepository(t)
	record := integrationRecord(t, "orphan-reconcile")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	storage := &integrationOrphanStorage{keys: []string{record.ObjectKey, "objects/0123456789abcdef0123456789abcdef.pdf"}}
	removed, err := repo.ReconcileOrphanedObjects(ctx, storage, time.Hour)
	if err != nil || removed != 1 {
		t.Fatalf("ReconcileOrphanedObjects() = %d, %v", removed, err)
	}
	if len(storage.deleted) != 1 || storage.deleted[0] != "objects/0123456789abcdef0123456789abcdef.pdf" {
		t.Fatalf("unexpected deletes: %v", storage.deleted)
	}
}
