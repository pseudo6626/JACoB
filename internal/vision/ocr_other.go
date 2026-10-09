//go:build !windows

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
	return "", errors.New("OCR is only available in the Windows Vision build")
}

func ocrTextLines(img image.Image) (ocrResult, error) {
	_ = img
	return ocrResult{}, errors.New("OCR is only available in the Windows Vision build")
}
