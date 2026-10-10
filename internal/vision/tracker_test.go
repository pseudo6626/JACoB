package vision

import "testing"

func TestEstimateTranslation(t *testing.T) {
	w, h := 80, 60
	prev := make([]uint8, w*h)
	cur := make([]uint8, w*h)
	for y := 10; y < 50; y++ {
		for x := 10; x < 70; x++ {
			v := uint8((x*7 + y*11) % 251)
			prev[y*w+x] = v
			nx, ny := x+3, y-2
			if nx >= 0 && nx < w && ny >= 0 && ny < h {
				cur[ny*w+nx] = v
			}
		}
	}
	dx, dy, conf := estimateTranslation(prev, cur, w, h)
	if dx != 3 || dy != -2 {
		t.Fatalf("motion=(%v,%v), want (3,-2)", dx, dy)
	}
	if conf <= 0 {
		t.Fatalf("confidence=%v", conf)
	}
}

func TestFeatureSignatureStable(t *testing.T) {
	w, h := 32, 24
	gray := make([]uint8, w*h)
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			gray[y*w+x] = uint8((x*17 + y*29) % 251)
		}
	}
	a := featureSignature(gray, w, h, 16, 12)
	b := featureSignature(gray, w, h, 16, 12)
	if a != b {
		t.Fatalf("signature changed: %x != %x", a, b)
	}
}
