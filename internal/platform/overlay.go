package platform

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
)

type OverlayScene struct {
	Z      int           `json:"z,omitempty"`
	Space  string        `json:"space,omitempty"`
	Width  int           `json:"width,omitempty"`
	Height int           `json:"height,omitempty"`
	Items  []OverlayItem `json:"items"`
}

type OverlayPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type OverlayItem struct {
	Type      string         `json:"type"`
	X         float64        `json:"x,omitempty"`
	Y         float64        `json:"y,omitempty"`
	X2        float64        `json:"x2,omitempty"`
	Y2        float64        `json:"y2,omitempty"`
	W         float64        `json:"w,omitempty"`
	H         float64        `json:"h,omitempty"`
	R         float64        `json:"r,omitempty"`
	Points    []OverlayPoint `json:"points,omitempty"`
	Text      string         `json:"text,omitempty"`
	Color     string         `json:"color,omitempty"`
	Stroke    string         `json:"stroke,omitempty"`
	Fill      string         `json:"fill,omitempty"`
	LineWidth float64        `json:"lineWidth,omitempty"`
	FontSize  float64        `json:"fontSize,omitempty"`
	Align     string         `json:"align,omitempty"`
}

type OverlayInfo struct {
	Available       bool     `json:"available"`
	Driver          string   `json:"driver"`
	Visible         bool     `json:"visible"`
	Layers          int      `json:"layers"`
	Width           int      `json:"width,omitempty"`
	Height          int      `json:"height,omitempty"`
	SupportsAlpha   bool     `json:"supportsAlpha"`
	SupportsText    bool     `json:"supportsText"`
	SupportsShapes  []string `json:"supportsShapes"`
	CoordinateSpace []string `json:"coordinateSpaces"`
	Note            string   `json:"note,omitempty"`
}

type OverlayDriver interface {
	Name() string
	Available() bool
	SetLayer(layer string, scene OverlayScene) error
	ClearLayer(layer string) error
	ClearPrefix(prefix string) error
	ClearAll() error
	SetVisible(visible bool) error
	Info() OverlayInfo
}

type overlayState struct {
	mu      sync.RWMutex
	layers  map[string]OverlayScene
	visible bool
}

func newOverlayState() *overlayState {
	return &overlayState{layers: map[string]OverlayScene{}, visible: true}
}

func (s *overlayState) set(layer string, scene OverlayScene) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.layers[layer] = scene
}
func (s *overlayState) clear(layer string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.layers, layer)
}
func (s *overlayState) clearPrefix(prefix string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	for layer := range s.layers {
		if layer == prefix || strings.HasPrefix(layer, prefix+":") {
			delete(s.layers, layer)
		}
	}
}
func (s *overlayState) clearAll() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.layers = map[string]OverlayScene{}
}
func (s *overlayState) setVisible(v bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.visible = v
}
func (s *overlayState) snapshot() (map[string]OverlayScene, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	out := make(map[string]OverlayScene, len(s.layers))
	for k, v := range s.layers {
		out[k] = v
	}
	return out, s.visible
}

func (s *overlayState) orderedScenes() []OverlayScene {
	s.mu.RLock()
	defer s.mu.RUnlock()
	type entry struct {
		key   string
		scene OverlayScene
	}
	entries := make([]entry, 0, len(s.layers))
	for k, v := range s.layers {
		entries = append(entries, entry{k, v})
	}
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].scene.Z == entries[j].scene.Z {
			return entries[i].key < entries[j].key
		}
		return entries[i].scene.Z < entries[j].scene.Z
	})
	out := make([]OverlayScene, len(entries))
	for i := range entries {
		out[i] = entries[i].scene
	}
	return out
}

func (s *overlayState) count() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.layers)
}

func ValidateOverlayScene(scene *OverlayScene) error {
	if scene == nil {
		return fmt.Errorf("scene is required")
	}
	if scene.Space == "" {
		scene.Space = "normalized"
	}
	if scene.Space != "normalized" && scene.Space != "pixels" {
		return fmt.Errorf("scene.space must be normalized or pixels")
	}
	if scene.Space == "pixels" && ((scene.Width < 0) || (scene.Height < 0)) {
		return fmt.Errorf("scene width/height cannot be negative")
	}
	if len(scene.Items) > 2000 {
		return fmt.Errorf("scene contains %d items; maximum is 2000", len(scene.Items))
	}
	for i := range scene.Items {
		it := &scene.Items[i]
		it.Type = strings.ToLower(strings.TrimSpace(it.Type))
		switch it.Type {
		case "text", "line", "polyline", "polygon", "rect", "circle":
		default:
			return fmt.Errorf("item %d has unsupported type %q", i, it.Type)
		}
		if it.LineWidth <= 0 {
			it.LineWidth = 2
		}
		if it.LineWidth > 64 {
			return fmt.Errorf("item %d lineWidth is too large", i)
		}
		if it.FontSize <= 0 {
			it.FontSize = 18
		}
		if it.FontSize > 256 {
			return fmt.Errorf("item %d fontSize is too large", i)
		}
		if it.W < 0 || it.H < 0 || it.R < 0 {
			return fmt.Errorf("item %d width, height, and radius cannot be negative", i)
		}
		if it.Align == "" {
			it.Align = "left"
		}
		if it.Align != "left" && it.Align != "center" && it.Align != "right" {
			return fmt.Errorf("item %d align must be left, center, or right", i)
		}
		for _, c := range []string{it.Color, it.Stroke, it.Fill} {
			if c != "" {
				if _, _, _, _, err := ParseOverlayColor(c); err != nil {
					return fmt.Errorf("item %d: %w", i, err)
				}
			}
		}
		if (it.Type == "polyline" || it.Type == "polygon") && len(it.Points) > 10000 {
			return fmt.Errorf("item %d has too many points", i)
		}
	}
	return nil
}

func ParseOverlayColor(s string) (r, g, b, a uint8, err error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "none") || strings.EqualFold(s, "transparent") {
		return 0, 0, 0, 0, nil
	}
	if !strings.HasPrefix(s, "#") {
		return 0, 0, 0, 0, fmt.Errorf("overlay colors must use #RGB, #RRGGBB, or #RRGGBBAA")
	}
	hex := s[1:]
	if len(hex) == 3 {
		hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
	}
	if len(hex) != 6 && len(hex) != 8 {
		return 0, 0, 0, 0, fmt.Errorf("invalid overlay color %q", s)
	}
	v, parseErr := strconv.ParseUint(hex, 16, 32)
	if parseErr != nil {
		return 0, 0, 0, 0, fmt.Errorf("invalid overlay color %q", s)
	}
	if len(hex) == 6 {
		return uint8(v >> 16), uint8(v >> 8), uint8(v), 255, nil
	}
	return uint8(v >> 24), uint8(v >> 16), uint8(v >> 8), uint8(v), nil
}

func overlayCoord(v float64, size int, space string) int {
	if space == "normalized" {
		return int(math.Round(v * float64(size)))
	}
	return int(math.Round(v))
}

func overlayRadius(v float64, width, height int, space string) int {
	if space == "normalized" {
		m := width
		if height < m {
			m = height
		}
		return int(math.Round(v * float64(m)))
	}
	return int(math.Round(v))
}
