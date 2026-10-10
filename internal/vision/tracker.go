package vision

import (
	"image"
	"math"
	"sort"
	"sync"
	"time"

	"jacob/internal/platform"
)

// Feature is a deliberately low-information visual landmark. Coordinates are
// normalized to [0,1] so custom tabs can visualize feature motion without ever
// receiving the underlying frame pixels.
type Feature struct {
	X     float64 `json:"x"`
	Y     float64 `json:"y"`
	Score float64 `json:"score"`
}

type Motion struct {
	DX         float64 `json:"dx"`
	DY         float64 `json:"dy"`
	Confidence float64 `json:"confidence"`
	Compared   bool    `json:"compared"`
}

// SpatialFeature is an internal low-information feature descriptor used by the
// spatial mapper. It is never exposed through the custom-tab Vision API.
type SpatialFeature struct {
	X         float64
	Y         float64
	Score     float64
	Signature uint64
}

type SpatialFrame struct {
	At        string
	Available bool
	Blanked   bool
	Reason    string
	Width     int
	Height    int
	Features  []SpatialFeature
	Motion    Motion
}

type Sample struct {
	At           string    `json:"at"`
	Available    bool      `json:"available"`
	Blanked      bool      `json:"blanked"`
	Reason       string    `json:"reason,omitempty"`
	Width        int       `json:"width"`
	Height       int       `json:"height"`
	FeatureCount int       `json:"featureCount"`
	Features     []Feature `json:"features"`
	MeanLuma     float64   `json:"meanLuma"`
	Contrast     float64   `json:"contrast"`
	Motion       Motion    `json:"motion"`
}

type Tracker struct {
	mu          sync.Mutex
	capture     platform.CaptureDriver
	prev        []uint8
	prevW       int
	prevH       int
	lastCapture time.Time
	lastSample  Sample
	lastSpatial SpatialFrame
}

func New(c platform.CaptureDriver) *Tracker { return &Tracker{capture: c} }

func (t *Tracker) Info() map[string]any {
	available := t != nil && t.capture != nil && t.capture.Available()
	driver := "vision-unavailable"
	if t != nil && t.capture != nil {
		driver = t.capture.Name()
	}
	return map[string]any{
		"available":         available,
		"captureDriver":     driver,
		"eliteOnly":         true,
		"foregroundOnly":    true,
		"rawFramesExposed":  false,
		"derived":           []string{"features", "frame-motion", "luma", "contrast", "region-color", "region-color-fill", "region-statistics", "text", "text-lines"},
		"inspectOperations": []string{"colorPresent", "colorCoverage", "colorVerticalFill", "luma", "contrast", "edgeDensity", "text", "textLines"},
	}
}

func (t *Tracker) Sample(maxWidth int) (Sample, error) {
	out := Sample{At: time.Now().UTC().Format(time.RFC3339Nano), Features: []Feature{}}
	if t == nil || t.capture == nil || !t.capture.Available() {
		return out, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	// Bound capture/analysis cost even for direct paired clients. Custom tabs
	// are also rate-limited by the host bridge, but the core remains safe when
	// called through the WebSocket protocol directly.
	if !t.lastCapture.IsZero() && time.Since(t.lastCapture) < 75*time.Millisecond {
		return cloneSample(t.lastSample), nil
	}
	t.lastCapture = time.Now()
	if maxWidth < 160 {
		maxWidth = 320
	}
	if maxWidth > 640 {
		maxWidth = 640
	}
	frame, err := t.capture.CaptureFrame(maxWidth, platform.CaptureIntentLive)
	if err != nil {
		return out, err
	}
	info := platform.CaptureInfoForFrame(frame)
	out.Available = true
	out.Blanked = info.Blanked
	out.Reason = info.Reason
	out.Width = info.Width
	out.Height = info.Height
	if info.Blanked {
		t.prev = nil
		t.prevW, t.prevH = 0, 0
		t.lastSample = cloneSample(out)
		t.lastSpatial = SpatialFrame{At: out.At, Available: out.Available, Blanked: true, Reason: out.Reason, Width: out.Width, Height: out.Height}
		return out, nil
	}
	img, err := frame.Image()
	if err != nil {
		return out, err
	}
	gray, w, h := downsampleGray(img, 192)
	features, mean, contrast := analyze(gray, w, h)
	spatialFeatures := spatializeFeatures(gray, w, h, features)
	out.Features = features
	out.FeatureCount = len(features)
	out.MeanLuma = mean
	out.Contrast = contrast

	if len(t.prev) == len(gray) && t.prevW == w && t.prevH == h && len(gray) > 0 {
		dx, dy, confidence := estimateTranslation(t.prev, gray, w, h)
		out.Motion = Motion{DX: dx / float64(w), DY: dy / float64(h), Confidence: confidence, Compared: true}
	}
	t.prev = append(t.prev[:0], gray...)
	t.prevW, t.prevH = w, h
	t.lastSample = cloneSample(out)
	t.lastSpatial = SpatialFrame{
		At: out.At, Available: out.Available, Blanked: out.Blanked, Reason: out.Reason,
		Width: out.Width, Height: out.Height, Features: spatialFeatures, Motion: out.Motion,
	}
	return out, nil
}

func cloneSample(in Sample) Sample {
	out := in
	out.Features = append([]Feature(nil), in.Features...)
	return out
}

// SpatialFrame returns derived feature descriptors for trusted core services.
// The public custom-tab Vision API continues to expose only Sample/Inspect.
func (t *Tracker) SpatialFrame(maxWidth int) (SpatialFrame, error) {
	sample, err := t.Sample(maxWidth)
	if err != nil {
		return SpatialFrame{}, err
	}
	if !sample.Available {
		return SpatialFrame{At: sample.At, Available: false}, nil
	}
	t.mu.Lock()
	defer t.mu.Unlock()
	out := t.lastSpatial
	out.Features = append([]SpatialFeature(nil), t.lastSpatial.Features...)
	if out.At == "" {
		out.At = sample.At
		out.Available = sample.Available
		out.Blanked = sample.Blanked
		out.Reason = sample.Reason
		out.Width = sample.Width
		out.Height = sample.Height
		out.Motion = sample.Motion
	}
	return out, nil
}

func spatializeFeatures(gray []uint8, w, h int, features []Feature) []SpatialFeature {
	out := make([]SpatialFeature, 0, len(features))
	if w <= 0 || h <= 0 || len(gray) != w*h {
		return out
	}
	for _, f := range features {
		x := int(math.Round(f.X * float64(w-1)))
		y := int(math.Round(f.Y * float64(h-1)))
		out = append(out, SpatialFeature{X: f.X, Y: f.Y, Score: f.Score, Signature: featureSignature(gray, w, h, x, y)})
	}
	return out
}

func featureSignature(gray []uint8, w, h, x, y int) uint64 {
	if w <= 0 || h <= 0 || len(gray) != w*h {
		return 0
	}
	var sig uint64
	for i := 0; i < 64; i++ {
		ax := ((i*5 + 1) % 9) - 4
		ay := ((i*7 + 3) % 9) - 4
		bx := ((i*11 + 2) % 9) - 4
		by := ((i*13 + 5) % 9) - 4
		x1, y1 := clampPixel(x+ax, w), clampPixel(y+ay, h)
		x2, y2 := clampPixel(x+bx, w), clampPixel(y+by, h)
		if gray[y1*w+x1] < gray[y2*w+x2] {
			sig |= uint64(1) << uint(i)
		}
	}
	return sig
}

func clampPixel(v, size int) int {
	if v < 0 {
		return 0
	}
	if v >= size {
		return size - 1
	}
	return v
}

func downsampleGray(img image.Image, maxWidth int) ([]uint8, int, int) {
	b := img.Bounds()
	sw, sh := b.Dx(), b.Dy()
	if sw <= 0 || sh <= 0 {
		return nil, 0, 0
	}
	w := sw
	if w > maxWidth {
		w = maxWidth
	}
	h := int(math.Round(float64(sh) * float64(w) / float64(sw)))
	if h < 1 {
		h = 1
	}
	out := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		sy := b.Min.Y + y*sh/h
		for x := 0; x < w; x++ {
			sx := b.Min.X + x*sw/w
			r, g, bl, _ := img.At(sx, sy).RGBA()
			// Integer approximation to Rec.601 luma using 8-bit channels.
			r8, g8, b8 := uint32(r>>8), uint32(g>>8), uint32(bl>>8)
			out[y*w+x] = uint8((299*r8 + 587*g8 + 114*b8) / 1000)
		}
	}
	return out, w, h
}

func analyze(gray []uint8, w, h int) ([]Feature, float64, float64) {
	if w < 5 || h < 5 || len(gray) != w*h {
		return []Feature{}, 0, 0
	}
	var sum, sum2 float64
	for _, v := range gray {
		f := float64(v) / 255
		sum += f
		sum2 += f * f
	}
	mean := sum / float64(len(gray))
	variance := math.Max(0, sum2/float64(len(gray))-mean*mean)
	contrast := math.Sqrt(variance)

	type candidate struct {
		x, y  int
		score float64
	}
	candidates := make([]candidate, 0, 600)
	for y := 2; y < h-2; y += 2 {
		for x := 2; x < w-2; x += 2 {
			gx := math.Abs(float64(gray[y*w+x+1]) - float64(gray[y*w+x-1]))
			gy := math.Abs(float64(gray[(y+1)*w+x]) - float64(gray[(y-1)*w+x]))
			d1 := math.Abs(float64(gray[(y+1)*w+x+1]) - float64(gray[(y-1)*w+x-1]))
			d2 := math.Abs(float64(gray[(y+1)*w+x-1]) - float64(gray[(y-1)*w+x+1]))
			score := math.Min(gx+0.35*(d1+d2), gy+0.35*(d1+d2))
			if score >= 36 {
				candidates = append(candidates, candidate{x, y, score})
			}
		}
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].score > candidates[j].score })
	features := make([]Feature, 0, 120)
	chosen := make([]candidate, 0, 120)
	for _, c := range candidates {
		ok := true
		for _, p := range chosen {
			dx, dy := c.x-p.x, c.y-p.y
			if dx*dx+dy*dy < 36 {
				ok = false
				break
			}
		}
		if !ok {
			continue
		}
		chosen = append(chosen, c)
		features = append(features, Feature{X: float64(c.x) / float64(w-1), Y: float64(c.y) / float64(h-1), Score: math.Min(1, c.score/255)})
		if len(features) >= 120 {
			break
		}
	}
	return features, mean, contrast
}

func estimateTranslation(prev, cur []uint8, w, h int) (float64, float64, float64) {
	if len(prev) != len(cur) || w < 40 || h < 30 {
		return 0, 0, 0
	}
	bestScore := math.Inf(1)
	second := math.Inf(1)
	bestX, bestY := 0, 0
	const search = 8
	// Sample every third pixel and avoid borders. This is intentionally coarse:
	// it reports global apparent frame motion without exposing frame content.
	for dy := -search; dy <= search; dy++ {
		for dx := -search; dx <= search; dx++ {
			var sad float64
			count := 0
			for y := 12; y < h-12; y += 3 {
				sy := y + dy
				if sy < 0 || sy >= h {
					continue
				}
				for x := 12; x < w-12; x += 3 {
					sx := x + dx
					if sx < 0 || sx >= w {
						continue
					}
					a := int(prev[y*w+x])
					b := int(cur[sy*w+sx])
					if a > b {
						sad += float64(a - b)
					} else {
						sad += float64(b - a)
					}
					count++
				}
			}
			if count == 0 {
				continue
			}
			score := sad / float64(count)
			if score < bestScore {
				second = bestScore
				bestScore, bestX, bestY = score, dx, dy
			} else if score < second {
				second = score
			}
		}
	}
	confidence := 0.0
	if math.IsInf(bestScore, 0) {
		return 0, 0, 0
	}
	if second < math.Inf(1) && second > 0 {
		confidence = math.Max(0, math.Min(1, (second-bestScore)/second*8))
	}
	// Also reduce confidence when frames differ too much globally.
	confidence *= math.Max(0, math.Min(1, 1-bestScore/90))
	return float64(bestX), float64(bestY), confidence
}
