package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"mime/multipart"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/reinhlord/invoiceflow/internal/documents"
	"github.com/reinhlord/invoiceflow/internal/export"
	"github.com/reinhlord/invoiceflow/internal/platform"
	"github.com/reinhlord/invoiceflow/internal/processing"
	"github.com/reinhlord/invoiceflow/internal/ratelimit"
	"github.com/reinhlord/invoiceflow/internal/webui"
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
	ApproveDocument(context.Context, string, int, string) (int, error)
	ExportCSV(context.Context, string, string) ([]byte, error)
	ApprovedVersionNumber(context.Context, string) (int, error)
	EnqueueWebhookExport(context.Context, string, string) (processing.ExportRecord, error)
	ListDocuments(context.Context, int, string) (processing.DocumentPage, error)
}
type apiDependencies struct {
	db                pinger
	intake            intakeService
	review            reviewService
	storage           platform.ObjectStorage
	web               *webui.Bundle
	actor, tempDir    string
	webhookConfigured bool
	// publicDemo only changes what the interface tells the visitor; it grants
	// and withholds nothing. uploadLimiter is nil when limiting is disabled.
	publicDemo    bool
	uploadLimiter *ratelimit.Limiter
}

func newHandler(db pinger) http.Handler { return newHandlerWithDependencies(apiDependencies{db: db}) }
func newHandlerWithDependencies(deps apiDependencies) http.Handler {
	mux := http.NewServeMux()
	// Both health routes are method-scoped so the optional GET "/" bundle
	// fallback below is an unambiguous, strictly less specific pattern.
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
	mux.HandleFunc("GET /api/v1/documents", func(w http.ResponseWriter, r *http.Request) { listDocuments(w, r, deps) })
	mux.HandleFunc("GET /api/v1/config", func(w http.ResponseWriter, r *http.Request) { getClientConfig(w, deps) })
	mux.HandleFunc("GET /api/v1/documents/{id}", func(w http.ResponseWriter, r *http.Request) { getDocumentReview(w, r, deps) })
	mux.HandleFunc("GET /api/v1/documents/{id}/source", func(w http.ResponseWriter, r *http.Request) { getDocumentSource(w, r, deps) })
	mux.HandleFunc("POST /api/v1/documents/{id}/human-reviews", func(w http.ResponseWriter, r *http.Request) { saveHumanReview(w, r, deps) })
	mux.HandleFunc("POST /api/v1/documents/{id}/reject", func(w http.ResponseWriter, r *http.Request) { rejectDocument(w, r, deps) })
	mux.HandleFunc("POST /api/v1/documents/{id}/approve", func(w http.ResponseWriter, r *http.Request) { approveDocument(w, r, deps) })
	mux.HandleFunc("GET /api/v1/documents/{id}/export/csv", func(w http.ResponseWriter, r *http.Request) { exportCSV(w, r, deps) })
	mux.HandleFunc("POST /api/v1/documents/{id}/export/webhook", func(w http.ResponseWriter, r *http.Request) { exportWebhook(w, r, deps) })
	if deps.web != nil {
		// Registered last on the bare "/" pattern (all methods) so every API and
		// health pattern above stays strictly more specific and wins by
		// specificity, while unmatched paths — including non-GET methods — reach
		// serveWebBundle. This lets the reserved-prefix guard answer with the JSON
		// envelope regardless of method instead of leaving Go's mux to emit a bare
		// 405 for, say, a POST to a mistyped /api route.
		mux.Handle("/", serveWebBundle(deps.web))
	}
	return mux
}

// reservedPrefixes never fall back to the application shell. An unknown path
// below them is a server-side route, so it must answer with the JSON error
// envelope rather than HTML that a client would try to parse as data.
var reservedPrefixes = []string{"/api/", "/healthz", "/readyz"}

// reservedRoute reports whether path belongs to a server-owned namespace that
// must answer with the JSON error envelope rather than the application shell.
// The comparison is case-insensitive so /API/... cannot slip through to HTML.
func reservedRoute(path string) bool {
	lower := strings.ToLower(path)
	for _, prefix := range reservedPrefixes {
		if lower == strings.TrimSuffix(prefix, "/") || strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

func serveWebBundle(bundle *webui.Bundle) http.Handler {
	assets := bundle.Handler()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if reservedRoute(r.URL.Path) {
			writeAPIError(w, http.StatusNotFound, "route_not_found", "route could not be found")
			return
		}
		// bundle.Handler applies its own GET/HEAD method gate (a non-GET request
		// to a non-reserved path gets a hardened 405) and serves the shell or a
		// 404 for known asset extensions.
		assets.ServeHTTP(w, r)
	})
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

// listDocuments returns one bounded, presentation-safe page of documents. The
// page size is clamped by the repository and the cursor is opaque, so a client
// can neither request an unbounded scan nor construct a cursor of its own.
func listDocuments(w http.ResponseWriter, r *http.Request, deps apiDependencies) {
	if deps.review == nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	pageSize := 0
	if raw := r.URL.Query().Get("limit"); raw != "" {
		parsed, err := strconv.Atoi(raw)
		if err != nil || parsed < 1 {
			writeAPIError(w, http.StatusBadRequest, "invalid_pagination", "limit must be a positive integer")
			return
		}
		pageSize = parsed
	}
	page, err := deps.review.ListDocuments(r.Context(), pageSize, r.URL.Query().Get("cursor"))
	if err != nil {
		if errors.Is(err, processing.ErrInvalidCursor) {
			writeAPIError(w, http.StatusBadRequest, "invalid_pagination", "cursor is not valid")
			return
		}
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeJSON(w, http.StatusOK, page)
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

func approveDocument(w http.ResponseWriter, r *http.Request, deps apiDependencies) {
	if deps.review == nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	id, ok := documentID(w, r)
	if !ok {
		return
	}
	var request struct {
		VersionNumber int  `json:"version_number"`
		Confirm       bool `json:"confirm"`
	}
	if err := decodeStrictJSON(w, r, &request); err != nil || !request.Confirm || request.VersionNumber < 1 {
		writeAPIError(w, http.StatusBadRequest, "invalid_approval", "approval must target an explicit version and be confirmed")
		return
	}
	ver, err := deps.review.ApproveDocument(r.Context(), id, request.VersionNumber, deps.actor)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeJSON(w, http.StatusOK, map[string]any{
		"document": map[string]any{
			"id":                      id,
			"status":                  "approved",
			"approved_version_number": ver,
		},
	})
}

func exportCSV(w http.ResponseWriter, r *http.Request, deps apiDependencies) {
	if deps.review == nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	id, ok := documentID(w, r)
	if !ok {
		return
	}
	csvBytes, err := deps.review.ExportCSV(r.Context(), id, deps.actor)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	version, err := deps.review.ApprovedVersionNumber(r.Context(), id)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("X-InvoiceFlow-CSV-Format", export.FormatVersionV1)
	w.Header().Set("Content-Disposition", "attachment; filename=\"invoice-"+id+"-v"+strconv.Itoa(version)+".csv\"")
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(csvBytes)
}

func exportWebhook(w http.ResponseWriter, r *http.Request, deps apiDependencies) {
	if deps.review == nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	id, ok := documentID(w, r)
	if !ok {
		return
	}
	if !deps.webhookConfigured {
		writeReviewError(w, processing.ErrWebhookNotConfigured)
		return
	}
	if r.ContentLength > 0 && r.Body != nil {
		var dummy struct{}
		if err := decodeStrictJSON(w, r, &dummy); err != nil {
			writeAPIError(w, http.StatusBadRequest, "invalid_export", "malformed request payload")
			return
		}
	}
	rec, err := deps.review.EnqueueWebhookExport(r.Context(), id, deps.actor)
	if err != nil {
		writeReviewError(w, err)
		return
	}
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeJSON(w, http.StatusAccepted, map[string]any{"export": rec})
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
	case errors.Is(err, processing.ErrInvalidApproval):
		writeAPIError(w, http.StatusBadRequest, "invalid_approval", "approval request was invalid")
	case errors.Is(err, processing.ErrInvalidExport):
		writeAPIError(w, http.StatusBadRequest, "invalid_export", "export request was invalid")
	case errors.Is(err, processing.ErrWebhookNotConfigured):
		writeAPIError(w, http.StatusBadRequest, "webhook_not_configured", "destination webhook URL is not configured on server")
	default:
		writeAPIError(w, http.StatusInternalServerError, "internal_error", "request could not be completed")
	}
}

// getClientConfig exposes only presentation flags the browser needs. It must
// never carry a secret, a destination, a path, or anything that grants
// authority — the client is untrusted and this route is unauthenticated.
func getClientConfig(w http.ResponseWriter, deps apiDependencies) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	writeJSON(w, http.StatusOK, map[string]any{"public_demo": deps.publicDemo})
}

// rateLimitClient identifies a caller for rate limiting by transport peer
// address only. A forwarded-for header is deliberately not trusted: any client
// can set one, so honouring it would let a single caller bypass the limit by
// varying a header. A deployment behind a proxy must therefore enforce its own
// limit at that proxy.
func rateLimitClient(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func uploadDocument(w http.ResponseWriter, r *http.Request, deps apiDependencies) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "no-store")
	if deps.intake == nil || deps.storage == nil {
		writeAPIError(w, 500, "internal_error", "request could not be completed")
		return
	}
	// Checked before the body is read: a refused caller must not be able to make
	// the server consume 20 MiB per attempt.
	if allowed, retryAfter := deps.uploadLimiter.Allow(rateLimitClient(r)); !allowed {
		w.Header().Set("Retry-After", strconv.Itoa(int(retryAfter.Seconds())+1))
		writeAPIError(w, http.StatusTooManyRequests, "rate_limited", "too many uploads; try again shortly")
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
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
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
