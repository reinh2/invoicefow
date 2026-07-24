package webui

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeBundle(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatalf("create assets directory: %v", err)
	}
	files := map[string]string{
		"index.html":              "<!doctype html><title>InvoiceFlow</title>",
		"assets/index-abc123.js":  "export const app = 1;",
		"assets/index-abc123.css": ".hero { color: #fff; }",
	}
	for name, body := range files {
		if err := os.WriteFile(filepath.Join(root, filepath.FromSlash(name)), []byte(body), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
	return root
}

func get(t *testing.T, handler http.Handler, method, target string) *httptest.ResponseRecorder {
	t.Helper()
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(method, target, nil))
	return recorder
}

func TestLoadReportsNoBundleForUnconfiguredOrEmptyDirectory(t *testing.T) {
	for name, dir := range map[string]string{
		"unconfigured":  "",
		"missing":       filepath.Join(t.TempDir(), "absent"),
		"without index": t.TempDir(),
	} {
		if _, err := Load(dir); !errors.Is(err, ErrNoBundle) {
			t.Fatalf("%s: expected ErrNoBundle, got %v", name, err)
		}
	}
}

func TestHandlerServesAssetsAndFallsBackToTheShell(t *testing.T) {
	bundle, err := Load(writeBundle(t))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	handler := bundle.Handler()

	asset := get(t, handler, http.MethodGet, "/assets/index-abc123.js")
	if asset.Code != http.StatusOK {
		t.Fatalf("asset status = %d, want 200", asset.Code)
	}
	if got := asset.Header().Get("Content-Type"); got != "text/javascript; charset=utf-8" {
		t.Fatalf("asset content type = %q", got)
	}
	if got := asset.Header().Get("Cache-Control"); got != "public, max-age=31536000, immutable" {
		t.Fatalf("hashed asset cache control = %q", got)
	}

	// A client-routed application path is not a file, so it must load the shell.
	shell := get(t, handler, http.MethodGet, "/app/documents/0d0c2342-2486-4f10-a858-e75bc763f3e4")
	if shell.Code != http.StatusOK || !strings.Contains(shell.Body.String(), "InvoiceFlow") {
		t.Fatalf("shell fallback status = %d body = %q", shell.Code, shell.Body.String())
	}
	if got := shell.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("shell cache control = %q, want no-store", got)
	}
}

func TestHandlerDoesNotAnswerMissingAssetsWithHTML(t *testing.T) {
	bundle, err := Load(writeBundle(t))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	missing := get(t, bundle.Handler(), http.MethodGet, "/assets/index-stale.js")
	if missing.Code != http.StatusNotFound {
		t.Fatalf("missing asset status = %d, want 404", missing.Code)
	}
	if strings.Contains(missing.Body.String(), "<!doctype html>") {
		t.Fatal("missing asset was answered with the application shell")
	}
}

func TestHandlerResolvesTraversalAttemptsToKnownAssetsOnly(t *testing.T) {
	root := writeBundle(t)
	secret := filepath.Join(filepath.Dir(root), "outside.txt")
	if err := os.WriteFile(secret, []byte("must never be served"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	bundle, err := Load(root)
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	handler := bundle.Handler()

	for _, target := range []string{"/../outside.txt", "/assets/../../outside.txt", "/assets/%2e%2e/%2e%2e/outside.txt"} {
		recorder := get(t, handler, http.MethodGet, target)
		if strings.Contains(recorder.Body.String(), "must never be served") {
			t.Fatalf("%s escaped the bundle", target)
		}
	}
}

func TestLoadSkipsFilesOutsideTheExtensionAllowlist(t *testing.T) {
	root := writeBundle(t)
	if err := os.WriteFile(filepath.Join(root, "deploy.env"), []byte("SECRET=1"), 0o644); err != nil {
		t.Fatalf("write env file: %v", err)
	}
	bundle, err := Load(root)
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	if _, loaded := bundle.assets["/deploy.env"]; loaded {
		t.Fatal("a file outside the allowlist was loaded into the bundle")
	}
	recorder := get(t, bundle.Handler(), http.MethodGet, "/deploy.env")
	if strings.Contains(recorder.Body.String(), "SECRET=1") {
		t.Fatal("a file outside the allowlist was served")
	}
}

func TestHandlerSetsHardenedResponseHeadersAndRejectsWrites(t *testing.T) {
	bundle, err := Load(writeBundle(t))
	if err != nil {
		t.Fatalf("load bundle: %v", err)
	}
	handler := bundle.Handler()

	shell := get(t, handler, http.MethodGet, "/")
	if got := shell.Header().Get("X-Content-Type-Options"); got != "nosniff" {
		t.Fatalf("nosniff header = %q", got)
	}
	policy := shell.Header().Get("Content-Security-Policy")
	for _, required := range []string{"default-src 'self'", "frame-ancestors 'none'", "base-uri 'none'", "object-src 'none'"} {
		if !strings.Contains(policy, required) {
			t.Fatalf("policy %q is missing %q", policy, required)
		}
	}
	for _, forbidden := range []string{"unsafe-inline", "unsafe-eval"} {
		if strings.Contains(policy, forbidden) {
			t.Fatalf("policy %q permits %q", policy, forbidden)
		}
	}

	head := get(t, handler, http.MethodHead, "/")
	if head.Code != http.StatusOK || head.Body.Len() != 0 {
		t.Fatalf("HEAD status = %d body length = %d", head.Code, head.Body.Len())
	}

	post := get(t, handler, http.MethodPost, "/")
	if post.Code != http.StatusMethodNotAllowed {
		t.Fatalf("POST status = %d, want 405", post.Code)
	}
	// The 405 must carry the same hardened headers as every other static
	// response, not just http.Error's default text/plain + nosniff.
	if got := post.Header().Get("Content-Security-Policy"); got != contentSecurityPolicy {
		t.Fatalf("405 Content-Security-Policy = %q", got)
	}
	if got := post.Header().Get("Referrer-Policy"); got != "same-origin" {
		t.Fatalf("405 Referrer-Policy = %q", got)
	}
	if got := post.Header().Get("Allow"); got != "GET, HEAD" {
		t.Fatalf("405 Allow = %q", got)
	}
}
