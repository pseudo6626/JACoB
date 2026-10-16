//go:build linux

package vision

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ocrResult struct {
	Text  string
	Lines []TextLine
}

type tesseractWord struct {
	page, block, par, line   int
	left, top, width, height int
	text                     string
}

func ocrText(img image.Image) (string, error) {
	out, err := ocrTextLines(img)
	if err != nil {
		return "", err
	}
	return out.Text, nil
}

func ocrTextLines(img image.Image) (ocrResult, error) {
	if img == nil {
		return ocrResult{}, fmt.Errorf("OCR image is nil")
	}
	b := img.Bounds()
	if b.Dx() < 2 || b.Dy() < 2 {
		return ocrResult{}, fmt.Errorf("OCR image is empty")
	}

	bin, err := linuxTesseractPath()
	if err != nil {
		return ocrResult{}, err
	}

	attempts := []struct {
		img image.Image
		psm string
	}{
		{img: img, psm: "6"},
		{img: img, psm: "11"},
		{img: linuxOCRLightTextVariant(img), psm: "11"},
	}

	var lastErr error
	hadSuccessfulRun := false
	for _, attempt := range attempts {
		out, runErr := runLinuxTesseract(bin, attempt.img, attempt.psm)
		if runErr != nil {
			lastErr = runErr
			continue
		}
		hadSuccessfulRun = true
		if strings.TrimSpace(out.Text) != "" || len(out.Lines) > 0 {
			return out, nil
		}
	}

	if hadSuccessfulRun {
		return ocrResult{}, nil
	}
	if lastErr != nil {
		return ocrResult{}, lastErr
	}
	return ocrResult{}, nil
}

func runLinuxTesseract(bin string, img image.Image, psm string) (ocrResult, error) {
	if img == nil {
		return ocrResult{}, fmt.Errorf("OCR image is nil")
	}
	b := img.Bounds()
	if b.Dx() < 2 || b.Dy() < 2 {
		return ocrResult{}, fmt.Errorf("OCR image is empty")
	}

	f, err := os.CreateTemp("", "jacob-ocr-*.png")
	if err != nil {
		return ocrResult{}, fmt.Errorf("create OCR temp image: %w", err)
	}
	name := f.Name()
	defer os.Remove(name)

	if err := png.Encode(f, img); err != nil {
		_ = f.Close()
		return ocrResult{}, fmt.Errorf("encode OCR temp image: %w", err)
	}
	if err := f.Close(); err != nil {
		return ocrResult{}, fmt.Errorf("close OCR temp image: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	cmd := exec.CommandContext(
		ctx,
		bin,
		name,
		"stdout",
		"-l", linuxOCRLanguage(),
		"--oem", "1",
		"--psm", psm,
		"tsv",
	)
	cmd.Env = append(os.Environ(), "OMP_THREAD_LIMIT=2")

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		if ctx.Err() != nil {
			return ocrResult{}, fmt.Errorf("Linux OCR timed out")
		}
		msg := strings.TrimSpace(stderr.String())
		if len(msg) > 240 {
			msg = msg[:240]
		}
		if msg != "" {
			return ocrResult{}, fmt.Errorf("Linux OCR failed: %s", msg)
		}
		return ocrResult{}, fmt.Errorf("Linux OCR failed: %w", err)
	}

	return parseTesseractTSV(stdout.Bytes(), b.Dx(), b.Dy()), nil
}

func linuxOCRLightTextVariant(src image.Image) image.Image {
	b := src.Bounds()
	gray := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))

	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := src.At(x, y).RGBA()
			r8 := uint32(r >> 8)
			g8 := uint32(g >> 8)
			b8 := uint32(bl >> 8)
			v := uint8((299*r8 + 587*g8 + 114*b8 + 500) / 1000)
			gray.SetGray(x-b.Min.X, y-b.Min.Y, color.Gray{Y: v})
		}
	}

	threshold := otsuThreshold(gray)
	if threshold < 48 {
		threshold = 48
	}

	for i, v := range gray.Pix {
		if v >= threshold {
			gray.Pix[i] = 0
		} else {
			gray.Pix[i] = 255
		}
	}
	return gray
}

func linuxOCRLanguage() string {
	lang := strings.TrimSpace(os.Getenv("JACOB_OCR_LANG"))
	if lang == "" {
		return "eng"
	}
	for _, r := range lang {
		if !((r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || r == '_' || r == '+' || r == '-') {
			return "eng"
		}
	}
	return lang
}

func ocrInfo() map[string]any {
	bin, err := linuxTesseractPath()
	return map[string]any{
		"available":      err == nil,
		"engine":         "tesseract",
		"strategy":       "psm6 -> psm11 -> light-text-binary/psm11",
		"activeLanguage": linuxOCRLanguage(),
		"binary": func() string {
			if err == nil {
				return bin
			}
			return ""
		}(),
		"error": func() string {
			if err != nil {
				return err.Error()
			}
			return ""
		}(),
	}
}

func linuxTesseractPath() (string, error) {
	candidates := []string{
		strings.TrimSpace(os.Getenv("JACOB_TESSERACT")),
	}
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		candidates = append(candidates, filepath.Join(home, ".local", "share", "JACoB", "ocr", "tesseract", "AppRun"))
	}
	for _, path := range candidates {
		if path == "" {
			continue
		}
		if st, err := os.Stat(path); err == nil && !st.IsDir() && st.Mode()&0111 != 0 {
			return path, nil
		}
	}
	if path, err := exec.LookPath("tesseract"); err == nil {
		return path, nil
	}
	return "", fmt.Errorf("Linux OCR engine is unavailable; reinstall the Steam Deck JACoB package to restore the bundled OCR runtime")
}

func parseTesseractTSV(data []byte, width, height int) ocrResult {
	if width < 1 || height < 1 {
		return ocrResult{}
	}
	type lineAcc struct {
		key            string
		words          []TextWord
		text           []string
		x0, y0, x1, y1 int
	}
	lines := make([]*lineAcc, 0, 24)
	byKey := map[string]*lineAcc{}

	sc := bufio.NewScanner(bytes.NewReader(data))
	sc.Buffer(make([]byte, 4096), 1024*1024)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		fields := strings.SplitN(sc.Text(), "\t", 12)
		if len(fields) < 12 || fields[0] != "5" {
			continue
		}
		wordText := strings.TrimSpace(fields[11])
		if wordText == "" {
			continue
		}
		vals := make([]int, 9)
		ok := true
		// TSV columns 1..9 (page_num through height) are integers.
		// Column 10 is confidence and is normally a decimal, e.g. 96.437.
		// We do not need confidence for the current structured OCR result.
		for i := 1; i <= 9; i++ {
			v, err := strconv.Atoi(fields[i])
			if err != nil {
				ok = false
				break
			}
			vals[i-1] = v
		}
		if !ok {
			continue
		}
		w := tesseractWord{
			page: vals[0], block: vals[1], par: vals[2], line: vals[3],
			left: vals[5], top: vals[6], width: vals[7], height: vals[8], text: wordText,
		}
		if w.width <= 0 || w.height <= 0 {
			continue
		}
		key := fmt.Sprintf("%d/%d/%d/%d", w.page, w.block, w.par, w.line)
		acc := byKey[key]
		if acc == nil {
			acc = &lineAcc{key: key, x0: w.left, y0: w.top, x1: w.left + w.width, y1: w.top + w.height}
			byKey[key] = acc
			lines = append(lines, acc)
		} else {
			if w.left < acc.x0 {
				acc.x0 = w.left
			}
			if w.top < acc.y0 {
				acc.y0 = w.top
			}
			if w.left+w.width > acc.x1 {
				acc.x1 = w.left + w.width
			}
			if w.top+w.height > acc.y1 {
				acc.y1 = w.top + w.height
			}
		}
		acc.text = append(acc.text, wordText)
		acc.words = append(acc.words, TextWord{
			Text:   wordText,
			Bounds: normalizeOCRBounds(w.left, w.top, w.width, w.height, width, height),
		})
	}

	out := ocrResult{Lines: make([]TextLine, 0, len(lines))}
	all := make([]string, 0, len(lines))
	for _, acc := range lines {
		text := strings.Join(acc.text, " ")
		if strings.TrimSpace(text) == "" {
			continue
		}
		line := TextLine{
			Text:   text,
			Bounds: normalizeOCRBounds(acc.x0, acc.y0, acc.x1-acc.x0, acc.y1-acc.y0, width, height),
			Words:  append([]TextWord(nil), acc.words...),
		}
		out.Lines = append(out.Lines, line)
		all = append(all, text)
	}
	out.Text = strings.Join(all, "\n")
	return out
}

func normalizeOCRBounds(x, y, w, h, width, height int) Region {
	clamp := func(v float64) float64 {
		if v < 0 {
			return 0
		}
		if v > 1 {
			return 1
		}
		return v
	}
	x0 := clamp(float64(x) / float64(width))
	y0 := clamp(float64(y) / float64(height))
	x1 := clamp(float64(x+w) / float64(width))
	y1 := clamp(float64(y+h) / float64(height))
	return Region{X: x0, Y: y0, Width: x1 - x0, Height: y1 - y0}
}
