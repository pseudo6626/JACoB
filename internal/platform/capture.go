package platform

import (
	"bytes"
	"fmt"
	"image"
	"image/color"
	"image/jpeg"
	"time"
)

// CaptureIntent describes why JACoB needs a frame. Backends may use this to
// perform platform-specific focus handling, but consumers never need to know
// how the operating system produced the frame.
type CaptureIntent string

const (
	CaptureIntentLive        CaptureIntent = "live"
	CaptureIntentCalibration CaptureIntent = "calibration"
)

// PixelFormat is deliberately tiny for SDK 10. The canonical in-core frame is
// RGBA8 so Vision, OCR and presentation code all consume the same pixels.
type PixelFormat string

const PixelFormatRGBA8 PixelFormat = "rgba8"

// GameFrame is the OS-neutral privacy boundary for JACoB capture.
//
// A successful non-blank frame is always normalized to the verified Elite
// Dangerous client area. Backends must never return desktop/monitor pixels in
// Pix. If they cannot safely isolate Elite, they must return a blank frame or
// an error instead.
type GameFrame struct {
	Pix          []byte
	Width        int
	Height       int
	Stride       int
	Format       PixelFormat
	CapturedAt   time.Time
	Driver       string
	SourceWidth  int
	SourceHeight int
	Verified     bool
	Foreground   bool
	Blanked      bool
	Reason       string
}

func (f GameFrame) Valid() bool {
	if f.Blanked {
		return f.Width > 0 && f.Height > 0
	}
	return f.Verified && f.Width > 0 && f.Height > 0 && f.Stride >= f.Width*4 && len(f.Pix) >= f.Stride*f.Height && f.Format == PixelFormatRGBA8
}

func (f GameFrame) Image() (*image.RGBA, error) {
	if f.Width <= 0 || f.Height <= 0 {
		return nil, fmt.Errorf("capture frame has invalid dimensions %dx%d", f.Width, f.Height)
	}
	if f.Blanked {
		img := image.NewRGBA(image.Rect(0, 0, f.Width, f.Height))
		black := color.RGBA{0, 0, 0, 255}
		for y := 0; y < f.Height; y++ {
			for x := 0; x < f.Width; x++ {
				img.SetRGBA(x, y, black)
			}
		}
		return img, nil
	}
	if !f.Valid() {
		return nil, fmt.Errorf("capture backend returned an invalid canonical Elite frame")
	}
	return &image.RGBA{Pix: f.Pix, Stride: f.Stride, Rect: image.Rect(0, 0, f.Width, f.Height)}, nil
}

type CaptureInfo struct {
	Driver       string `json:"driver"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	SourceWidth  int    `json:"sourceWidth"`
	SourceHeight int    `json:"sourceHeight"`
	Blanked      bool   `json:"blanked,omitempty"`
	Reason       string `json:"reason,omitempty"`
	Canonical    bool   `json:"canonicalEliteFrame,omitempty"`
	PixelFormat  string `json:"pixelFormat,omitempty"`
}

// CaptureDriver is the stable OS-neutral capture contract. Platform backends
// provide CaptureFrame. CaptureJPEG remains as a compatibility/presentation
// helper and must be derived from that same canonical frame.
type CaptureDriver interface {
	Name() string
	Available() bool
	CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error)
	CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error)
}

// PrivacyMatteCaptureDriver is an optional Game View presentation capability.
// It is not part of the canonical frame contract and Vision never calls it.
type PrivacyMatteCaptureDriver interface {
	CapturePrivacyMatteJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error)
}

func NewCaptureDriver() CaptureDriver { return newCaptureDriver() }

func CaptureInfoForFrame(f GameFrame) CaptureInfo {
	return CaptureInfo{
		Driver:       f.Driver,
		Width:        f.Width,
		Height:       f.Height,
		SourceWidth:  f.SourceWidth,
		SourceHeight: f.SourceHeight,
		Blanked:      f.Blanked,
		Reason:       f.Reason,
		Canonical:    true,
		PixelFormat:  string(f.Format),
	}
}

func EncodeGameFrameJPEG(f GameFrame, quality int) ([]byte, CaptureInfo, error) {
	img, err := f.Image()
	if err != nil {
		return nil, CaptureInfoForFrame(f), err
	}
	if quality < 20 {
		quality = 20
	}
	if quality > 95 {
		quality = 95
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, CaptureInfoForFrame(f), err
	}
	return buf.Bytes(), CaptureInfoForFrame(f), nil
}

func blankGameFrame(driver string, width, height int, reason string) GameFrame {
	if width < 160 {
		width = 960
	}
	if height < 1 {
		height = width * 9 / 16
	}
	if height < 90 {
		height = 90
	}
	return GameFrame{
		Width:        width,
		Height:       height,
		Stride:       width * 4,
		Format:       PixelFormatRGBA8,
		CapturedAt:   time.Now().UTC(),
		Driver:       driver,
		SourceWidth:  width,
		SourceHeight: height,
		Verified:     false,
		Foreground:   false,
		Blanked:      true,
		Reason:       reason,
	}
}
