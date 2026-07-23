package extraction

import (
	"context"
	"fmt"
	"image"
	"os"
	"strings"

	_ "image/jpeg"
	_ "image/png"
)

// TesseractOCR is a bounded local OCR adapter. JPEG and PNG are supported in
// Stage 3; PDF raster OCR is intentionally a separate adapter concern so its
// Poppler raster pixel accounting cannot be accidentally bypassed.
type TesseractOCR struct {
	TesseractPath string
	TemporaryDir  string
}

func (o TesseractOCR) ExtractOCR(ctx context.Context, document DocumentInput, limits Limits) (OCRResult, error) {
	if err := limits.ValidateDocumentInput(document); err != nil {
		return OCRResult{}, err
	}
	if document.MediaType != "image/jpeg" && document.MediaType != "image/png" {
		return OCRResult{}, ErrUnsupportedOCR
	}
	ctx, cancel := context.WithTimeout(ctx, limits.OCRTimeout)
	defer cancel()
	suffix := ".jpg"
	if document.MediaType == "image/png" {
		suffix = ".png"
	}
	path, cleanup, err := copyToPrivateTemp(document, o.TemporaryDir, suffix, limits.MaxDocumentBytes)
	if err != nil {
		return OCRResult{}, err
	}
	defer cleanup()
	input, err := os.Open(path)
	if err != nil {
		return OCRResult{}, err
	}
	config, _, configErr := image.DecodeConfig(input)
	_ = input.Close()
	if configErr != nil || config.Width < 1 || config.Height < 1 || config.Width > limits.MaxRasterDimension || config.Height > limits.MaxRasterDimension || int64(config.Width)*int64(config.Height) > limits.MaxRasterPixels {
		return OCRResult{}, ErrInvalidInput
	}
	executable := o.TesseractPath
	if executable == "" {
		executable = "/usr/bin/tesseract"
	}
	out, err := runBounded(ctx, executable, []string{path, "stdout", "--psm", "6"}, limits.MaxProcessOutputBytes)
	if err != nil {
		return OCRResult{}, fmt.Errorf("ocr failed: %w", err)
	}
	text := string(out)
	if strings.TrimSpace(text) == "" {
		return OCRResult{}, nil
	}
	result := OCRResult{Pages: []PageText{{PageNumber: 1, Text: text}}}
	if err := limits.ValidatePageText(result.Pages); err != nil {
		return OCRResult{}, err
	}
	return result, nil
}
