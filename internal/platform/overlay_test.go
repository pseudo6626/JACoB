package platform

import "testing"

func TestValidateOverlayScene(t *testing.T) {
	s := OverlayScene{Items: []OverlayItem{
		{Type: "text", X: .5, Y: .1, Text: "hello", Color: "#ff8b2c"},
		{Type: "polyline", Points: []OverlayPoint{{0, 0}, {1, 1}}, Stroke: "#ffffff80", LineWidth: 3},
		{Type: "circle", X: .5, Y: .5, R: .02, Fill: "#fff"},
	}}
	if err := ValidateOverlayScene(&s); err != nil {
		t.Fatal(err)
	}
	if s.Space != "normalized" {
		t.Fatalf("expected normalized default, got %q", s.Space)
	}
	if s.Items[0].FontSize != 18 {
		t.Fatalf("expected default font size")
	}
}
func TestValidateOverlaySceneRejectsUnknownShape(t *testing.T) {
	s := OverlayScene{Items: []OverlayItem{{Type: "bezier"}}}
	if err := ValidateOverlayScene(&s); err == nil {
		t.Fatal("expected error")
	}
}
func TestOverlayColor(t *testing.T) {
	r, g, b, a, err := ParseOverlayColor("#ff8b2c80")
	if err != nil {
		t.Fatal(err)
	}
	if r != 255 || g != 139 || b != 44 || a != 128 {
		t.Fatalf("unexpected rgba %d %d %d %d", r, g, b, a)
	}
}
