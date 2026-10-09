package platform

import "testing"

func TestCanonicalGameFrameValidity(t *testing.T) {
	f := GameFrame{
		Pix:          make([]byte, 4*3*2),
		Width:        3,
		Height:       2,
		Stride:       12,
		Format:       PixelFormatRGBA8,
		Driver:       "test",
		SourceWidth:  3,
		SourceHeight: 2,
		Verified:     true,
		Foreground:   true,
	}
	if !f.Valid() {
		t.Fatal("verified RGBA8 Elite frame should be valid")
	}
	f.Verified = false
	if f.Valid() {
		t.Fatal("non-blank frame without verification must be rejected")
	}
}

func TestBlankFrameEncodesWithoutPixels(t *testing.T) {
	f := blankGameFrame("test", 320, 180, "not-elite")
	if !f.Blanked || f.Reason != "not-elite" {
		t.Fatalf("unexpected blank frame: %#v", f)
	}
	b, info, err := EncodeGameFrameJPEG(f, 60)
	if err != nil || len(b) == 0 {
		t.Fatalf("blank frame did not encode: bytes=%d err=%v", len(b), err)
	}
	if !info.Blanked || !info.Canonical {
		t.Fatalf("blank frame metadata lost privacy state: %#v", info)
	}
}
