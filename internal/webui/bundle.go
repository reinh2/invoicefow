// Package webui serves one pre-built browser bundle (ADR-013).
//
// The whole bundle is read into memory once at startup and every request is
// answered from an exact map lookup. No request string ever reaches a
// filesystem call, so path traversal, symlink escape, and directory listing are
// structurally impossible rather than filtered out afterwards.
package webui

import (
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const (
	// MaxFiles and MaxTotalBytes bound what a misconfigured or unexpectedly
	// large WEB_DIR can pull into process memory.
	MaxFiles      = 512
	MaxTotalBytes = 32 << 20

	indexPath = "index.html"
)

// ErrNoBundle reports that the configured directory holds no usable bundle.
var ErrNoBundle = errors.New("web bundle not found")

// contentTypes is an allowlist. A file extension outside it is not loaded, so
// the served content type is never derived from file contents or client input.
var contentTypes = map[string]string{
	".html":  "text/html; charset=utf-8",
	".css":   "text/css; charset=utf-8",
	".js":    "text/javascript; charset=utf-8",
	".json":  "application/json",
	".map":   "application/json",
	".svg":   "image/svg+xml",
	".png":   "image/png",
	".jpg":   "image/jpeg",
	".jpeg":  "image/jpeg",
	".webp":  "image/webp",
	".avif":  "image/avif",
	".ico":   "image/vnd.microsoft.icon",
	".webm":  "video/webm",
	".mp4":   "video/mp4",
	".woff2": "font/woff2",
	".txt":   "text/plain; charset=utf-8",
}

// contentSecurityPolicy is deliberately fixed rather than configurable. The
// bundle is first-party only: it loads nothing from another origin, inlines no
// script, and evaluates no string as code.
const contentSecurityPolicy = "default-src 'self'; script-src 'self'; style-src 'self'; img-src 'self' data:; media-src 'self'; font-src 'self'; connect-src 'self'; object-src 'self'; base-uri 'none'; frame-ancestors 'none'; form-action 'self'"

type asset struct {
	body        []byte
	contentType string
	immutable   bool
}

// Bundle is an immutable in-memory copy of one built frontend.
type Bundle struct {
	assets map[string]asset
	index  asset
}

// Load reads dir into memory. An empty dir reports ErrNoBundle so a caller can
// keep running as an API-only process, which is the Stage 5 behavior.
func Load(dir string) (*Bundle, error) {
	if strings.TrimSpace(dir) == "" {
		return nil, ErrNoBundle
	}
	root, err := filepath.Abs(dir)
	if err != nil {
		return nil, fmt.Errorf("resolve web directory: %w", err)
	}
	info, err := os.Stat(root)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNoBundle
		}
		return nil, fmt.Errorf("read web directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("web directory %w", fs.ErrInvalid)
	}

	assets := make(map[string]asset)
	var total int64
	walkErr := filepath.WalkDir(root, func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		// Only regular files are loaded: a symlink inside the bundle would
		// otherwise let a build step point at bytes outside the directory.
		if !entry.Type().IsRegular() {
			return nil
		}
		contentType, allowed := contentTypes[strings.ToLower(filepath.Ext(name))]
		if !allowed {
			return nil
		}
		relative, err := filepath.Rel(root, name)
		if err != nil {
			return err
		}
		stat, err := entry.Info()
		if err != nil {
			return err
		}
		if len(assets) >= MaxFiles {
			return fmt.Errorf("web bundle exceeds %d files", MaxFiles)
		}
		if total+stat.Size() > MaxTotalBytes {
			return fmt.Errorf("web bundle exceeds %d bytes", MaxTotalBytes)
		}
		body, err := os.ReadFile(name)
		if err != nil {
			return err
		}
		total += int64(len(body))
		key := path.Clean("/" + filepath.ToSlash(relative))
		assets[key] = asset{
			body:        body,
			contentType: contentType,
			// Vite emits content-hashed names under assets/; only those may be
			// cached indefinitely.
			immutable: strings.HasPrefix(key, "/assets/"),
		}
		return nil
	})
	if walkErr != nil {
		return nil, fmt.Errorf("load web bundle: %w", walkErr)
	}

	index, ok := assets["/"+indexPath]
	if !ok {
		return nil, ErrNoBundle
	}
	return &Bundle{assets: assets, index: index}, nil
}

// Handler answers browser requests for the bundle. The caller is responsible
// for registering it so that it cannot shadow API or health routes.
func (b *Bundle) Handler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			w.Header().Set("Allow", "GET, HEAD")
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		requested := path.Clean("/" + strings.TrimPrefix(r.URL.Path, "/"))
		if found, ok := b.assets[requested]; ok {
			b.write(w, r, found)
			return
		}
		// A missing asset is a genuine 404. Serving the application shell for a
		// stale script URL would answer JavaScript requests with HTML.
		if ext := path.Ext(requested); ext != "" && ext != ".html" {
			b.writeHeaders(w, asset{contentType: "text/plain; charset=utf-8"})
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
		// Client-routed application paths fall back to the shell.
		b.write(w, r, b.index)
	})
}

func (b *Bundle) write(w http.ResponseWriter, r *http.Request, a asset) {
	b.writeHeaders(w, a)
	w.Header().Set("Content-Type", a.contentType)
	w.WriteHeader(http.StatusOK)
	if r.Method == http.MethodHead {
		return
	}
	_, _ = w.Write(a.body)
}

func (b *Bundle) writeHeaders(w http.ResponseWriter, a asset) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
	w.Header().Set("Referrer-Policy", "same-origin")
	if a.immutable {
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		return
	}
	// The shell must never be cached: a redeployed bundle would otherwise load
	// hashed asset names that no longer exist.
	w.Header().Set("Cache-Control", "no-store")
}
