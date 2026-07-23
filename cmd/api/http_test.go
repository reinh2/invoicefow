package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/reinhlord/invoiceflow/internal/processing"
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

func TestReviewRoutesUseStableShapesAndTransitions(t *testing.T) {
	id := "0d0c2342-2486-4f10-a858-e75bc763f3e4"
	editable := processing.EditableProposal{Currency: "USD", Total: "24.00", LineItems: []processing.EditableLineItem{}}
	review := &fakeReview{detail: processing.ReviewDocument{ID: id, Status: "needs_review", MediaType: "application/pdf", Versions: []processing.ReviewVersion{{VersionNumber: 1, Source: "extraction", Proposal: json.RawMessage(`{}`), Normalized: json.RawMessage(`{}`), Warnings: json.RawMessage(`[]`), Evidence: json.RawMessage(`[]`), Diagnostics: json.RawMessage(`[]`), RoundingPolicyVersion: "money-v1", Editable: editable}}}}
	h := newHandlerWithDependencies(apiDependencies{db: fakePinger{}, review: review, actor: "local-demo"})
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
