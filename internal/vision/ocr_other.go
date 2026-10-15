//go:build !windows && !linux

package vision

import (
	"errors"
	"image"
)

type ocrResult struct {
	Text  string
	Lines []TextLine
}

func ocrText(img image.Image) (string, error) {
	_ = img
	return "", errors.New("OCR is unavailable on this platform")
}

func ocrTextLines(img image.Image) (ocrResult, error) {
	_ = img
	return ocrResult{}, errors.New("OCR is unavailable on this platform")
}

func ocrInfo() map[string]any {
	return map[string]any{"available": false, "engine": "unavailable"}
}
