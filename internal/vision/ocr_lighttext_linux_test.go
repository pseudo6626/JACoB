//go:build linux

package vision

import (
	"image"
	"image/color"
	"testing"
)

func TestLinuxOCRLightTextVariant(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 100, 50))
	for y := 0; y < 50; y++ {
		for x := 0; x < 100; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 8, G: 8, B: 8, A: 255})
		}
	}

	for y := 15; y < 35; y++ {
		for x := 25; x < 30; x++ {
			img.SetRGBA(x, y, color.RGBA{R: 240, G: 240, B: 240, A: 255})
		}
	}

	out := linuxOCRLightTextVariant(img)
	g, ok := out.(*image.Gray)
	if !ok {
		t.Fatalf("expected *image.Gray, got %T", out)
	}
	if got := g.GrayAt(27, 25).Y; got > 32 {
		t.Fatalf("bright text should become dark ink, got %d", got)
	}
	if got := g.GrayAt(5, 5).Y; got < 223 {
		t.Fatalf("dark background should become white, got %d", got)
	}
}
