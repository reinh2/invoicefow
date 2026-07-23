package documents

import (
	"bytes"
	"errors"
	"os"
	"testing"
)

func TestPrepareUploadValidatesTripleAndHash(t *testing.T) {
	dir := t.TempDir()
	data := []byte("%PDF-1.7\nfictional\n%%EOF\n")
	got, err := PrepareUpload("invoice.pdf", "application/pdf", bytes.NewReader(data), dir)
	if err != nil {
		t.Fatal(err)
	}
	defer got.Remove()
	if got.Size != int64(len(data)) || got.MediaType != "application/pdf" {
		t.Fatalf("%+v", got)
	}
	if _, err := os.Stat(got.Path); err != nil {
		t.Fatal(err)
	}
}
func TestPrepareUploadRejectsMismatchedType(t *testing.T) {
	dir := t.TempDir()
	_, err := PrepareUpload("invoice.pdf", "application/pdf", bytes.NewReader([]byte("not pdf")), dir)
	if !errors.Is(err, ErrInvalidUpload) {
		t.Fatalf("got %v", err)
	}
	entries, readErr := os.ReadDir(dir)
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("failed upload left temporary files: %v", entries)
	}
}

func TestPrepareUploadRejectsTruncatedPDFAndMalformedPNG(t *testing.T) {
	if _, err := PrepareUpload("invoice.pdf", "application/pdf", bytes.NewReader([]byte("%PDF-1.7\n")), t.TempDir()); !errors.Is(err, ErrInvalidUpload) {
		t.Fatalf("truncated PDF error = %v", err)
	}
	// This has the right signature and plausible dimensions, but no IHDR chunk
	// or image payload. Full decoder validation must reject it.
	png := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rBAD!"), []byte{0, 0, 0, 1, 0, 0, 0, 1}...)
	if _, err := PrepareUpload("invoice.png", "image/png", bytes.NewReader(png), t.TempDir()); !errors.Is(err, ErrInvalidUpload) {
		t.Fatalf("malformed PNG error = %v", err)
	}
}

func TestPrepareUploadRejectsOversizedImageDimensions(t *testing.T) {
	// Signature plus IHDR width/height; validation reads only metadata at intake.
	data := append([]byte("\x89PNG\r\n\x1a\n\x00\x00\x00\rIHDR"), []byte{0, 0, 0x27, 0x11, 0, 0, 0, 1}...)
	_, err := PrepareUpload("invoice.png", "image/png", bytes.NewReader(data), t.TempDir())
	if !errors.Is(err, ErrInvalidUpload) {
		t.Fatalf("got %v", err)
	}
}
