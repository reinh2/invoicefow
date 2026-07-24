package extraction

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
)

var (
	ErrMalformedPDF   = errors.New("malformed pdf")
	ErrEncryptedPDF   = errors.New("encrypted pdf")
	ErrUnsupportedOCR = errors.New("unsupported OCR input")
)

// PDFTextExtractor uses Poppler tools from a server-owned container image. It
// never passes a browser name or storage path to a child process.
type PDFTextExtractor struct {
	PDFInfoPath   string
	PDFToTextPath string
	TemporaryDir  string
}

func (e PDFTextExtractor) ExtractText(ctx context.Context, document DocumentInput, limits Limits) (TextExtractionResult, error) {
	if err := limits.ValidateDocumentInput(document); err != nil {
		return TextExtractionResult{}, err
	}
	if document.MediaType != "application/pdf" {
		return TextExtractionResult{}, nil
	}
	ctx, cancel := context.WithTimeout(ctx, limits.PDFTimeout)
	defer cancel()
	path, cleanup, err := copyToPrivateTemp(document, e.TemporaryDir, ".pdf", limits.MaxDocumentBytes)
	if err != nil {
		return TextExtractionResult{}, err
	}
	defer cleanup()
	info, err := runBounded(ctx, e.pdfInfoPath(), []string{"-isodates", path}, limits.MaxProcessOutputBytes)
	if err != nil {
		return TextExtractionResult{}, fmt.Errorf("%w", ErrMalformedPDF)
	}
	pages, encrypted, ok := parsePDFInfo(string(info))
	if !ok {
		return TextExtractionResult{}, ErrMalformedPDF
	}
	if encrypted {
		return TextExtractionResult{}, ErrEncryptedPDF
	}
	if pages > limits.MaxPages {
		return TextExtractionResult{}, ErrTooManyPages
	}
	// -layout preserves the physical row structure of the page. Without it
	// Poppler emits column-major text, which tears an invoice table apart: every
	// label lands on a different line from its amount, so a reader downstream
	// cannot tell which number belongs to "Subtotal". The flag is still a fixed
	// literal argument under the same output and timeout bounds.
	out, err := runBounded(ctx, e.pdfToTextPath(), []string{"-enc", "UTF-8", "-layout", "-f", "1", "-l", strconv.Itoa(pages), path, "-"}, limits.MaxProcessOutputBytes)
	if err != nil {
		if errors.Is(err, ErrProcessOutputTooLarge) || errors.Is(err, context.DeadlineExceeded) {
			return TextExtractionResult{}, err
		}
		return TextExtractionResult{}, ErrMalformedPDF
	}
	result := TextExtractionResult{Pages: splitPageText(string(out), pages)}
	if err := limits.ValidatePageText(result.Pages); err != nil {
		return TextExtractionResult{}, err
	}
	return result, nil
}

func (e PDFTextExtractor) pdfInfoPath() string {
	if e.PDFInfoPath != "" {
		return e.PDFInfoPath
	}
	return "/usr/bin/pdfinfo"
}
func (e PDFTextExtractor) pdfToTextPath() string {
	if e.PDFToTextPath != "" {
		return e.PDFToTextPath
	}
	return "/usr/bin/pdftotext"
}

func parsePDFInfo(info string) (pages int, encrypted, ok bool) {
	for _, line := range strings.Split(info, "\n") {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		switch strings.TrimSpace(key) {
		case "Pages":
			parsed, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || parsed < 1 {
				return 0, false, false
			}
			pages = parsed
		case "Encrypted":
			encrypted = strings.HasPrefix(strings.ToLower(strings.TrimSpace(value)), "yes")
		}
	}
	return pages, encrypted, pages > 0
}

func splitPageText(text string, pageCount int) []PageText {
	parts := strings.Split(text, "\f")
	if len(parts) > 0 && parts[len(parts)-1] == "" {
		parts = parts[:len(parts)-1]
	}
	if len(parts) > pageCount {
		parts = parts[:pageCount]
	}
	pages := make([]PageText, 0, len(parts))
	for index, part := range parts {
		pages = append(pages, PageText{PageNumber: index + 1, Text: part})
	}
	return pages
}

func copyToPrivateTemp(document DocumentInput, directory, suffix string, maxBytes int64) (string, func(), error) {
	if directory == "" {
		return "", nil, fmt.Errorf("private temporary directory is required")
	}
	file, err := os.CreateTemp(directory, "invoiceflow-extract-*"+suffix)
	if err != nil {
		return "", nil, err
	}
	path := file.Name()
	cleanup := func() { _ = os.Remove(path) }
	written, copyErr := io.Copy(file, io.LimitReader(document.Reader, maxBytes+1))
	closeErr := file.Close()
	if copyErr != nil {
		cleanup()
		return "", nil, copyErr
	}
	if closeErr != nil {
		cleanup()
		return "", nil, closeErr
	}
	if written != document.SizeBytes || written > maxBytes {
		cleanup()
		return "", nil, ErrInputTooLarge
	}
	return path, cleanup, nil
}

type boundedBuffer struct {
	bytes.Buffer
	limit    int
	exceeded bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if b.Len()+len(p) > b.limit {
		b.exceeded = true
		return 0, ErrProcessOutputTooLarge
	}
	return b.Buffer.Write(p)
}

func runBounded(ctx context.Context, executable string, args []string, limit int) ([]byte, error) {
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if !filepath.IsAbs(executable) {
		return nil, fmt.Errorf("executable path must be absolute")
	}
	stdout, stderr := &boundedBuffer{limit: limit}, &boundedBuffer{limit: limit}
	command := exec.CommandContext(ctx, executable, args...)
	command.Stdout, command.Stderr = stdout, stderr
	err := command.Run()
	if stdout.exceeded || stderr.exceeded {
		return nil, ErrProcessOutputTooLarge
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if err != nil {
		return nil, fmt.Errorf("process failed")
	}
	return stdout.Bytes(), nil
}
