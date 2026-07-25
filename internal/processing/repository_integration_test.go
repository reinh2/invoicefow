//go:build integration

package processing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/reinhlord/invoiceflow/internal/export"
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
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status='succeeded', next_attempt_at=NULL WHERE document_id=$1`, record.DocumentID); err != nil {
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
	if _, err := pool.Exec(ctx, `UPDATE jobs SET status='succeeded', next_attempt_at=NULL WHERE document_id IN (SELECT id FROM documents WHERE sha256=$1)`, first.SHA256[:]); err != nil {
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
		outcome, err := repo.FinishRetry(ctx, job.ID, job.LeaseToken, summary, 0)
		if err != nil {
			t.Fatalf("FinishRetry %d: %v", attempt, err)
		}
		// The fifth attempt exhausts max_attempts, so only it may report a
		// dead letter. An earlier dead letter would mean the budget shrank.
		want := OutcomeRetry
		if attempt == 5 {
			want = OutcomeDeadLetter
		}
		if outcome != want {
			t.Fatalf("FinishRetry %d reported %q, want %q", attempt, outcome, want)
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
	// Every required field is present so the assertion below stays a check that
	// a consistent invoice produces *no* warnings at all.
	invoiceNumber, issueDate := "WORKER-001", "2026-07-01"
	fake, err := extraction.NewFakeStructuredExtractor([]extraction.FakeFixture{{DocumentSHA256: fmt.Sprintf("%x", record.SHA256), Marker: "INVOICEFLOW_FIXTURE:WORKER-001", Proposal: extraction.Proposal{SupplierName: &supplier, InvoiceNumber: &invoiceNumber, IssueDate: &issueDate, Currency: &currency, Subtotal: &subtotal, TaxAmount: &tax, Total: &total, LineItems: []extraction.LineItemProposal{{Quantity: &quantity, UnitPrice: &price, Total: &lineTotal}}}}})
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

func ptrInt64(value int64) *int64    { return &value }
func ptrString(value string) *string { return &value }

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

func TestStage5ApprovalAndExportFlow(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	repo.WithWebhookURL("https://example.test/webhook")

	record := integrationRecord(t, "stage5-approval-export")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimReady(ctx, time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("ClaimReady() = %+v, %v", claimed, err)
	}
	snapshot := ExtractionSnapshot{
		Currency: "USD", TotalMinor: ptrInt64(2400), RoundingPolicyVersion: "money-v1",
		Proposal:   json.RawMessage(`{"supplier_name":"Vendor Inc","total":"24.00"}`),
		Normalized: json.RawMessage(`{"rounding_policy_version":"money-v1","currency":"USD","total_minor":2400,"supplier_name":"Vendor Inc"}`),
		Warnings:   json.RawMessage(`[]`), Evidence: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`[]`),
	}
	if err := repo.FinishExtraction(ctx, claimed.ID, claimed.LeaseToken, snapshot); err != nil {
		t.Fatal(err)
	}

	// Save human review version 2
	input := HumanReviewInput{
		SupplierName: ptrString("Vendor Inc Corrected"),
		Currency:     ptrString("USD"),
		Total:        ptrString("24.00"),
		LineItems:    []HumanLineItem{},
	}
	v2, err := repo.SaveHumanReview(ctx, record.DocumentID, 1, input, "test-editor")
	if err != nil || v2 != 2 {
		t.Fatalf("SaveHumanReview() = %d, %v", v2, err)
	}

	// Attempting to approve version 1 when version 2 exists MUST fail with ErrStaleReviewVersion
	if _, err := repo.ApproveDocument(ctx, record.DocumentID, 1, "test-approver"); !errors.Is(err, ErrStaleReviewVersion) {
		t.Fatalf("approving stale version 1 error = %v, want ErrStaleReviewVersion", err)
	}

	// Approve current version 2
	ver, err := repo.ApproveDocument(ctx, record.DocumentID, 2, "test-approver")
	if err != nil || ver != 2 {
		t.Fatalf("ApproveDocument() = %d, %v", ver, err)
	}

	// Cannot approve again
	if _, err := repo.ApproveDocument(ctx, record.DocumentID, 2, "test-approver"); !errors.Is(err, ErrInvalidDocumentState) {
		t.Fatalf("second ApproveDocument() = %v, want ErrInvalidDocumentState", err)
	}

	// Export CSV
	csvData, err := repo.ExportCSV(ctx, record.DocumentID, "test-exporter")
	if err != nil {
		t.Fatalf("ExportCSV() error = %v", err)
	}
	if !strings.Contains(string(csvData), "Vendor Inc Corrected") {
		t.Fatalf("unexpected CSV data: %s", string(csvData))
	}

	// Repeat CSV export (idempotent)
	csvData2, err := repo.ExportCSV(ctx, record.DocumentID, "test-exporter")
	if err != nil || string(csvData2) != string(csvData) {
		t.Fatalf("idempotent ExportCSV() error = %v", err)
	}

	// Enqueue Webhook Export
	exportRec, err := repo.EnqueueWebhookExport(ctx, record.DocumentID, "test-exporter")
	if err != nil || exportRec.Status != "pending" {
		t.Fatalf("EnqueueWebhookExport() = %+v, %v", exportRec, err)
	}
	exportRecAgain, err := repo.EnqueueWebhookExport(ctx, record.DocumentID, "test-exporter")
	if err != nil || exportRecAgain.ID != exportRec.ID {
		t.Fatalf("repeated webhook enqueue = %+v, %v; expected the same durable export record", exportRecAgain, err)
	}

	// Claim and execute export job
	jobClaimed, err := repo.ClaimExportReady(ctx, time.Minute)
	if err != nil || jobClaimed == nil {
		t.Fatalf("ClaimExportReady() = %+v, %v", jobClaimed, err)
	}
	details, err := repo.LoadExportJobDetails(ctx, jobClaimed.ID)
	if err != nil || details.DestinationRef != "server:webhook:v1" || details.DestinationLabel != "Server-configured webhook" {
		t.Fatalf("LoadExportJobDetails() = %+v, %v", details, err)
	}
	if details.IdempotencyKey != exportRec.IdempotencyKey || exportRecAgain.IdempotencyKey != exportRec.IdempotencyKey {
		t.Fatalf("persisted idempotency key changed: record=%q repeated=%q details=%q", exportRec.IdempotencyKey, exportRecAgain.IdempotencyKey, details.IdempotencyKey)
	}
	if err := repo.FinishExportSuccess(ctx, jobClaimed.ID, jobClaimed.LeaseToken, details.ExportID, "test-worker"); err != nil {
		t.Fatalf("FinishExportSuccess() error = %v", err)
	}

	// Check final document detail
	detail, err := repo.GetReviewDocument(ctx, record.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	if detail.Status != "exported" || detail.ApprovedVersionNumber == nil || *detail.ApprovedVersionNumber != 2 || len(detail.Exports) == 0 {
		t.Fatalf("final GetReviewDocument() = %+v", detail)
	}

	var approvedAudits, csvAudits, exportEnqueuedAudits, webhookExportedAudits int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='document_approved'),
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='csv_exported'),
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='export_enqueued'),
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='webhook_exported')`, record.DocumentID).Scan(&approvedAudits, &csvAudits, &exportEnqueuedAudits, &webhookExportedAudits); err != nil {
		t.Fatal(err)
	}
	if approvedAudits != 1 || csvAudits != 1 || exportEnqueuedAudits != 1 || webhookExportedAudits != 1 {
		t.Fatalf("audits: approved=%d csv=%d enqueued=%d webhook=%d", approvedAudits, csvAudits, exportEnqueuedAudits, webhookExportedAudits)
	}
}

func TestExportHistoryDatabaseFailureIsReturned(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record := integrationRecord(t, "export-history-db-failure")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimReady(ctx, time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim processing job: %+v %v", claimed, err)
	}
	if err := repo.FinishExtraction(ctx, claimed.ID, claimed.LeaseToken, ExtractionSnapshot{Currency: "USD", TotalMinor: ptrInt64(100), RoundingPolicyVersion: "money-v1", Proposal: json.RawMessage(`{}`), Normalized: json.RawMessage(`{"currency":"USD","total_minor":100` + `}`), Warnings: json.RawMessage(`[]`), Evidence: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`[]`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `ALTER TABLE exports RENAME TO exports_unavailable`); err != nil {
		t.Fatal(err)
	}
	defer func() { _, _ = pool.Exec(context.Background(), `ALTER TABLE exports_unavailable RENAME TO exports`) }()
	if _, err := repo.GetReviewDocument(ctx, record.DocumentID); err == nil {
		t.Fatal("GetReviewDocument returned partial success after export history query failed")
	}
}

func TestExportCompositeForeignKeyRejectsCrossDocumentVersion(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	first, second := integrationRecord(t, "export-cross-document-first"), integrationRecord(t, "export-cross-document-second")
	for _, record := range []IntakeRecord{first, second} {
		if err := repo.CreateQueuedDocument(ctx, record); err != nil {
			t.Fatal(err)
		}
		claimed, err := repo.ClaimReady(ctx, time.Minute)
		if err != nil || claimed == nil {
			t.Fatalf("claim processing job: %+v %v", claimed, err)
		}
		if err := repo.FinishExtraction(ctx, claimed.ID, claimed.LeaseToken, ExtractionSnapshot{Currency: "USD", TotalMinor: ptrInt64(100), RoundingPolicyVersion: "money-v1", Proposal: json.RawMessage(`{}`), Normalized: json.RawMessage(`{"currency":"USD","total_minor":100` + `}`), Warnings: json.RawMessage(`[]`), Evidence: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`[]`)}); err != nil {
			t.Fatal(err)
		}
	}
	var firstVersion, secondVersion string
	if err := pool.QueryRow(ctx, `SELECT id FROM invoice_versions WHERE document_id=$1`, first.DocumentID).Scan(&firstVersion); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT id FROM invoice_versions WHERE document_id=$1`, second.DocumentID).Scan(&secondVersion); err != nil {
		t.Fatal(err)
	}
	jobID, exportID := mustID(t), mustID(t)
	_, err := pool.Exec(ctx, `INSERT INTO jobs (id,document_id,job_type,status,attempts,max_attempts,next_attempt_at,idempotency_key) VALUES ($1,$2,'export_document','ready',0,5,now(),$3)`, jobID, first.DocumentID, "cross-document-job:"+jobID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, `INSERT INTO exports (id,document_id,version_id,export_type,status,idempotency_key,destination_ref,destination_label,job_id) VALUES ($1,$2,$3,'webhook','pending',$4,'server:webhook:v1','Server-configured webhook',$5)`, exportID, first.DocumentID, secondVersion, "cross-document-export:"+exportID, jobID); err == nil {
		t.Fatal("cross-document export version was accepted")
	}
	_ = firstVersion
}

func TestExportForeignKeyRejectsAnotherVersionOfTheSameDocument(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record := integrationRecord(t, "export-unapproved-version")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimReady(ctx, time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim processing job: %+v %v", claimed, err)
	}
	if err := repo.FinishExtraction(ctx, claimed.ID, claimed.LeaseToken, ExtractionSnapshot{Currency: "USD", TotalMinor: ptrInt64(100), RoundingPolicyVersion: "money-v1", Proposal: json.RawMessage(`{}`), Normalized: json.RawMessage(`{"currency":"USD","total_minor":100}`), Warnings: json.RawMessage(`[]`), Evidence: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`[]`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.SaveHumanReview(ctx, record.DocumentID, 1, HumanReviewInput{Currency: ptrString("USD"), Total: ptrString("1.00"), LineItems: []HumanLineItem{}}, "test-editor"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApproveDocument(ctx, record.DocumentID, 2, "test-approver"); err != nil {
		t.Fatal(err)
	}
	var firstVersion, approvedVersion string
	if err := pool.QueryRow(ctx, `SELECT id FROM invoice_versions WHERE document_id=$1 AND version_number=1`, record.DocumentID).Scan(&firstVersion); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, `SELECT approved_version_id FROM documents WHERE id=$1`, record.DocumentID).Scan(&approvedVersion); err != nil {
		t.Fatal(err)
	}
	exportID := mustID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO exports (id,document_id,version_id,export_type,status,idempotency_key,destination_ref,destination_label) VALUES ($1,$2,$3,'webhook','pending',$4,'server:webhook:v1','Server-configured webhook')`, exportID, record.DocumentID, firstVersion, "same-document-unapproved:"+exportID); err == nil {
		t.Fatal("same-document unapproved export version was accepted")
	}
	if _, err := pool.Exec(ctx, `INSERT INTO exports (id,document_id,version_id,export_type,status,idempotency_key,destination_ref,destination_label) VALUES ($1,$2,$3,'webhook','pending',$4,'server:webhook:v1','Server-configured webhook')`, exportID, record.DocumentID, approvedVersion, "same-document-approved:"+exportID); err != nil {
		t.Fatalf("approved export version was rejected: %v", err)
	}
	duplicateID := mustID(t)
	if _, err := pool.Exec(ctx, `INSERT INTO exports (id,document_id,version_id,export_type,status,idempotency_key,destination_ref,destination_label) VALUES ($1,$2,$3,'webhook','pending',$4,'server:webhook:v1','Server-configured webhook')`, duplicateID, record.DocumentID, approvedVersion, "duplicate-version-type:"+duplicateID); err == nil || !strings.Contains(err.Error(), "exports_document_version_type_unique") {
		t.Fatalf("duplicate export version/type error = %v, want uniqueness constraint", err)
	}
}

func mustID(t *testing.T) string {
	t.Helper()
	id, err := NewID()
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestExportLeaseRecoveryDoesNotTouchApprovedDocument(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	repo.WithWebhookURL("https://user:password@example.test/hook?token=secret")
	record := integrationRecord(t, "export-lease-recovery")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	claimed, err := repo.ClaimReady(ctx, time.Minute)
	if err != nil || claimed == nil {
		t.Fatalf("claim processing job: %+v %v", claimed, err)
	}
	if err := repo.FinishExtraction(ctx, claimed.ID, claimed.LeaseToken, ExtractionSnapshot{Currency: "USD", TotalMinor: ptrInt64(100), RoundingPolicyVersion: "money-v1", Proposal: json.RawMessage(`{}`), Normalized: json.RawMessage(`{"currency":"USD","total_minor":100}`), Warnings: json.RawMessage(`[]`), Evidence: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`[]`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApproveDocument(ctx, record.DocumentID, 1, "test-approver"); err != nil {
		t.Fatal(err)
	}
	exportRecord, err := repo.EnqueueWebhookExport(ctx, record.DocumentID, "test-exporter")
	if err != nil {
		t.Fatal(err)
	}
	exportJob, err := repo.ClaimExportReady(ctx, time.Minute)
	if err != nil || exportJob == nil {
		t.Fatalf("claim export job: %+v %v", exportJob, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET leased_until=now()-interval '1 second' WHERE id=$1`, exportJob.ID); err != nil {
		t.Fatal(err)
	}
	if recovered, err := repo.RecoverExpiredLeases(ctx); err != nil || recovered != 1 {
		t.Fatalf("RecoverExpiredLeases()=%d, %v", recovered, err)
	}
	var jobStatus, exportStatus, documentStatus, outcome string
	var attempts, auditCount int
	if err := pool.QueryRow(ctx, `SELECT j.status,e.status,d.status,ja.outcome,j.attempts,(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='export_retry_scheduled') FROM jobs j JOIN exports e ON e.job_id=j.id JOIN documents d ON d.id=j.document_id JOIN job_attempts ja ON ja.job_id=j.id WHERE j.id=$2`, record.DocumentID, exportJob.ID).Scan(&jobStatus, &exportStatus, &documentStatus, &outcome, &attempts, &auditCount); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "ready" || exportStatus != "retrying" || documentStatus != "approved" || outcome != "retryable_failure" || attempts != 1 || auditCount != 1 {
		t.Fatalf("recovery job=%q export=%q document=%q outcome=%q attempts=%d audits=%d", jobStatus, exportStatus, documentStatus, outcome, attempts, auditCount)
	}
	reclaimed, err := repo.ClaimExportReady(ctx, time.Minute)
	if err != nil || reclaimed == nil || reclaimed.Attempt != 2 {
		t.Fatalf("restarted worker claim=%+v err=%v, want attempt 2", reclaimed, err)
	}
	if err := repo.FinishExportSuccess(ctx, reclaimed.ID, reclaimed.LeaseToken, exportRecord.ID, "test-worker"); err != nil {
		t.Fatal(err)
	}
	var successCount int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='webhook_exported'`, record.DocumentID).Scan(&successCount); err != nil {
		t.Fatal(err)
	}
	if successCount != 1 {
		t.Fatalf("webhook success audit count=%d, want 1", successCount)
	}
}

func TestExportLeaseRecoveryDeadLettersWithoutFailingDocument(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	repo.WithWebhookURL("https://example.test/webhook")
	record := integrationRecord(t, "export-lease-dead-letter")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	processingJob, err := repo.ClaimReady(ctx, time.Minute)
	if err != nil || processingJob == nil {
		t.Fatalf("claim processing job: %+v %v", processingJob, err)
	}
	if err := repo.FinishExtraction(ctx, processingJob.ID, processingJob.LeaseToken, ExtractionSnapshot{Currency: "USD", TotalMinor: ptrInt64(100), RoundingPolicyVersion: "money-v1", Proposal: json.RawMessage(`{}`), Normalized: json.RawMessage(`{"currency":"USD","total_minor":100}`), Warnings: json.RawMessage(`[]`), Evidence: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`[]`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApproveDocument(ctx, record.DocumentID, 1, "test-approver"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.EnqueueWebhookExport(ctx, record.DocumentID, "test-exporter"); err != nil {
		t.Fatal(err)
	}
	exportJob, err := repo.ClaimExportReady(ctx, time.Minute)
	if err != nil || exportJob == nil {
		t.Fatalf("claim export job: %+v %v", exportJob, err)
	}
	if _, err := pool.Exec(ctx, `UPDATE jobs SET attempts=max_attempts,leased_until=now()-interval '1 second' WHERE id=$1`, exportJob.ID); err != nil {
		t.Fatal(err)
	}
	if recovered, err := repo.RecoverExpiredLeases(ctx); err != nil || recovered != 1 {
		t.Fatalf("RecoverExpiredLeases()=%d, %v", recovered, err)
	}
	var jobStatus, exportStatus, documentStatus, auditAction string
	if err := pool.QueryRow(ctx, `SELECT j.status,e.status,d.status,(SELECT action FROM audit_events WHERE document_id=$1 AND action='export_dead_lettered') FROM jobs j JOIN exports e ON e.job_id=j.id JOIN documents d ON d.id=j.document_id WHERE j.id=$2`, record.DocumentID, exportJob.ID).Scan(&jobStatus, &exportStatus, &documentStatus, &auditAction); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "dead_letter" || exportStatus != "dead_letter" || documentStatus != "approved" || auditAction != "export_dead_lettered" {
		t.Fatalf("dead-letter job=%q export=%q document=%q audit=%q", jobStatus, exportStatus, documentStatus, auditAction)
	}
}

func TestWebhookDestinationNeverLeavesServerOwnedURL(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	secretURL := "https://user:password@example.test/private?token=secret"
	repo.WithWebhookURL(secretURL)
	record := integrationRecord(t, "destination-redaction")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	processingJob, err := repo.ClaimReady(ctx, time.Minute)
	if err != nil || processingJob == nil {
		t.Fatalf("claim processing job: %+v %v", processingJob, err)
	}
	if err := repo.FinishExtraction(ctx, processingJob.ID, processingJob.LeaseToken, ExtractionSnapshot{Currency: "USD", TotalMinor: ptrInt64(100), RoundingPolicyVersion: "money-v1", Proposal: json.RawMessage(`{}`), Normalized: json.RawMessage(`{"currency":"USD","total_minor":100}`), Warnings: json.RawMessage(`[]`), Evidence: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`[]`)}); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApproveDocument(ctx, record.DocumentID, 1, "test-approver"); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.EnqueueWebhookExport(ctx, record.DocumentID, "test-exporter"); err != nil {
		t.Fatal(err)
	}
	detail, err := repo.GetReviewDocument(ctx, record.DocumentID)
	if err != nil {
		t.Fatal(err)
	}
	serialized, _ := json.Marshal(detail)
	if strings.Contains(string(serialized), secretURL) || strings.Contains(string(serialized), "password") || strings.Contains(string(serialized), "token=secret") || strings.Contains(string(serialized), `"version_id"`) {
		t.Fatalf("server-owned destination leaked through detail: %s", serialized)
	}
	if len(detail.Exports) != 1 || detail.Exports[0].VersionNumber != 1 || detail.Exports[0].DestinationRef != "server:webhook:v1" || detail.Exports[0].DestinationLabel != "Server-configured webhook" {
		t.Fatalf("unsafe export projection: %+v", detail.Exports)
	}
	var exportsDestination, auditPayload []byte
	if err := pool.QueryRow(ctx, `SELECT e.destination_ref,a.payload FROM exports e JOIN audit_events a ON a.document_id=e.document_id WHERE e.document_id=$1 AND a.action='export_enqueued'`, record.DocumentID).Scan(&exportsDestination, &auditPayload); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(exportsDestination), "http") || strings.Contains(string(auditPayload), "token=secret") || strings.Contains(string(auditPayload), "password") {
		t.Fatalf("unsafe destination persisted: export=%s audit=%s", exportsDestination, auditPayload)
	}
}

type webhookRequest struct {
	IdempotencyKey string
	Body           []byte
}

type webhookReceiver struct {
	statuses []int
	requests []webhookRequest
	mu       sync.Mutex
}

func (receiver *webhookReceiver) serveHTTP(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "receiver could not read request", http.StatusInternalServerError)
		return
	}
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	receiver.requests = append(receiver.requests, webhookRequest{IdempotencyKey: r.Header.Get("X-InvoiceFlow-Idempotency-Key"), Body: body})
	statusIndex := len(receiver.requests) - 1
	if statusIndex >= len(receiver.statuses) {
		statusIndex = len(receiver.statuses) - 1
	}
	status := receiver.statuses[statusIndex]
	w.WriteHeader(status)
}

func (receiver *webhookReceiver) recordedRequests() []webhookRequest {
	receiver.mu.Lock()
	defer receiver.mu.Unlock()
	return append([]webhookRequest(nil), receiver.requests...)
}

type receiverRoundTripper struct{ target *url.URL }

func (roundTripper receiverRoundTripper) RoundTrip(request *http.Request) (*http.Response, error) {
	clone := request.Clone(request.Context())
	clone.URL = roundTripper.target
	clone.Host = roundTripper.target.Host
	clone.RequestURI = ""
	return http.DefaultTransport.RoundTrip(clone)
}

func webhookWorkerForReceiver(t *testing.T, repo *Repository, serverURL string) Worker {
	t.Helper()
	target, err := url.Parse(serverURL)
	if err != nil {
		t.Fatal(err)
	}
	sender := export.NewControlledWebhookSender("integration-webhook-secret", export.ControlledWebhookDestination)
	sender.Client = &http.Client{Transport: receiverRoundTripper{target: target}, Timeout: time.Second}
	return Worker{Repository: repo, Lease: time.Minute, RetryDelay: 0, WebhookSender: sender}
}

func createApprovedWebhookExport(t *testing.T, repo *Repository, ctx context.Context) (IntakeRecord, ExportRecord) {
	t.Helper()
	repo.WithWebhookDestination(true, "server:webhook:v1", "Server-configured webhook")
	record := integrationRecord(t, "approved-webhook-export")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}
	processingJob, err := repo.ClaimReady(ctx, time.Minute)
	if err != nil || processingJob == nil {
		t.Fatalf("claim processing job: %+v %v", processingJob, err)
	}
	snapshot := ExtractionSnapshot{
		Currency: "USD", TotalMinor: ptrInt64(100), RoundingPolicyVersion: "money-v1",
		Proposal:   json.RawMessage(`{"supplier_name":"Fictional Vendor"}`),
		Normalized: json.RawMessage(`{"rounding_policy_version":"money-v1","currency":"USD","total_minor":100,"supplier_name":"Fictional Vendor"}`),
		Warnings:   json.RawMessage(`[]`), Evidence: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`[]`),
	}
	if err := repo.FinishExtraction(ctx, processingJob.ID, processingJob.LeaseToken, snapshot); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.ApproveDocument(ctx, record.DocumentID, 1, "test-approver"); err != nil {
		t.Fatal(err)
	}
	exportRecord, err := repo.EnqueueWebhookExport(ctx, record.DocumentID, "test-exporter")
	if err != nil {
		t.Fatal(err)
	}
	return record, exportRecord
}

func TestExportWorkerRetries429And5xxWithStablePayloadThenSucceeds(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record, exportRecord := createApprovedWebhookExport(t, repo, ctx)
	receiver := &webhookReceiver{statuses: []int{http.StatusTooManyRequests, http.StatusInternalServerError, http.StatusNoContent}}
	server := httptest.NewServer(http.HandlerFunc(receiver.serveHTTP))
	t.Cleanup(server.Close)
	worker := webhookWorkerForReceiver(t, repo, server.URL)

	for attempt := 1; attempt <= 3; attempt++ {
		worked, err := worker.RunExportOnce(ctx)
		if err != nil || !worked {
			t.Fatalf("RunExportOnce attempt %d worked=%t err=%v", attempt, worked, err)
		}
		var documentStatus, exportStatus string
		var jobAttempts, exportAttempts int
		var jobNext, exportNext *time.Time
		if err := pool.QueryRow(ctx, `SELECT d.status,e.status,j.attempts,e.attempts,j.next_attempt_at,e.next_attempt_at FROM documents d JOIN jobs j ON j.document_id=d.id JOIN exports e ON e.job_id=j.id WHERE e.id=$1`, exportRecord.ID).Scan(&documentStatus, &exportStatus, &jobAttempts, &exportAttempts, &jobNext, &exportNext); err != nil {
			t.Fatal(err)
		}
		if jobAttempts != attempt || exportAttempts != attempt {
			t.Fatalf("attempt %d persisted job=%d export=%d", attempt, jobAttempts, exportAttempts)
		}
		if attempt < 3 {
			if documentStatus != "approved" || exportStatus != "retrying" || jobNext == nil || exportNext == nil {
				t.Fatalf("transient attempt %d document=%q export=%q job_next=%v export_next=%v", attempt, documentStatus, exportStatus, jobNext, exportNext)
			}
			continue
		}
		if documentStatus != "exported" || exportStatus != "succeeded" || jobNext != nil || exportNext != nil {
			t.Fatalf("success document=%q export=%q job_next=%v export_next=%v", documentStatus, exportStatus, jobNext, exportNext)
		}
	}

	requests := receiver.recordedRequests()
	if len(requests) != 3 {
		t.Fatalf("receiver requests=%d, want 3", len(requests))
	}
	for index, request := range requests {
		if request.IdempotencyKey != exportRecord.IdempotencyKey || !bytes.Equal(request.Body, requests[0].Body) {
			t.Fatalf("request %d did not retain canonical idempotency payload", index+1)
		}
	}
	var retryAudits, successAudits, deadLetterAudits int
	if err := pool.QueryRow(ctx, `SELECT
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='export_retry_scheduled'),
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='webhook_exported'),
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='export_dead_lettered')`, record.DocumentID).Scan(&retryAudits, &successAudits, &deadLetterAudits); err != nil {
		t.Fatal(err)
	}
	if retryAudits != 2 || successAudits != 1 || deadLetterAudits != 0 {
		t.Fatalf("audit counts retry=%d success=%d dead_letter=%d", retryAudits, successAudits, deadLetterAudits)
	}
}

func TestExportWorkerDeadLettersRetryableFailuresAtAttemptLimit(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record, exportRecord := createApprovedWebhookExport(t, repo, ctx)
	receiver := &webhookReceiver{statuses: []int{http.StatusTooManyRequests}}
	server := httptest.NewServer(http.HandlerFunc(receiver.serveHTTP))
	t.Cleanup(server.Close)
	worker := webhookWorkerForReceiver(t, repo, server.URL)

	for attempt := 1; attempt <= 5; attempt++ {
		worked, err := worker.RunExportOnce(ctx)
		if err != nil || !worked {
			t.Fatalf("RunExportOnce attempt %d worked=%t err=%v", attempt, worked, err)
		}
	}
	if worked, err := worker.RunExportOnce(ctx); err != nil || worked {
		t.Fatalf("RunExportOnce after dead-letter worked=%t err=%v", worked, err)
	}
	var jobStatus, exportStatus, documentStatus, jobError, exportError string
	var jobAttempts, exportAttempts, retryAudits, deadLetterAudits, retryOutcomes, permanentOutcomes int
	var jobNext, exportNext *time.Time
	if err := pool.QueryRow(ctx, `SELECT
		j.status,e.status,d.status,COALESCE(j.last_error,''),COALESCE(e.error_summary,''),j.attempts,e.attempts,j.next_attempt_at,e.next_attempt_at,
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='export_retry_scheduled'),
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='export_dead_lettered'),
		(SELECT count(*) FROM job_attempts WHERE job_id=j.id AND outcome='retryable_failure'),
		(SELECT count(*) FROM job_attempts WHERE job_id=j.id AND outcome='permanent_failure')
		FROM jobs j JOIN exports e ON e.job_id=j.id JOIN documents d ON d.id=j.document_id WHERE e.id=$2`, record.DocumentID, exportRecord.ID).Scan(&jobStatus, &exportStatus, &documentStatus, &jobError, &exportError, &jobAttempts, &exportAttempts, &jobNext, &exportNext, &retryAudits, &deadLetterAudits, &retryOutcomes, &permanentOutcomes); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "dead_letter" || exportStatus != "dead_letter" || documentStatus != "approved" || jobAttempts != 5 || exportAttempts != 5 || jobNext != nil || exportNext != nil || jobError != "webhook delivery temporary failure" || exportError != "webhook delivery failed (exhausted retries)" || retryAudits != 4 || deadLetterAudits != 1 || retryOutcomes != 4 || permanentOutcomes != 1 {
		t.Fatalf("dead-letter job=%q export=%q document=%q attempts=%d/%d next=%v/%v errors=%q/%q audits=%d/%d outcomes=%d/%d", jobStatus, exportStatus, documentStatus, jobAttempts, exportAttempts, jobNext, exportNext, jobError, exportError, retryAudits, deadLetterAudits, retryOutcomes, permanentOutcomes)
	}
	requests := receiver.recordedRequests()
	if len(requests) != 5 {
		t.Fatalf("receiver requests=%d, want 5", len(requests))
	}
	for index, request := range requests {
		if request.IdempotencyKey != exportRecord.IdempotencyKey || !bytes.Equal(request.Body, requests[0].Body) {
			t.Fatalf("request %d did not retain canonical idempotency payload", index+1)
		}
	}
}

func TestExportWorkerDeadLettersPermanent4xxWithoutChangingApprovedDocument(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)
	record, exportRecord := createApprovedWebhookExport(t, repo, ctx)
	receiver := &webhookReceiver{statuses: []int{http.StatusBadRequest}}
	server := httptest.NewServer(http.HandlerFunc(receiver.serveHTTP))
	t.Cleanup(server.Close)
	worked, err := webhookWorkerForReceiver(t, repo, server.URL).RunExportOnce(ctx)
	if err != nil || !worked {
		t.Fatalf("RunExportOnce worked=%t err=%v", worked, err)
	}
	var jobStatus, exportStatus, documentStatus, jobError, exportError string
	var jobAttempts, exportAttempts, deadLetterAudits int
	var jobNext, exportNext *time.Time
	if err := pool.QueryRow(ctx, `SELECT
		j.status,e.status,d.status,COALESCE(j.last_error,''),COALESCE(e.error_summary,''),j.attempts,e.attempts,j.next_attempt_at,e.next_attempt_at,
		(SELECT count(*) FROM audit_events WHERE document_id=$1 AND action='export_dead_lettered')
		FROM jobs j JOIN exports e ON e.job_id=j.id JOIN documents d ON d.id=j.document_id WHERE e.id=$2`, record.DocumentID, exportRecord.ID).Scan(&jobStatus, &exportStatus, &documentStatus, &jobError, &exportError, &jobAttempts, &exportAttempts, &jobNext, &exportNext, &deadLetterAudits); err != nil {
		t.Fatal(err)
	}
	if jobStatus != "dead_letter" || exportStatus != "dead_letter" || documentStatus != "approved" || jobAttempts != 1 || exportAttempts != 1 || jobNext != nil || exportNext != nil || jobError != "webhook delivery failed (permanent)" || exportError != "webhook delivery failed (permanent)" || deadLetterAudits != 1 {
		t.Fatalf("permanent job=%q export=%q document=%q attempts=%d/%d next=%v/%v errors=%q/%q audits=%d", jobStatus, exportStatus, documentStatus, jobAttempts, exportAttempts, jobNext, exportNext, jobError, exportError, deadLetterAudits)
	}
}

func TestListDocumentsPagesWithAStableKeysetCursor(t *testing.T) {
	repo, pool, ctx := integrationRepository(t)

	// Distinct created_at values so the newest-first ordering is unambiguous.
	base := time.Now().UTC().Add(-time.Hour)
	ids := make([]string, 0, 5)
	for index := 0; index < 5; index++ {
		record := integrationRecord(t, fmt.Sprintf("list-%d", index))
		record.CreatedAt = base.Add(time.Duration(index) * time.Minute)
		if err := repo.CreateQueuedDocument(ctx, record); err != nil {
			t.Fatal(err)
		}
		if _, err := pool.Exec(ctx, `UPDATE documents SET created_at=$2 WHERE id=$1`, record.DocumentID, record.CreatedAt); err != nil {
			t.Fatal(err)
		}
		ids = append(ids, record.DocumentID)
	}

	first, err := repo.ListDocuments(ctx, 2, "")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(first.Documents) != 2 || first.NextCursor == "" {
		t.Fatalf("first page = %d documents, cursor %q", len(first.Documents), first.NextCursor)
	}
	// Newest first: the last seeded document leads.
	if first.Documents[0].ID != ids[4] || first.Documents[1].ID != ids[3] {
		t.Errorf("first page order = %s, %s; want %s, %s", first.Documents[0].ID, first.Documents[1].ID, ids[4], ids[3])
	}

	// Inserting a newer document mid-paging must not shift the next page, which
	// is exactly what offset pagination would get wrong.
	newer := integrationRecord(t, "list-newer")
	if err := repo.CreateQueuedDocument(ctx, newer); err != nil {
		t.Fatal(err)
	}

	second, err := repo.ListDocuments(ctx, 2, first.NextCursor)
	if err != nil {
		t.Fatalf("second page: %v", err)
	}
	if len(second.Documents) != 2 || second.Documents[0].ID != ids[2] || second.Documents[1].ID != ids[1] {
		t.Fatalf("second page = %+v; want %s then %s", second.Documents, ids[2], ids[1])
	}

	seen := map[string]bool{}
	for _, document := range append(first.Documents, second.Documents...) {
		if seen[document.ID] {
			t.Errorf("document %s repeated across pages", document.ID)
		}
		seen[document.ID] = true
	}

	if _, err := repo.ListDocuments(ctx, 2, "not-a-cursor"); !errors.Is(err, ErrInvalidCursor) {
		t.Errorf("foreign cursor error = %v, want ErrInvalidCursor", err)
	}
}

func TestListDocumentsClampsPageSizeAndExposesNoServerIdentifiers(t *testing.T) {
	repo, _, ctx := integrationRepository(t)
	record := integrationRecord(t, "list-bounds")
	if err := repo.CreateQueuedDocument(ctx, record); err != nil {
		t.Fatal(err)
	}

	// A caller cannot request an unbounded scan.
	page, err := repo.ListDocuments(ctx, 100_000, "")
	if err != nil {
		t.Fatalf("ListDocuments: %v", err)
	}
	if len(page.Documents) > maxDocumentPageSize {
		t.Errorf("page size = %d, want at most %d", len(page.Documents), maxDocumentPageSize)
	}

	encoded, err := json.Marshal(page)
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{record.ObjectKey, record.ObjectID, "sha256", "object_key"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Errorf("list projection leaked %q: %s", forbidden, encoded)
		}
	}
}
