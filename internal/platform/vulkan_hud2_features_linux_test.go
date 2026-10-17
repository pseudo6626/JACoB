//go:build linux && cgo

package platform

import (
	"math"
	"testing"
)

func TestVulkanHUD2FeatureSceneTessellates(t *testing.T) {
	scene := OverlayScene{
		Space: "pixels",
		Theme: "clean",
		Items: []OverlayItem{
			{Type: "text", X: 40, Y: 40, Text: "HUD2 123", Role: "primary", FontSize: 20, FontWeight: 700, Italic: true, Glow: 3, GlowColor: "#79B8FFAA", OutlineWidth: 1, OutlineColor: "#000000FF", ShadowX: 2, ShadowY: 2, ShadowColor: "#00000099"},
			{Type: "line", X: 20, Y: 90, X2: 260, Y2: 90, Role: "accent", LineWidth: 3, Dash: "dash", Glow: 2},
			{Type: "polyline", Points: []OverlayPoint{{X: 20, Y: 120}, {X: 80, Y: 105}, {X: 140, Y: 130}}, Role: "success", LineWidth: 2, Dash: "dot"},
			{Type: "polygon", Points: []OverlayPoint{{X: 190, Y: 105}, {X: 250, Y: 115}, {X: 220, Y: 155}}, Stroke: "#FFFFFFFF", Fill: "#FF8A2B66", LineWidth: 2},
			{Type: "rect", X: 20, Y: 180, W: 150, H: 70, Role: "panel"},
			{Type: "circle", X: 230, Y: 215, R: 30, Stroke: "#FFFFFFFF", Fill: "#79B8FF55", LineWidth: 2},
			{Type: "ring", X: 330, Y: 215, R: 26, Role: "warning", LineWidth: 3},
			{Type: "arc", X: 420, Y: 215, R: 30, StartAngle: -90, SweepAngle: 220, Role: "danger", LineWidth: 3},
			{Type: "reticle", X: 520, Y: 215, R: 16, Role: "accent", LineWidth: 2},
			{Type: "bracket", X: 590, Y: 180, W: 90, H: 70, Role: "success", LineWidth: 2},
			{Type: "chevron", X: 760, Y: 215, W: 55, H: 35, Role: "warning", LineWidth: 3},
			{Type: "panel", X: 20, Y: 300, W: 280, H: 90, Label: "PANEL"},
			{Type: "progress", X: 340, Y: 315, W: 260, H: 18, Label: "PROGRESS", Progress: 0.63, Role: "accent"},
			{Type: "metric", X: 690, Y: 310, Label: "SPEED", Value: "512", Unit: "M/S", FontSize: 30, Role: "metric"},
		},
	}
	if err := ValidateOverlayScene(&scene); err != nil {
		t.Fatalf("ValidateOverlayScene: %v", err)
	}
	if len(scene.Items) <= 14 {
		t.Fatalf("semantic HUD2 items were not expanded: got %d items", len(scene.Items))
	}
	var vertices []vulkanHUDVertex
	vulkanTessellateScene(&vertices, scene)
	if len(vertices) < 500 {
		t.Fatalf("expected substantial HUD2 geometry, got %d vertices", len(vertices))
	}
	if len(vertices)%3 != 0 {
		t.Fatalf("triangle stream length must be divisible by 3, got %d", len(vertices))
	}
	for i, v := range vertices {
		for _, n := range []float32{v.X, v.Y, v.R, v.G, v.B, v.A} {
			if math.IsNaN(float64(n)) || math.IsInf(float64(n), 0) {
				t.Fatalf("vertex %d contains non-finite value", i)
			}
		}
		if v.A < 0 || v.A > 1 {
			t.Fatalf("vertex %d alpha out of range: %f", i, v.A)
		}
	}
}
