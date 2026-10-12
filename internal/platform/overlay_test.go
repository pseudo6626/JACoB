package platform

import (
	"math"
	"testing"
)

func TestParseOverlayColorFormats(t *testing.T) {
	tests := []struct {
		in         string
		r, g, b, a uint8
	}{
		{"#fff", 255, 255, 255, 255},
		{"#f80c", 255, 136, 0, 204},
		{"#123456", 0x12, 0x34, 0x56, 255},
		{"#12345678", 0x12, 0x34, 0x56, 0x78},
	}
	for _, tt := range tests {
		r, g, b, a, err := ParseOverlayColor(tt.in)
		if err != nil {
			t.Fatalf("ParseOverlayColor(%q): %v", tt.in, err)
		}
		if r != tt.r || g != tt.g || b != tt.b || a != tt.a {
			t.Fatalf("ParseOverlayColor(%q) = %02x %02x %02x %02x", tt.in, r, g, b, a)
		}
	}
}

func TestLegacySceneStaysCompatible(t *testing.T) {
	s := OverlayScene{Items: []OverlayItem{{Type: "text", X: .5, Y: .2, Text: "hello"}}}
	if err := ValidateOverlayScene(&s); err != nil {
		t.Fatal(err)
	}
	if s.Theme != "legacy" || s.Space != "normalized" {
		t.Fatalf("unexpected defaults: %+v", s)
	}
	if got := s.Items[0]; got.Type != "text" || got.Color != "#FFFFFFFF" || got.FontSize != 18 || got.Align != "left" {
		t.Fatalf("legacy item changed unexpectedly: %+v", got)
	}
}

func TestCockpitMetricExpands(t *testing.T) {
	s := OverlayScene{Theme: "cockpit", Items: []OverlayItem{{Type: "metric", X: .9, Y: .1, Label: "dist", Value: "1.42", Unit: "km", Align: "right", Glow: 2}}}
	if err := ValidateOverlayScene(&s); err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 3 {
		t.Fatalf("metric expanded to %d items", len(s.Items))
	}
	if s.Items[0].Text != "DIST" || s.Items[1].Text != "1.42" || s.Items[2].Text != "km" {
		t.Fatalf("unexpected metric expansion: %+v", s.Items)
	}
	if s.Items[1].FontFamily != "Bahnschrift" || s.Items[1].Color == "" {
		t.Fatalf("cockpit defaults not applied: %+v", s.Items[1])
	}
}

func TestArcExpandsToPolyline(t *testing.T) {
	s := OverlayScene{Items: []OverlayItem{{Type: "arc", X: .5, Y: .5, R: .1, StartAngle: 0, SweepAngle: 90}}}
	if err := ValidateOverlayScene(&s); err != nil {
		t.Fatal(err)
	}
	if len(s.Items) != 1 || s.Items[0].Type != "polyline" || len(s.Items[0].Points) < 9 {
		t.Fatalf("bad arc expansion: %+v", s.Items)
	}
	first, last := s.Items[0].Points[0], s.Items[0].Points[len(s.Items[0].Points)-1]
	if math.Abs(first.X-.6) > 1e-9 || math.Abs(first.Y-.5) > 1e-9 || math.Abs(last.X-.5) > 1e-9 || math.Abs(last.Y-.6) > 1e-9 {
		t.Fatalf("bad arc endpoints: first=%+v last=%+v", first, last)
	}
}

func TestInvalidThemeAndOpacity(t *testing.T) {
	for _, s := range []OverlayScene{
		{Theme: "nope", Items: []OverlayItem{{Type: "text", Text: "x"}}},
		{Opacity: 1.1, Items: []OverlayItem{{Type: "text", Text: "x"}}},
		{Items: []OverlayItem{{Type: "text", Text: "x", Opacity: 1.1}}},
	} {
		if err := ValidateOverlayScene(&s); err == nil {
			t.Fatalf("expected validation failure for %+v", s)
		}
	}
}

func TestHUD2Info(t *testing.T) {
	i := HUD2Info(OverlayInfo{Available: true})
	if i.HUDVersion != 2 || len(i.Themes) < 4 || len(i.SemanticTypes) < 6 || len(i.Effects) < 6 {
		t.Fatalf("incomplete HUD2 capability info: %+v", i)
	}
}
