package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/reinhlord/invoiceflow/internal/processing"
	"github.com/reinhlord/invoiceflow/internal/webui"
)

type fakePinger struct{ err error }

func (p fakePinger) Ping(context.Context) error { return p.err }

func TestHealthAndReadiness(t *testing.T) {
	h := newHandler(fakePinger{})
	for _, path := range []string{"/healthz", "/readyz"} {
		r := httptest.NewRecorder()
		h.ServeHTTP(r, httptest.NewRequest(http.MethodGet, path, nil))
		if r.Code != http.StatusOK {
			t.Fatalf("%s: got %d", path, r.Code)
		}
	}
	r := httptest.NewRecorder()
	newHandler(fakePinger{errors.New("down")}).ServeHTTP(r, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if r.Code != http.StatusServiceUnavailable {
		t.Fatalf("got %d", r.Code)
	}
}

type fakeReview struct {
	detail   processing.ReviewDocument
	err      error
	saved    bool
	rejected bool
}

func (f *fakeReview) GetReviewDocument(context.Context, string) (processing.ReviewDocument, error) {
	return f.detail, f.err
}
func (f *fakeReview) LoadReviewSource(context.Context, string) (processing.SourceDocument, error) {
	return processing.SourceDocument{ObjectKey: "objects/0123456789abcdef0123456789abcdef.pdf", MediaType: "application/pdf"}, f.err
}
func (f *fakeReview) SaveHumanReview(_ context.Context, _ string, base int, input processing.HumanReviewInput, _ string) (int, error) {
	f.saved = base == 1 && input.Currency != nil
	return 2, f.err
}
func (f *fakeReview) RejectDocument(context.Context, string, string) error {
	f.rejected = true
	return f.err
}
func (f *fakeReview) ApproveDocument(_ context.Context, _ string, ver int, _ string) (int, error) {
	return ver, f.err
}
func (f *fakeReview) ExportCSV(context.Context, string, string) ([]byte, error) {
	return []byte("supplier_name,supplier_email\nVendor,vendor@test\n"), f.err
}
func (f *fakeReview) ApprovedVersionNumber(context.Context, string) (int, error) { return 1, f.err }
func (f *fakeReview) EnqueueWebhookExport(_ context.Context, docID, _ string) (processing.ExportRecord, error) {
	return processing.ExportRecord{ID: "exp-1", DocumentID: docID, ExportType: "webhook", Status: "pending", DestinationRef: "server:webhook:v1", DestinationLabel: "Server-configured webhook"}, f.err
}

func TestReviewRoutesUseStableShapesAndTransitions(t *testing.T) {
	id := "0d0c2342-2486-4f10-a858-e75bc763f3e4"
	editable := processing.EditableProposal{Currency: "USD", Total: "24.00", LineItems: []processing.EditableLineItem{}}
	review := &fakeReview{detail: processing.ReviewDocument{ID: id, Status: "needs_review", MediaType: "application/pdf", Versions: []processing.ReviewVersion{{VersionNumber: 1, Source: "extraction", Proposal: json.RawMessage(`{}`), Normalized: json.RawMessage(`{}`), Warnings: json.RawMessage(`[]`), Evidence: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`[]`), RoundingPolicyVersion: "money-v1", Editable: editable}}}}
	h := newHandlerWithDependencies(apiDependencies{db: fakePinger{}, review: review, actor: "local-demo", webhookConfigured: true})
	get := httptest.NewRecorder()
	h.ServeHTTP(get, httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+id, nil))
	if get.Code != http.StatusOK || !strings.Contains(get.Body.String(), `"media_type":"application/pdf"`) || get.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("detail = %d %s headers=%v", get.Code, get.Body.String(), get.Header())
	}
	save := httptest.NewRecorder()
	h.ServeHTTP(save, httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+id+"/human-reviews", strings.NewReader(`{"base_version":1,"proposal":{"currency":"USD","total":"24.00","line_items":[]}}`)))
	if save.Code != http.StatusCreated || !review.saved {
		t.Fatalf("save = %d %s saved=%v", save.Code, save.Body.String(), review.saved)
	}
	approve := httptest.NewRecorder()
	h.ServeHTTP(approve, httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+id+"/approve", strings.NewReader(`{"version_number":1,"confirm":true}`)))
	if approve.Code != http.StatusOK || !strings.Contains(approve.Body.String(), `"status":"approved"`) || approve.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("approve = %d %s", approve.Code, approve.Body.String())
	}
	csvRec := httptest.NewRecorder()
	h.ServeHTTP(csvRec, httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+id+"/export/csv", nil))
	if csvRec.Code != http.StatusOK || !strings.Contains(csvRec.Body.String(), "supplier_name") || csvRec.Header().Get("Content-Type") != "text/csv; charset=utf-8" || csvRec.Header().Get("X-InvoiceFlow-CSV-Format") != "csv-v1" || csvRec.Header().Get("Content-Disposition") != `attachment; filename="invoice-`+id+`-v1.csv"` {
		t.Fatalf("csv export = %d %s", csvRec.Code, csvRec.Body.String())
	}
	whRec := httptest.NewRecorder()
	h.ServeHTTP(whRec, httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+id+"/export/webhook", strings.NewReader(`{}`)))
	if whRec.Code != http.StatusAccepted || !strings.Contains(whRec.Body.String(), `"status":"pending"`) || whRec.Header().Get("X-Content-Type-Options") != "nosniff" {
		t.Fatalf("webhook export = %d %s", whRec.Code, whRec.Body.String())
	}
	reject := httptest.NewRecorder()
	h.ServeHTTP(reject, httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+id+"/reject", strings.NewReader(`{"confirm":true}`)))
	if reject.Code != http.StatusNoContent || !review.rejected {
		t.Fatalf("reject = %d %s rejected=%v", reject.Code, reject.Body.String(), review.rejected)
	}
	invalid := httptest.NewRecorder()
	h.ServeHTTP(invalid, httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+id+"/reject", strings.NewReader(`{}`)))
	if invalid.Code != http.StatusBadRequest || !strings.Contains(invalid.Body.String(), "invalid_rejection") {
		t.Fatalf("invalid reject = %d %s", invalid.Code, invalid.Body.String())
	}
}

func TestApprovalAndExportErrorsCarryNoStoreSecurityHeaders(t *testing.T) {
	id := "0d0c2342-2486-4f10-a858-e75bc763f3e4"
	review := &fakeReview{err: errors.New("database unavailable")}
	h := newHandlerWithDependencies(apiDependencies{db: fakePinger{}, review: review, actor: "local-demo", webhookConfigured: true})
	for name, request := range map[string]*http.Request{
		"approval": httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+id+"/approve", strings.NewReader(`{"version_number":1,"confirm":true}`)),
		"csv":      httptest.NewRequest(http.MethodGet, "/api/v1/documents/"+id+"/export/csv", nil),
		"webhook":  httptest.NewRequest(http.MethodPost, "/api/v1/documents/"+id+"/export/webhook", strings.NewReader(`{}`)),
	} {
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code != http.StatusInternalServerError || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("X-Content-Type-Options") != "nosniff" {
			t.Fatalf("%s error response=%d headers=%v body=%s", name, response.Code, response.Header(), response.Body.String())
		}
	}
}

func webBundleHandler(t *testing.T, review reviewService) http.Handler {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatalf("create assets directory: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("<!doctype html><title>InvoiceFlow</title>"), 0o644); err != nil {
		t.Fatalf("write shell: %v", err)
	}
	bundle, err := webui.Load(root)
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	return newHandlerWithDependencies(apiDependencies{db: fakePinger{}, review: review, web: bundle, actor: "local-demo"})
}

// The browser bundle is registered last and only on GET "/", so it must never
// take priority over an API or health route, and an unknown server route must
// still answer with the JSON envelope rather than the application shell.
func TestWebBundleDoesNotShadowAPIOrHealthRoutes(t *testing.T) {
	handler := webBundleHandler(t, &fakeReview{})

	for _, path := range []string{"/healthz", "/readyz"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s: status = %d, want 200", path, recorder.Code)
		}
		if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
			t.Fatalf("%s: content type = %q", path, got)
		}
	}

	for _, path := range []string{"/api/v1/unknown", "/api/", "/healthz/extra"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusNotFound {
			t.Fatalf("%s: status = %d, want 404", path, recorder.Code)
		}
		if strings.Contains(recorder.Body.String(), "<!doctype html>") {
			t.Fatalf("%s: answered with the application shell", path)
		}
		var body apiError
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil || body.Error.Code != "route_not_found" {
			t.Fatalf("%s: body = %q err = %v", path, recorder.Body.String(), err)
		}
	}
}

func TestWebBundleServesTheShellForClientRoutedPaths(t *testing.T) {
	handler := webBundleHandler(t, &fakeReview{})
	for _, path := range []string{"/", "/app", "/app/documents/0d0c2342-2486-4f10-a858-e75bc763f3e4"} {
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, path, nil))
		if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), "InvoiceFlow") {
			t.Fatalf("%s: status = %d body = %q", path, recorder.Code, recorder.Body.String())
		}
	}
}

// Without a configured bundle the process stays API-only, which keeps the
// pre-ADR-013 behavior available for deployments that front their own static host.
func TestUnconfiguredBundleLeavesTheAPIWithoutStaticRoutes(t *testing.T) {
	recorder := httptest.NewRecorder()
	newHandler(fakePinger{}).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", recorder.Code)
	}
	if strings.Contains(recorder.Body.String(), "<!doctype html>") {
		t.Fatal("an API-only process served an application shell")
	}
}
