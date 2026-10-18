//go:build linux

package platform

import (
	"os"
	"strings"
	"testing"
)

func TestVulkanVertexShaderUsesTopLeftHUDCoordinates(t *testing.T) {
	b, err := os.ReadFile("../../native/vulkan-hud/overlay.vert")
	if err != nil {
		t.Fatalf("read Vulkan HUD vertex shader: %v", err)
	}
	shader := string(b)

	want := "vec2 ndc = vec2(inPos.x * 2.0 - 1.0, inPos.y * 2.0 - 1.0);"
	if !strings.Contains(shader, want) {
		t.Fatalf("Vulkan HUD vertex shader does not map JACoB top-left coordinates correctly")
	}

	if strings.Contains(shader, "1.0 - inPos.y * 2.0") {
		t.Fatalf("Vulkan HUD vertex shader contains the vertically inverted Y transform")
	}
}
