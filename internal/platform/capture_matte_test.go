//go:build windows && !nocapture

package platform

import (
	"image"
	"image/color"
	"testing"
)

func TestDrawScaledNearestLeavesMatteOutsideDestination(t *testing.T) {
	dst := image.NewRGBA(image.Rect(0, 0, 8, 6))
	black := color.RGBA{0, 0, 0, 255}
	for y := 0; y < 6; y++ {
		for x := 0; x < 8; x++ {
			dst.SetRGBA(x, y, black)
		}
	}
	src := image.NewRGBA(image.Rect(0, 0, 2, 2))
	src.SetRGBA(0, 0, color.RGBA{255, 0, 0, 255})
	src.SetRGBA(1, 0, color.RGBA{0, 255, 0, 255})
	src.SetRGBA(0, 1, color.RGBA{0, 0, 255, 255})
	src.SetRGBA(1, 1, color.RGBA{255, 255, 255, 255})

	drawScaledNearest(dst, src, 2, 1, 4, 4)

	if got := dst.RGBAAt(0, 0); got != black {
		t.Fatalf("matte pixel changed outside Elite destination: %#v", got)
	}
	if got := dst.RGBAAt(7, 5); got != black {
		t.Fatalf("matte pixel changed outside Elite destination: %#v", got)
	}
	if got := dst.RGBAAt(2, 1); got.R != 255 || got.G != 0 || got.B != 0 {
		t.Fatalf("scaled Elite image not drawn at expected origin: %#v", got)
	}
}
