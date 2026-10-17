package core

import (
	"os"
	"runtime"
	"strings"

	"jacob/internal/platform"
)

func steamDeckRuntime() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	if strings.EqualFold(strings.TrimSpace(os.Getenv("JACOB_STEAM_DECK")), "1") {
		return true
	}
	if b, err := os.ReadFile("/etc/os-release"); err == nil {
		t := strings.ToLower(string(b))
		if strings.Contains(t, "steamos") || strings.Contains(t, "steam deck") {
			return true
		}
	}
	return false
}

func gamingModeRuntime() bool {
	if runtime.GOOS != "linux" {
		return false
	}
	for _, key := range []string{"SteamGameId", "SteamAppId", "STEAM_GAME", "JACOB_STEAM_APP_ID"} {
		if strings.TrimSpace(os.Getenv(key)) == "359320" {
			return true
		}
	}
	return strings.TrimSpace(os.Getenv("GAMESCOPE_WAYLAND_DISPLAY")) != ""
}

func (s *Server) performanceProfile() map[string]any {
	deck := steamDeckRuntime()
	recommended := map[string]any{
		"spatialHz":     8,
		"gameViewFps":   8,
		"visionColorHz": 6,
		"visionOcrHz":   3,
		"hudHz":         15,
	}
	tier := "desktop"
	if deck {
		tier = "handheld"
		recommended = map[string]any{
			"spatialHz":     5,
			"gameViewFps":   5,
			"visionColorHz": 4,
			"visionOcrHz":   2,
			"hudHz":         10,
		}
	}
	return map[string]any{
		"platform":    runtime.GOOS,
		"arch":        runtime.GOARCH,
		"steamDeck":   deck,
		"gamingMode":  gamingModeRuntime(),
		"tier":        tier,
		"recommended": recommended,
	}
}

func (s *Server) capabilities() map[string]any {
	foregroundVerified := false
	focusReason := ""
	if s.input != nil && s.input.Available() {
		if err := s.input.FocusGame(); err == nil {
			foregroundVerified = true
		} else {
			focusReason = err.Error()
		}
	}

	input := map[string]any{
		"available": s.input != nil && s.input.Available(),
		"driver": func() string {
			if s.input != nil {
				return s.input.Name()
			}
			return "input-unavailable"
		}(),
		"foregroundVerified": foregroundVerified,
		"focusReason":        focusReason,
		"heldKeys":           0,
		"heldActions":        []string{},
	}
	if s.controls != nil {
		input["heldKeys"] = s.controls.HeldCount()
		input["heldActions"] = s.controls.HeldActions()
	}

	capture := map[string]any{
		"available": s.capture != nil && s.capture.Available(),
		"driver": func() string {
			if s.capture != nil {
				return s.capture.Name()
			}
			return "capture-unavailable"
		}(),
		"eliteOnly":       true,
		"desktopFallback": false,
	}
	if stats, ok := s.capture.(platform.CaptureStatsProvider); ok {
		capture["stats"] = stats.CaptureStats()
	}

	visionInfo := map[string]any{"available": false}
	if s.vision != nil {
		visionInfo = s.vision.Info()
	}
	spatialInfo := map[string]any{"available": false}
	if s.spatial != nil {
		spatialInfo = s.spatial.Info()
	}
	overlayInfo := any(platform.OverlayInfo{Available: false, Driver: "overlay-unavailable"})
	if s.overlay != nil {
		overlayInfo = s.overlay.Info()
	}
	recorderInfo := any(platform.RecorderStatus{Available: false, Driver: "recorder-unavailable"})
	if s.recorder != nil {
		recorderInfo = s.recorder.Status()
	}

	return map[string]any{
		"input": input,
		"clipboard": func() any {
			if s.clipboard != nil {
				return s.clipboard.Info()
			}
			return map[string]any{"available": false, "writeOnly": true, "hostOnly": true}
		}(),
		"capture":     capture,
		"vision":      visionInfo,
		"spatial":     spatialInfo,
		"overlay":     overlayInfo,
		"recorder":    recorderInfo,
		"performance": s.performanceProfile(),
	}
}
