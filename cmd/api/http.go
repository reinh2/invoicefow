package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/reinhlord/invoiceflow/internal/documents"
	"github.com/reinhlord/invoiceflow/internal/platform"
	"github.com/reinhlord/invoiceflow/internal/processing"
)

const maxRequestBytes int64 = 21 << 20

type pinger interface{ Ping(context.Context) error }
type intakeService interface {
	CreateQueuedDocument(context.Context, processing.IntakeRecord) error
}
type reviewService interface {
	GetReviewDocument(context.Context, string) (processing.ReviewDocument, error)
	LoadReviewSource(context.Context, string) (processing.SourceDocument, error)
	SaveHumanReview(context.Context, string, int, processing.HumanReviewInput, string) (int, error)
	RejectDocument(context.Context, string, string) error
}
type apiDependencies struct {
	db             pinger
	intake         intakeService
	review         reviewService
	storage        platform.ObjectStorage
	actor, tempDir string
}

func newHandler(db pinger) http.Handler { return newHandlerWithDependencies(apiDependencies{db: db}) }
func newHandlerWithDependencies(deps apiDependencies) http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /readyz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if err := deps.db.Ping(ctx); err != nil {
			writeJSON(w, http.StatusServiceUnavailable, map[string]string{"status": "not_ready"})
			return
		}
		writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("POST /api/v1/documents", func(w http.ResponseWriter, r *http.Request) { uploadDocument(w, r, deps) })
	mux.HandleFunc("GET /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) { getDocumentReview(w, r, deps) })
	mux.HandleFunc("GET /api/v1/documents/{id}/source", func(w http.ResponseWriter, r *http.Request) { getDocumentSource(w, r, deps) })
	mux.HandleFunc("POST /api/v1/documents/{id}/human-reviews", func(w http.ResponseWriter, r *http.Request) { saveHumanReview(w, r, deps) })
	mux.HandleFunc("POST /api/v1/documents/{id}/reject", func(w http.ResponseWriter, r *http.Request) { rejectDocument(w, r, deps) })
	return mux
}

var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func documentID(w http.ResponseWriter, r *http.Request) (string, bool) {
	id := r.PathValue("id")
	if !uuidPattern.MatchString(id) {
		writeAPIError(w, http.StatusNotFound, "document_not_found", "document could not be found")
		return "", false
	}
	return id, true
}

func getDocumentReview(w http.ResponseWriter, r *http.Request, deps apiDependencies) {
	if deps.review == nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	id, ok := documentID(w, r)
	if !ok {
		return
	}
	document, err := deps.review.GetReviewDocument(r.Context(), id)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeJSON(w, http.StatusOK, map[string]any{"document": document})
}

func getDocumentSource(w http.ResponseWriter, r *http.Request, deps apiDependencies) {
	if deps.review == nil || deps.storage == nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	id, ok := documentID(w, r)
	if !ok {
		return
	}
	source, err := deps.review.LoadReviewSource(r.Context(), id)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	stream, err := deps.storage.Open(r.Context(), source.ObjectKey)
	if err != nil {
		writeAPIError(w, 500, "source_unavailable", "original document could not be loaded")
		return
	}
	defer stream.Close()
	w.Header().Set("Content-Type", source.MediaType)
	w.Header().Set("Content-Disposition", "inline")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = io.Copy(w, stream)
}

type saveHumanReviewRequest struct {
	BaseVersion int             `json:"base_version"`
	Proposal    json.RawMessage `json:"proposal"`
}

func saveHumanReview(w http.ResponseWriter, r *http.Request, deps apiDependencies) {
	if deps.review == nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	id, ok := documentID(w, r)
	if !ok {
		return
	}
	var request saveHumanReviewRequest
	if err := decodeStrictJSON(w, r, &request); err != nil || request.BaseVersion < 1 {
		writeAPIError(w, http.StatusBadRequest, "invalid_review", "review changes could not be accepted")
		return
	}
	proposal, err := processing.DecodeHumanReviewInput(request.Proposal)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, "invalid_review", "review changes could not be accepted")
		return
	}
	version, err := deps.review.SaveHumanReview(r.Context(), id, request.BaseVersion, proposal, deps.actor)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeJSON(w, http.StatusCreated, map[string]any{"version_number": version})
}

func rejectDocument(w http.ResponseWriter, r *http.Request, deps apiDependencies) {
	if deps.review == nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	id, ok := documentID(w, r)
	if !ok {
		return
	}
	var request struct {
		Confirm bool `json:"confirm"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil || !request.Confirm {
		writeAPIError(w, http.StatusBadRequest, "invalid_rejection", "rejection must be confirmed")
		return
	}
	if err := deps.review.RejectDocument(r.Context(), id, deps.actor); err != nil {
		writeReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusNoContent)
}

func decodeStrictJSON(w http.ResponseWriter, r *http.Request, target any) error {
	r.Body = http.MaxBytesReader(w, r.Body, 65<<10)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		return errors.New("trailing JSON")
	}
	return nil
}

func writeReviewError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, processing.ErrDocumentNotFound):
		writeAPIError(w, http.StatusNotFound, "document_not_found", "document could not be found")
	case errors.Is(err, processing.ErrInvalidDocumentState):
		writeAPIError(w, http.StatusConflict, "invalid_document_transition", "document is not available for that action")
	case errors.Is(err, processing.ErrStaleReviewVersion):
		writeAPIError(w, http.StatusConflict, "stale_review_version", "review changed; reload the document before saving")
	case errors.Is(err, processing.ErrInvalidHumanReviewEdit):
		writeAPIError(w, http.StatusBadRequest, "invalid_review", "review changes could not be accepted")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "request could not be completed")
	}
}
func uploadDocument(w http.ResponseWriter, r *http.Request, deps apiDependencies) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if deps.intake == nil || deps.storage == nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	if encoding := r.Header.Get("Content-Encoding"); encoding != "" && encoding != "identity" {
		writeAPIError(w, 400, "invalid_request", "request encoding is not supported")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	mr, err := r.MultipartReader()
	if err != nil {
		var maxErr *http.MaxBytesError
		if errors.As(err, &maxErr) {
			writeAPIError(w, 413, "file_too_large", "file could not be accepted")
			return
		}
		writeAPIError(w, 400, "invalid_request", "expected multipart form data")
		return
	}
	var part *multipart.Part
	for {
		p, e := mr.NextPart()
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			var maxErr *http.MaxBytesError
			if errors.As(e, &maxErr) {
				writeAPIError(w, 413, "file_too_large", "file could not be accepted")
				return
			}
			writeAPIError(w, 400, "invalid_request", "invalid multipart body")
			return
		}
		if p.FormName() == "file" {
			part = p
			break // NextPart would discard the current stream; trailing fields are ignored.
		} else {
			_ = p.Close()
			writeAPIError(w, 400, "invalid_request", "exactly one file is required")
			return
		}
	}
	if part == nil {
		writeAPIError(w, 400, "invalid_request", "exactly one file is required")
		return
	}
	defer part.Close()
	prepared, err := documents.PrepareUpload(part.FileName(), part.Header.Get("Content-Type"), part, deps.tempDir)
	if err != nil {
		status, code := 400, "invalid_file"
		if errors.Is(err, documents.ErrTooLarge) {
			status, code = 413, "file_too_large"
		}
		writeAPIError(w, status, code, "file could not be accepted")
		return
	}
	defer prepared.Remove()
	for {
		p, nextErr := mr.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			var maxErr *http.MaxBytesError
			if errors.As(nextErr, &maxErr) {
				writeAPIError(w, 413, "file_too_large", "file could not be accepted")
				return
			}
			writeAPIError(w, 400, "invalid_request", "invalid multipart body")
			return
		}
		_ = p.Close()
		writeAPIError(w, 400, "invalid_request", "exactly one file is required")
		return
	}
	id, err := processing.NewID()
	if err != nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	objectID, err := processing.NewID()
	if err != nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	key := "objects/" + strings.ReplaceAll(objectID, "-", "") + prepared.Suffix
	f, err := os.Open(prepared.Path)
	if err != nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	err = deps.storage.Put(r.Context(), key, f, prepared.Size)
	_ = f.Close()
	if err != nil {
		writeAPIError(w, 500, "storage_error", "file could not be accepted")
		return
	}
	now := time.Now().UTC()
	err = deps.intake.CreateQueuedDocument(r.Context(), processing.IntakeRecord{DocumentID: id, ObjectID: objectID, ObjectKey: key, MediaType: prepared.MediaType, Actor: deps.actor, SHA256: prepared.SHA256, Size: prepared.Size, CreatedAt: now})
	if err != nil {
		_ = deps.storage.Delete(context.Background(), key)
		if errors.Is(err, processing.ErrDuplicate) {
			writeAPIError(w, 409, "duplicate_document", "a matching document already exists")
			return
		}
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	writeJSON(w, 201, map[string]any{"document": documents.UploadResult{ID: id, Status: "queued", CreatedAt: now}})
}

type apiError struct {
	Error struct {
		Code      string `json:"code"`
		Message   string `json:"message"`
		RequestID string `json:"request_id"`
	} `json:"error"`
}

func writeAPIError(w http.ResponseWriter, status int, code, message string) {
	var body apiError
	body.Error.Code = code
	body.Error.Message = message
	body.Error.RequestID, _ = processing.NewID()
	if body.Error.RequestID == "" {
		body.Error.RequestID = "request-failed"
	}
	writeJSON(w, status, body)
}
func writeJSON(w http.ResponseWriter, status int, body any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(body)
}
