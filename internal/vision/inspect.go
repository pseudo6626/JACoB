package vision

import (
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/color"
	"math"
	"strings"
	"time"

	"jacob/internal/platform"
)

// Region is normalized to the verified Elite Dangerous client area, or to a
// parent Vision region when used as Operation.Subregion.
type Region struct {
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}

// Operation describes a derived observation. No operation can return pixels.
// Subregion, when present, is normalized inside InspectRequest.Region.
type Operation struct {
	Type            string          `json:"type"`
	Color           string          `json:"color,omitempty"`
	Tolerance       float64         `json:"tolerance,omitempty"`
	MinimumCoverage float64         `json:"minimumCoverage,omitempty"`
	FullThreshold   float64         `json:"fullThreshold,omitempty"`
	Subregion       *Region         `json:"subregion,omitempty"`
	Preprocess      *TextPreprocess `json:"preprocess,omitempty"`
}

// TextPreprocess describes optional in-core OCR cleanup. The transformed image
// is never returned to a tab. It exists only for the duration of recognition.
type TextPreprocess struct {
	Scale            float64 `json:"scale,omitempty"`
	Grayscale        bool    `json:"grayscale,omitempty"`
	AutoContrast     bool    `json:"autoContrast,omitempty"`
	AutoThreshold    bool    `json:"autoThreshold,omitempty"`
	Invert           bool    `json:"invert,omitempty"`
	IsolateColor     string  `json:"isolateColor,omitempty"`
	IsolateTolerance float64 `json:"isolateTolerance,omitempty"`
}

type InspectRequest struct {
	Region     Region               `json:"region"`
	Operations map[string]Operation `json:"operations"`
	MaxWidth   int                  `json:"maxWidth,omitempty"`
}

type TextWord struct {
	Text   string `json:"text"`
	Bounds Region `json:"bounds"`
}

type TextLine struct {
	Text   string     `json:"text"`
	Bounds Region     `json:"bounds"`
	Words  []TextWord `json:"words,omitempty"`
}

type Observation struct {
	Type       string     `json:"type"`
	Matched    *bool      `json:"matched,omitempty"`
	Number     *float64   `json:"number,omitempty"`
	Text       string     `json:"text,omitempty"`
	Lines      []TextLine `json:"lines,omitempty"`
	Confidence float64    `json:"confidence,omitempty"`
	Samples    int        `json:"samples,omitempty"`
}

type InspectResult struct {
	At        string                 `json:"at"`
	Available bool                   `json:"available"`
	Blanked   bool                   `json:"blanked"`
	Reason    string                 `json:"reason,omitempty"`
	Region    Region                 `json:"region"`
	Results   map[string]Observation `json:"results"`
}

func (t *Tracker) Inspect(req InspectRequest) (InspectResult, error) {
	out := InspectResult{
		At:      time.Now().UTC().Format(time.RFC3339Nano),
		Region:  req.Region,
		Results: map[string]Observation{},
	}
	if t == nil || t.capture == nil || !t.capture.Available() {
		return out, nil
	}
	if err := validateInspect(req); err != nil {
		return out, err
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	if !t.lastCapture.IsZero() && time.Since(t.lastCapture) < 125*time.Millisecond {
		return out, errors.New("vision inspection rate limited")
	}
	t.lastCapture = time.Now()

	maxWidth := req.MaxWidth
	if maxWidth == 0 {
		maxWidth = 1280
	}
	if maxWidth < 480 {
		maxWidth = 480
	}
	if maxWidth > 1600 {
		maxWidth = 1600
	}
	frame, err := t.capture.CaptureFrame(maxWidth, platform.CaptureIntentLive)
	if err != nil {
		return out, err
	}
	info := platform.CaptureInfoForFrame(frame)
	out.Available = true
	out.Blanked = info.Blanked
	out.Reason = info.Reason
	if info.Blanked {
		return out, nil
	}

	img, err := frame.Image()
	if err != nil {
		return out, err
	}
	crop, err := cropRegion(img, req.Region)
	if err != nil {
		return out, err
	}

	for name, op := range req.Operations {
		opImage := image.Image(crop)
		if op.Subregion != nil {
			opImage, err = cropRegion(crop, *op.Subregion)
			if err != nil {
				return out, fmt.Errorf("operation %q subregion: %w", name, err)
			}
		}

		obs := Observation{Type: op.Type}
		switch op.Type {
		case "colorCoverage", "colorPresent", "colorVerticalFill":
			target, err := parseColor(op.Color)
			if err != nil {
				return out, fmt.Errorf("operation %q: %w", name, err)
			}
			tol := op.Tolerance
			if tol == 0 {
				tol = 55
			}
			switch op.Type {
			case "colorCoverage", "colorPresent":
				coverage, n := regionColorCoverage(opImage, target, tol)
				obs.Number = numberPtr(coverage)
				obs.Samples = n
				if op.Type == "colorPresent" {
					threshold := op.MinimumCoverage
					if threshold == 0 {
						threshold = 0.03
					}
					matched := coverage >= threshold
					obs.Matched = &matched
					obs.Confidence = coverageConfidence(coverage, threshold)
				}
			case "colorVerticalFill":
				rowThreshold := op.MinimumCoverage
				if rowThreshold == 0 {
					rowThreshold = 0.05
				}
				fill, n := regionColorVerticalFill(opImage, target, tol, rowThreshold)
				obs.Number = numberPtr(fill)
				obs.Samples = n
				obs.Confidence = 1
				if op.FullThreshold > 0 {
					matched := fill >= op.FullThreshold
					obs.Matched = &matched
					obs.Confidence = fillThresholdConfidence(fill, op.FullThreshold)
				}
			}
		case "luma", "contrast", "edgeDensity":
			mean, contrast, edgeDensity, statSamples := regionStats(opImage)
			obs.Samples = statSamples
			obs.Confidence = 1
			switch op.Type {
			case "luma":
				obs.Number = numberPtr(mean)
			case "contrast":
				obs.Number = numberPtr(contrast)
			case "edgeDensity":
				obs.Number = numberPtr(edgeDensity)
			}
		case "text", "textLines":
			processed, err := preprocessText(opImage, op.Preprocess)
			if err != nil {
				return out, fmt.Errorf("operation %q preprocess: %w", name, err)
			}
			ocr, err := ocrTextLines(processed)
			if err != nil {
				return out, fmt.Errorf("operation %q: %w", name, err)
			}
			obs.Text = strings.TrimSpace(ocr.Text)
			if op.Type == "textLines" {
				obs.Lines = append([]TextLine(nil), ocr.Lines...)
			}
		case "changed":
			return out, fmt.Errorf("operation %q: changed is reserved for the watch API and is not available in inspect", name)
		default:
			return out, fmt.Errorf("operation %q: unsupported type %q", name, op.Type)
		}
		out.Results[name] = obs
	}
	return out, nil
}

func validateInspect(req InspectRequest) error {
	if err := validateRegion(req.Region); err != nil {
		return err
	}
	if len(req.Operations) < 1 || len(req.Operations) > 8 {
		return errors.New("vision inspect requires 1..8 operations")
	}
	for name, op := range req.Operations {
		if strings.TrimSpace(name) == "" || len(name) > 64 {
			return errors.New("vision operation names must be 1..64 characters")
		}
		if op.Subregion != nil {
			if err := validateRegion(*op.Subregion); err != nil {
				return fmt.Errorf("operation %q subregion: %w", name, err)
			}
			effective := Region{
				X:      req.Region.X + op.Subregion.X*req.Region.Width,
				Y:      req.Region.Y + op.Subregion.Y*req.Region.Height,
				Width:  req.Region.Width * op.Subregion.Width,
				Height: req.Region.Height * op.Subregion.Height,
			}
			if err := validateRegion(effective); err != nil {
				return fmt.Errorf("operation %q effective subregion: %w", name, err)
			}
		}
		switch op.Type {
		case "colorCoverage", "colorPresent", "colorVerticalFill":
			if _, err := parseColor(op.Color); err != nil {
				return fmt.Errorf("operation %q: %w", name, err)
			}
			if op.Tolerance < 0 || op.Tolerance > 255 {
				return fmt.Errorf("operation %q: tolerance must be 0..255", name)
			}
			if op.MinimumCoverage < 0 || op.MinimumCoverage > 1 {
				return fmt.Errorf("operation %q: minimumCoverage must be 0..1", name)
			}
			if op.FullThreshold < 0 || op.FullThreshold > 1 {
				return fmt.Errorf("operation %q: fullThreshold must be 0..1", name)
			}
			if op.Type != "colorVerticalFill" && op.FullThreshold != 0 {
				return fmt.Errorf("operation %q: fullThreshold is only valid for colorVerticalFill", name)
			}
		case "luma", "contrast", "edgeDensity":
		case "text", "textLines":
			if err := validateTextPreprocess(op.Preprocess); err != nil {
				return fmt.Errorf("operation %q preprocess: %w", name, err)
			}
		default:
			return fmt.Errorf("operation %q: unsupported type %q", name, op.Type)
		}
	}
	return nil
}

func validateRegion(r Region) error {
	const eps = 1e-9
	if r.X < -eps || r.Y < -eps || r.Width <= 0 || r.Height <= 0 || r.X+r.Width > 1+eps || r.Y+r.Height > 1+eps {
		return errors.New("vision region must be normalized and remain inside its parent area")
	}
	return nil
}

func cropRegion(img image.Image, r Region) (image.Image, error) {
	b := img.Bounds()
	x0 := b.Min.X + int(math.Round(r.X*float64(b.Dx())))
	y0 := b.Min.Y + int(math.Round(r.Y*float64(b.Dy())))
	x1 := b.Min.X + int(math.Round((r.X+r.Width)*float64(b.Dx())))
	y1 := b.Min.Y + int(math.Round((r.Y+r.Height)*float64(b.Dy())))
	if x0 < b.Min.X {
		x0 = b.Min.X
	}
	if y0 < b.Min.Y {
		y0 = b.Min.Y
	}
	if x1 > b.Max.X {
		x1 = b.Max.X
	}
	if y1 > b.Max.Y {
		y1 = b.Max.Y
	}
	if x1-x0 < 2 || y1-y0 < 2 {
		return nil, errors.New("vision region produced an empty crop")
	}
	out := image.NewRGBA(image.Rect(0, 0, x1-x0, y1-y0))
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			out.Set(x-x0, y-y0, img.At(x, y))
		}
	}
	return out, nil
}

func validateTextPreprocess(p *TextPreprocess) error {
	if p == nil {
		return nil
	}
	if p.Scale != 0 && (p.Scale < 1 || p.Scale > 4) {
		return errors.New("scale must be between 1 and 4")
	}
	if p.IsolateTolerance < 0 || p.IsolateTolerance > 255 {
		return errors.New("isolateTolerance must be 0..255")
	}
	if strings.TrimSpace(p.IsolateColor) != "" {
		if _, err := parseColor(p.IsolateColor); err != nil {
			return fmt.Errorf("isolateColor: %w", err)
		}
	}
	return nil
}

func preprocessText(src image.Image, cfg *TextPreprocess) (image.Image, error) {
	if cfg == nil {
		return src, nil
	}
	b := src.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 {
		return src, nil
	}
	gray := image.NewGray(image.Rect(0, 0, b.Dx(), b.Dy()))
	isolate := strings.TrimSpace(cfg.IsolateColor) != ""
	var target color.RGBA
	var tol2 float64
	if isolate {
		var err error
		target, err = parseColor(cfg.IsolateColor)
		if err != nil {
			return nil, err
		}
		tol := cfg.IsolateTolerance
		if tol == 0 {
			tol = 85
		}
		tol2 = tol * tol * 3
	}
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := src.At(x, y).RGBA()
			r8, g8, b8 := float64(r>>8), float64(g>>8), float64(bl>>8)
			var v uint8
			if isolate {
				dr := r8 - float64(target.R)
				dg := g8 - float64(target.G)
				db := b8 - float64(target.B)
				if dr*dr+dg*dg+db*db <= tol2 {
					v = 0
				} else {
					v = 255
				}
			} else {
				v = uint8(math.Max(0, math.Min(255, 0.299*r8+0.587*g8+0.114*b8)))
			}
			gray.SetGray(x-b.Min.X, y-b.Min.Y, color.Gray{Y: v})
		}
	}

	if cfg.AutoContrast && !isolate {
		minV, maxV := uint8(255), uint8(0)
		for _, v := range gray.Pix {
			if v < minV {
				minV = v
			}
			if v > maxV {
				maxV = v
			}
		}
		if maxV > minV+4 {
			span := float64(maxV - minV)
			for i, v := range gray.Pix {
				gray.Pix[i] = uint8(math.Round((float64(v-minV) / span) * 255))
			}
		}
	}

	if cfg.AutoThreshold && !isolate {
		threshold := otsuThreshold(gray)
		for i, v := range gray.Pix {
			if v >= threshold {
				gray.Pix[i] = 255
			} else {
				gray.Pix[i] = 0
			}
		}
	}
	if cfg.Invert {
		for i, v := range gray.Pix {
			gray.Pix[i] = 255 - v
		}
	}

	scale := cfg.Scale
	if scale == 0 {
		scale = 1
	}
	if math.Abs(scale-1) < 0.001 {
		return gray, nil
	}
	dw := int(math.Round(float64(gray.Bounds().Dx()) * scale))
	dh := int(math.Round(float64(gray.Bounds().Dy()) * scale))
	if dw < 1 || dh < 1 {
		return gray, nil
	}
	out := image.NewGray(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		sy := int(float64(y) / scale)
		if sy >= gray.Bounds().Dy() {
			sy = gray.Bounds().Dy() - 1
		}
		for x := 0; x < dw; x++ {
			sx := int(float64(x) / scale)
			if sx >= gray.Bounds().Dx() {
				sx = gray.Bounds().Dx() - 1
			}
			out.SetGray(x, y, gray.GrayAt(sx, sy))
		}
	}
	return out, nil
}

func otsuThreshold(img *image.Gray) uint8 {
	var hist [256]int
	total := 0
	for _, v := range img.Pix {
		hist[int(v)]++
		total++
	}
	if total == 0 {
		return 128
	}
	var sum float64
	for i, n := range hist {
		sum += float64(i * n)
	}
	var sumB float64
	wB := 0
	best := -1.0
	threshold := 128
	for i, n := range hist {
		wB += n
		if wB == 0 {
			continue
		}
		wF := total - wB
		if wF == 0 {
			break
		}
		sumB += float64(i * n)
		mB := sumB / float64(wB)
		mF := (sum - sumB) / float64(wF)
		between := float64(wB*wF) * (mB - mF) * (mB - mF)
		if between > best {
			best = between
			threshold = i
		}
	}
	return uint8(threshold)
}

func parseColor(s string) (color.RGBA, error) {
	s = strings.TrimPrefix(strings.TrimSpace(s), "#")
	if len(s) != 6 {
		return color.RGBA{}, errors.New("color must be #RRGGBB")
	}
	b, err := hex.DecodeString(s)
	if err != nil {
		return color.RGBA{}, errors.New("color must be #RRGGBB")
	}
	return color.RGBA{R: b[0], G: b[1], B: b[2], A: 255}, nil
}

func regionColorCoverage(img image.Image, target color.RGBA, tolerance float64) (float64, int) {
	b := img.Bounds()
	step := 1
	if b.Dx()*b.Dy() > 200000 {
		step = 2
	}
	tr, tg, tb := float64(target.R), float64(target.G), float64(target.B)
	tol2 := tolerance * tolerance * 3
	hits, total := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r, g, bl, _ := img.At(x, y).RGBA()
			dr := float64(r>>8) - tr
			dg := float64(g>>8) - tg
			db := float64(bl>>8) - tb
			if dr*dr+dg*dg+db*db <= tol2 {
				hits++
			}
			total++
		}
	}
	if total == 0 {
		return 0, 0
	}
	return float64(hits) / float64(total), total
}

// regionColorVerticalFill estimates a bottom-up segmented gauge fill. A row is
// considered active when enough of that row matches the target color. The
// returned value is 0..1 and depends only on the top-most active row, making it
// useful for segmented HUD gauges with gaps between segments.
func regionColorVerticalFill(img image.Image, target color.RGBA, tolerance, minimumRowCoverage float64) (float64, int) {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	if w < 1 || h < 1 {
		return 0, 0
	}
	tr, tg, tb := float64(target.R), float64(target.G), float64(target.B)
	tol2 := tolerance * tolerance * 3
	top := -1
	samples := 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		hits := 0
		for x := b.Min.X; x < b.Max.X; x++ {
			r, g, bl, _ := img.At(x, y).RGBA()
			dr := float64(r>>8) - tr
			dg := float64(g>>8) - tg
			db := float64(bl>>8) - tb
			if dr*dr+dg*dg+db*db <= tol2 {
				hits++
			}
			samples++
		}
		if float64(hits)/float64(w) >= minimumRowCoverage && top < 0 {
			top = y - b.Min.Y
		}
	}
	if top < 0 {
		return 0, samples
	}
	if h == 1 {
		return 1, samples
	}
	fill := 1 - float64(top)/float64(h-1)
	if fill < 0 {
		fill = 0
	}
	if fill > 1 {
		fill = 1
	}
	return fill, samples
}

func regionStats(img image.Image) (mean, contrast, edgeDensity float64, samples int) {
	b := img.Bounds()
	step := 1
	if b.Dx()*b.Dy() > 200000 {
		step = 2
	}
	w := (b.Dx() + step - 1) / step
	vals := make([]float64, 0, w*((b.Dy()+step-1)/step))
	var sum, sum2 float64
	for y := b.Min.Y; y < b.Max.Y; y += step {
		for x := b.Min.X; x < b.Max.X; x += step {
			r, g, bl, _ := img.At(x, y).RGBA()
			v := (0.299*float64(r>>8) + 0.587*float64(g>>8) + 0.114*float64(bl>>8)) / 255
			vals = append(vals, v)
			sum += v
			sum2 += v * v
		}
	}
	if len(vals) == 0 {
		return 0, 0, 0, 0
	}
	mean = sum / float64(len(vals))
	contrast = math.Sqrt(math.Max(0, sum2/float64(len(vals))-mean*mean))
	h := len(vals) / w
	edges, total := 0, 0
	if w > 2 && h > 2 && w*h == len(vals) {
		for y := 1; y < h-1; y++ {
			for x := 1; x < w-1; x++ {
				gx := math.Abs(vals[y*w+x+1] - vals[y*w+x-1])
				gy := math.Abs(vals[(y+1)*w+x] - vals[(y-1)*w+x])
				if gx+gy > 0.22 {
					edges++
				}
				total++
			}
		}
	}
	if total > 0 {
		edgeDensity = float64(edges) / float64(total)
	}
	return mean, contrast, edgeDensity, len(vals)
}

func fillThresholdConfidence(fill, threshold float64) float64 {
	if threshold <= 0 {
		return 1
	}
	// Confidence expresses distance from the decision boundary, not OCR-like certainty.
	d := math.Abs(fill - threshold)
	return math.Max(0, math.Min(1, d/0.12))
}

func coverageConfidence(coverage, threshold float64) float64 {
	if threshold <= 0 {
		return 1
	}
	d := math.Abs(coverage - threshold)
	return math.Max(0, math.Min(1, d/math.Max(threshold, 0.01)))
}

func numberPtr(v float64) *float64 { return &v }
