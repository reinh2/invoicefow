package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"net/textproto"
	"testing"

	"github.com/reinhlord/invoiceflow/internal/processing"
)

type fakeIntake struct {
	err error
	got processing.IntakeRecord
}

func (f *fakeIntake) CreateQueuedDocument(_ context.Context, r processing.IntakeRecord) error {
	f.got = r
	return f.err
}

type memoryStorage struct {
	put, deleted bool
	err          error
}

func (s *memoryStorage) Put(_ context.Context, _ string, r io.Reader, _ int64) error {
	_, _ = io.ReadAll(r)
	s.put = true
	return s.err
}
func (s *memoryStorage) Open(context.Context, string) (io.ReadCloser, error) {
	return nil, errors.New("not implemented")
}
func (s *memoryStorage) Delete(context.Context, string) error { s.deleted = true; return nil }
func uploadRequest(t *testing.T) *http.Request {
	t.Helper()
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	h := textproto.MIMEHeader{"Content-Disposition": {`form-data; name="file"; filename="fictional.pdf"`}, "Content-Type": {"application/pdf"}}
	p, err := w.CreatePart(h)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = p.Write([]byte("%PDF-1.7\nfictional\n%%EOF\n"))
	_ = w.Close()
	r := httptest.NewRequest("POST", "/api/v1/documents", &b)
	r.Header.Set("Content-Type", w.FormDataContentType())
	return r
}
func TestUploadQueuesDocument(t *testing.T) {
	intake := &fakeIntake{}
	storage := &memoryStorage{}
	h := newHandlerWithDependencies(apiDependencies{db: fakePinger{}, intake: intake, storage: storage, actor: "local-demo", tempDir: t.TempDir()})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, uploadRequest(t))
	if rr.Code != 201 {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if !storage.put || intake.got.MediaType != "application/pdf" || intake.got.ObjectKey == "" {
		t.Fatalf("not queued: %+v", intake.got)
	}
}
func TestUploadDuplicateIsOpaqueConflict(t *testing.T) {
	intake := &fakeIntake{err: processing.ErrDuplicate}
	storage := &memoryStorage{}
	h := newHandlerWithDependencies(apiDependencies{db: fakePinger{}, intake: intake, storage: storage, tempDir: t.TempDir()})
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, uploadRequest(t))
	if rr.Code != 409 || !storage.deleted {
		t.Fatalf("code=%d body=%s", rr.Code, rr.Body.String())
	}
	if bytes.Contains(rr.Body.Bytes(), []byte("fictional.pdf")) {
		t.Fatal("client filename leaked")
	}
}

func TestUploadAllowsIdentityAndRejectsAdditionalParts(t *testing.T) {
	intake := &fakeIntake{}
	storage := &memoryStorage{}
	h := newHandlerWithDependencies(apiDependencies{db: fakePinger{}, intake: intake, storage: storage, tempDir: t.TempDir()})
	r := uploadRequest(t)
	r.Header.Set("Content-Encoding", "identity")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusCreated {
		t.Fatalf("identity code=%d", rr.Code)
	}
	var b bytes.Buffer
	w := multipart.NewWriter(&b)
	_, _ = w.CreateFormField("unexpected")
	p, _ := w.CreateFormFile("file", "fictional.pdf")
	_, _ = p.Write([]byte("%PDF-1.7\nfictional\n%%EOF\n"))
	_ = w.Close()
	r = httptest.NewRequest("POST", "/api/v1/documents", &b)
	r.Header.Set("Content-Type", w.FormDataContentType())
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusBadRequest || !bytes.Contains(rr.Body.Bytes(), []byte(`"request_id":"`)) {
		t.Fatalf("bad multipart: %d %s", rr.Code, rr.Body.String())
	}
}

func TestUploadRejectsMalformedPDFBeforeStorageAndSurfacesStorageFailure(t *testing.T) {
	intake := &fakeIntake{}
	storage := &memoryStorage{}
	h := newHandlerWithDependencies(apiDependencies{db: fakePinger{}, intake: intake, storage: storage, tempDir: t.TempDir()})
	r := uploadRequest(t)
	r.Body = io.NopCloser(bytes.NewReader([]byte("not a multipart request")))
	// Keep the multipart header but make the body invalid; storage must not run.
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, r)
	if rr.Code != http.StatusBadRequest || storage.put {
		t.Fatalf("invalid request code=%d storage.put=%v", rr.Code, storage.put)
	}

	storage = &memoryStorage{err: errors.New("storage unavailable")}
	h = newHandlerWithDependencies(apiDependencies{db: fakePinger{}, intake: intake, storage: storage, tempDir: t.TempDir()})
	rr = httptest.NewRecorder()
	h.ServeHTTP(rr, uploadRequest(t))
	if rr.Code != http.StatusInternalServerError || !storage.put || !bytes.Contains(rr.Body.Bytes(), []byte(`"storage_error"`)) {
		t.Fatalf("storage failure code=%d body=%s", rr.Code, rr.Body.String())
	}
}
