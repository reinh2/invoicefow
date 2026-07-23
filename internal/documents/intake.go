package documents

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"image"
	"io"
	"mime"
	"os"
	"path/filepath"
	"strings"

	_ "image/jpeg"
	_ "image/png"
)

const (
	MaxFileBytes int64 = 20 << 20
	maxPixels          = 40_000_000
)

var ErrInvalidUpload = errors.New("invalid upload")
var ErrTooLarge = errors.New("upload too large")

type PreparedUpload struct {
	Path      string
	SHA256    [32]byte
	Size      int64
	MediaType string
	Suffix    string
}

// PrepareUpload copies an untrusted stream to a private temporary file while
// calculating its identity. It validates metadata and signatures before bytes
// can become a durable object.
func PrepareUpload(filename, declaredType string, r io.Reader, temporaryDir string) (prepared PreparedUpload, err error) {
	suffix, expected, err := declaredUploadType(filename, declaredType)
	if err != nil {
		return PreparedUpload{}, err
	}
	file, err := os.CreateTemp(temporaryDir, "invoiceflow-upload-*")
	if err != nil {
		return PreparedUpload{}, fmt.Errorf("create intake temporary file: %w", err)
	}
	temporaryPath := file.Name()
	prepared.Path = temporaryPath
	defer func() {
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()

	header := make([]byte, 16)
	n, readErr := io.ReadFull(r, header)
	if readErr != nil && !errors.Is(readErr, io.ErrUnexpectedEOF) && !errors.Is(readErr, io.EOF) {
		_ = file.Close()
		return PreparedUpload{}, fmt.Errorf("read upload: %w", readErr)
	}
	header = header[:n]
	actual := detectSignature(header)
	if actual == "" || actual != expected {
		_ = file.Close()
		return PreparedUpload{}, ErrInvalidUpload
	}
	hash := sha256.New()
	written, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(io.MultiReader(bytes.NewReader(header), r), MaxFileBytes+1))
	if copyErr != nil {
		_ = file.Close()
		return PreparedUpload{}, fmt.Errorf("copy upload: %w", copyErr)
	}
	if written > MaxFileBytes {
		_ = file.Close()
		return PreparedUpload{}, ErrTooLarge
	}
	if written == 0 {
		_ = file.Close()
		return PreparedUpload{}, ErrInvalidUpload
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return PreparedUpload{}, fmt.Errorf("sync upload: %w", err)
	}
	if err := file.Close(); err != nil {
		return PreparedUpload{}, fmt.Errorf("close upload: %w", err)
	}
	if actual == "application/pdf" {
		if !hasPDFEndMarker(prepared.Path) {
			return PreparedUpload{}, ErrInvalidUpload
		}
	}
	if actual == "image/jpeg" || actual == "image/png" {
		if validateImage(prepared.Path, actual) != nil {
			return PreparedUpload{}, ErrInvalidUpload
		}
	}
	copy(prepared.SHA256[:], hash.Sum(nil))
	prepared.Size = written
	prepared.MediaType = actual
	prepared.Suffix = suffix
	return prepared, nil
}

func hasPDFEndMarker(path string) bool {
	f, err := os.Open(path)
	if err != nil {
		return false
	}
	defer f.Close()
	info, err := f.Stat()
	if err != nil || info.Size() < 8 {
		return false
	}
	start := max(info.Size()-1024, 0)
	if _, err := f.Seek(start, io.SeekStart); err != nil {
		return false
	}
	tail, err := io.ReadAll(f)
	return err == nil && bytes.Contains(tail, []byte("%%EOF"))
}

func validateImage(path, expectedType string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	config, format, err := image.DecodeConfig(f)
	if err != nil || format != strings.TrimPrefix(expectedType, "image/") || config.Width < 1 || config.Height < 1 || config.Width > 10_000 || config.Height > 10_000 || int64(config.Width)*int64(config.Height) > maxPixels {
		return ErrInvalidUpload
	}
	if _, err := f.Seek(0, io.SeekStart); err != nil {
		return err
	}
	_, format, err = image.Decode(f)
	if err != nil || format != strings.TrimPrefix(expectedType, "image/") {
		return ErrInvalidUpload
	}
	return nil
}

func (p PreparedUpload) Remove() {
	if p.Path != "" {
		_ = os.Remove(p.Path)
	}
}

func declaredUploadType(filename, declared string) (string, string, error) {
	suffix := strings.ToLower(filepath.Ext(filename))
	want := map[string]string{".pdf": "application/pdf", ".jpg": "image/jpeg", ".jpeg": "image/jpeg", ".png": "image/png"}[suffix]
	if want == "" {
		return "", "", ErrInvalidUpload
	}
	mt, _, err := mime.ParseMediaType(declared)
	if err != nil || strings.ToLower(mt) != want {
		return "", "", ErrInvalidUpload
	}
	return suffix, want, nil
}

func detectSignature(b []byte) string {
	if len(b) >= 5 && string(b[:5]) == "%PDF-" {
		return "application/pdf"
	}
	if len(b) >= 3 && b[0] == 0xff && b[1] == 0xd8 && b[2] == 0xff {
		return "image/jpeg"
	}
	if len(b) >= 8 && string(b[:8]) == "\x89PNG\r\n\x1a\n" {
		return "image/png"
	}
	return ""
}
