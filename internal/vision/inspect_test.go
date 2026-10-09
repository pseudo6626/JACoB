package vision

import (
	"image"
	"image/color"
	"testing"
)

func TestValidateInspectAllowsTinyEffectiveSubregion(t *testing.T) {
	sub := Region{X: 0, Y: 0, Width: 0.05, Height: 0.05}
	err := validateInspect(InspectRequest{
		Region: Region{X: 0.1, Y: 0.1, Width: 0.1, Height: 0.1},
		Operations: map[string]Operation{
			"tiny": {Type: "colorPresent", Color: "#ff8000", Subregion: &sub},
		},
	})
	if err != nil {
		t.Fatalf("expected tiny normalized subregion to validate: %v", err)
	}
}

func TestValidateInspectAllowsReasonableSubregion(t *testing.T) {
	sub := Region{X: 0.05, Y: 0.05, Width: 0.3, Height: 0.4}
	err := validateInspect(InspectRequest{
		Region: Region{X: 0.7, Y: 0.7, Width: 0.2, Height: 0.2},
		Operations: map[string]Operation{
			"ok": {Type: "colorPresent", Color: "#ff8000", Subregion: &sub},
		},
	})
	if err != nil {
		t.Fatalf("expected subregion to validate: %v", err)
	}
}

func TestColorVerticalFill(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 100))
	orange := color.RGBA{R: 200, G: 90, B: 25, A: 255}
	for y := 25; y < 100; y++ {
		for x := 4; x < 16; x++ {
			img.SetRGBA(x, y, orange)
		}
	}
	fill, _ := regionColorVerticalFill(img, orange, 5, 0.1)
	if fill < 0.74 || fill > 0.77 {
		t.Fatalf("expected about 75%% fill, got %.4f", fill)
	}
}

func TestColorVerticalFillEmpty(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20, 100))
	orange := color.RGBA{R: 200, G: 90, B: 25, A: 255}
	fill, _ := regionColorVerticalFill(img, orange, 5, 0.1)
	if fill != 0 {
		t.Fatalf("expected 0 fill, got %.4f", fill)
	}
}

func TestValidateTextLinesPreprocess(t *testing.T) {
	err := validateInspect(InspectRequest{
		Region: Region{X: 0.2, Y: 0.2, Width: 0.5, Height: 0.4},
		Operations: map[string]Operation{
			"rows": {Type: "textLines", Preprocess: &TextPreprocess{Scale: 2, Grayscale: true, AutoContrast: true}},
		},
	})
	if err != nil {
		t.Fatalf("expected textLines preprocessing to validate: %v", err)
	}
}

func TestPreprocessScaleAndContrast(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 8; x++ {
			v := uint8(50 + x*10)
			img.SetRGBA(x, y, color.RGBA{R: v, G: v, B: v, A: 255})
		}
	}
	out, err := preprocessText(img, &TextPreprocess{Scale: 2, Grayscale: true, AutoContrast: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Bounds().Dx() != 16 || out.Bounds().Dy() != 8 {
		t.Fatalf("expected 16x8 preprocessed image, got %dx%d", out.Bounds().Dx(), out.Bounds().Dy())
	}
}

func TestValidateColorVerticalFillFullThreshold(t *testing.T) {
	sub := Region{X: 0.15, Y: 0.1, Width: 0.2, Height: 0.5}
	err := validateInspect(InspectRequest{
		Region: Region{X: 0.75, Y: 0.72, Width: 0.12, Height: 0.2},
		Operations: map[string]Operation{
			"sysFill": {Type: "colorVerticalFill", Color: "#c85a19", MinimumCoverage: 0.06, FullThreshold: 0.92, Subregion: &sub},
		},
	})
	if err != nil {
		t.Fatalf("expected full-sized gauge with fullThreshold to validate: %v", err)
	}
}

func TestValidateRejectsFullThresholdOnOtherOperation(t *testing.T) {
	err := validateInspect(InspectRequest{
		Region: Region{X: 0.1, Y: 0.1, Width: 0.3, Height: 0.3},
		Operations: map[string]Operation{
			"bad": {Type: "colorPresent", Color: "#c85a19", FullThreshold: 0.9},
		},
	})
	if err == nil {
		t.Fatal("expected fullThreshold on colorPresent to be rejected")
	}
}
