package platform

import (
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"sync"
)

const HUDVersion = 2

var overlayThemes = map[string]overlayTheme{
	"legacy": {
		Font: "Segoe UI", Primary: "#FFFFFFFF", Secondary: "#C9D1D9FF", Muted: "#8B949EFF",
		Accent: "#FF8A2BFF", Success: "#78D7A9FF", Warning: "#F2C66DFF", Danger: "#FF6961FF",
		Panel: "#081017C0", Outline: "#FFFFFFFF",
	},
	"cockpit": {
		Font: "Bahnschrift", Primary: "#F6F2E8EB", Secondary: "#B8C1C7D9", Muted: "#7E8B92A8",
		Accent: "#FF8A2BEF", Success: "#7BD7A9EB", Warning: "#F2C66DEB", Danger: "#FF6961F2",
		Panel: "#071016B8", Outline: "#FF8A2B7A",
	},
	"clean": {
		Font: "Segoe UI", Primary: "#F4F7FAEE", Secondary: "#C5CDD5D9", Muted: "#8B98A4B0",
		Accent: "#79B8FFEE", Success: "#7BD7A9EE", Warning: "#F2C66DEE", Danger: "#FF7373F2",
		Panel: "#091018B8", Outline: "#A7B4C088",
	},
	"highcontrast": {
		Font: "Segoe UI", Primary: "#FFFFFFFF", Secondary: "#FFFFFFFF", Muted: "#D8D8D8FF",
		Accent: "#FFB000FF", Success: "#65FF9AFF", Warning: "#FFE066FF", Danger: "#FF5C5CFF",
		Panel: "#000000E8", Outline: "#FFFFFFFF",
	},
}

type overlayTheme struct {
	Font      string
	Primary   string
	Secondary string
	Muted     string
	Accent    string
	Success   string
	Warning   string
	Danger    string
	Panel     string
	Outline   string
}

type OverlayScene struct {
	Z       int           `json:"z,omitempty"`
	Space   string        `json:"space,omitempty"`
	Width   int           `json:"width,omitempty"`
	Height  int           `json:"height,omitempty"`
	Theme   string        `json:"theme,omitempty"`
	Opacity float64       `json:"opacity,omitempty"`
	Scale   float64       `json:"scale,omitempty"`
	Items   []OverlayItem `json:"items"`
}

type OverlayPoint struct {
	X float64 `json:"x"`
	Y float64 `json:"y"`
}

type OverlayItem struct {
	Type         string         `json:"type"`
	X            float64        `json:"x,omitempty"`
	Y            float64        `json:"y,omitempty"`
	X2           float64        `json:"x2,omitempty"`
	Y2           float64        `json:"y2,omitempty"`
	W            float64        `json:"w,omitempty"`
	H            float64        `json:"h,omitempty"`
	R            float64        `json:"r,omitempty"`
	Points       []OverlayPoint `json:"points,omitempty"`
	Text         string         `json:"text,omitempty"`
	Label        string         `json:"label,omitempty"`
	Value        string         `json:"value,omitempty"`
	Unit         string         `json:"unit,omitempty"`
	Color        string         `json:"color,omitempty"`
	Stroke       string         `json:"stroke,omitempty"`
	Fill         string         `json:"fill,omitempty"`
	Role         string         `json:"role,omitempty"`
	LineWidth    float64        `json:"lineWidth,omitempty"`
	FontSize     float64        `json:"fontSize,omitempty"`
	Size         float64        `json:"size,omitempty"`
	FontFamily   string         `json:"fontFamily,omitempty"`
	FontWeight   int            `json:"fontWeight,omitempty"`
	Italic       bool           `json:"italic,omitempty"`
	Align        string         `json:"align,omitempty"`
	Opacity      float64        `json:"opacity,omitempty"`
	Glow         float64        `json:"glow,omitempty"`
	GlowColor    string         `json:"glowColor,omitempty"`
	OutlineWidth float64        `json:"outlineWidth,omitempty"`
	OutlineColor string         `json:"outlineColor,omitempty"`
	ShadowX      float64        `json:"shadowX,omitempty"`
	ShadowY      float64        `json:"shadowY,omitempty"`
	ShadowColor  string         `json:"shadowColor,omitempty"`
	CornerRadius float64        `json:"cornerRadius,omitempty"`
	Dash         string         `json:"dash,omitempty"`
	Progress     float64        `json:"progress,omitempty"`
	Min          float64        `json:"min,omitempty"`
	Max          float64        `json:"max,omitempty"`
	StartAngle   float64        `json:"startAngle,omitempty"`
	SweepAngle   float64        `json:"sweepAngle,omitempty"`
}

type OverlayInfo struct {
	Available           bool     `json:"available"`
	Driver              string   `json:"driver"`
	Visible             bool     `json:"visible"`
	Layers              int      `json:"layers"`
	Width               int      `json:"width,omitempty"`
	Height              int      `json:"height,omitempty"`
	SupportsAlpha       bool     `json:"supportsAlpha"`
	SupportsText        bool     `json:"supportsText"`
	SupportsShapes      []string `json:"supportsShapes"`
	CoordinateSpace     []string `json:"coordinateSpaces"`
	HUDVersion          int      `json:"hudVersion,omitempty"`
	Themes              []string `json:"themes,omitempty"`
	SemanticTypes       []string `json:"semanticTypes,omitempty"`
	Effects             []string `json:"effects,omitempty"`
	TargetDisplay       string   `json:"targetDisplay,omitempty"`
	SurfaceCreated      bool     `json:"surfaceCreated,omitempty"`
	Mapped              bool     `json:"mapped,omitempty"`
	CompositionVerified bool     `json:"compositionVerified,omitempty"`
	Note                string   `json:"note,omitempty"`
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

func HUD2Info(info OverlayInfo) OverlayInfo {
	info.HUDVersion = HUDVersion
	info.Themes = []string{"legacy", "cockpit", "clean", "highcontrast"}
	info.SemanticTypes = []string{"metric", "panel", "progress", "reticle", "bracket", "chevron", "arc", "ring"}
	info.Effects = []string{"opacity", "glow", "outline", "shadow", "fontWeight", "italic", "dash", "cornerRadius"}
	return info
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
	if scene.Space == "pixels" && (scene.Width < 0 || scene.Height < 0) {
		return fmt.Errorf("scene width/height cannot be negative")
	}
	if scene.Theme == "" {
		scene.Theme = "legacy"
	}
	scene.Theme = normalizeThemeName(scene.Theme)
	if _, ok := overlayThemes[scene.Theme]; !ok {
		return fmt.Errorf("unknown overlay theme %q", scene.Theme)
	}
	if scene.Opacity <= 0 {
		scene.Opacity = 1
	}
	if scene.Opacity > 1 {
		return fmt.Errorf("scene.opacity must be between 0 and 1")
	}
	if scene.Scale <= 0 {
		scene.Scale = 1
	}
	if scene.Scale > 4 {
		return fmt.Errorf("scene.scale is too large")
	}
	if len(scene.Items) > 2000 {
		return fmt.Errorf("scene contains %d items; maximum is 2000", len(scene.Items))
	}

	expanded := make([]OverlayItem, 0, len(scene.Items)*2)
	for i := range scene.Items {
		items, err := expandOverlayItem(*scene, scene.Items[i])
		if err != nil {
			return fmt.Errorf("item %d: %w", i, err)
		}
		expanded = append(expanded, items...)
		if len(expanded) > 4000 {
			return fmt.Errorf("expanded scene contains too many items")
		}
	}
	scene.Items = expanded

	theme := overlayThemes[scene.Theme]
	for i := range scene.Items {
		it := &scene.Items[i]
		it.Type = strings.ToLower(strings.TrimSpace(it.Type))
		switch it.Type {
		case "text", "line", "polyline", "polygon", "rect", "circle":
		default:
			return fmt.Errorf("item %d has unsupported type %q", i, it.Type)
		}
		applyOverlayRole(it, theme)
		if it.Size > 0 && it.FontSize <= 0 {
			it.FontSize = it.Size
		}
		if it.LineWidth <= 0 {
			if scene.Theme == "legacy" {
				it.LineWidth = 2
			} else {
				it.LineWidth = 1.5
			}
		}
		it.LineWidth *= scene.Scale
		if it.LineWidth > 64 {
			return fmt.Errorf("item %d lineWidth is too large", i)
		}
		if it.FontSize <= 0 {
			it.FontSize = 18
		}
		it.FontSize *= scene.Scale
		if it.FontSize > 256 {
			return fmt.Errorf("item %d fontSize is too large", i)
		}
		if it.FontFamily == "" {
			it.FontFamily = theme.Font
		}
		if it.FontWeight == 0 {
			if strings.EqualFold(it.Role, "metric") {
				it.FontWeight = 600
			} else {
				it.FontWeight = 400
			}
		}
		if it.FontWeight < 100 || it.FontWeight > 900 {
			return fmt.Errorf("item %d fontWeight must be 100..900", i)
		}
		if it.W < 0 || it.H < 0 || it.R < 0 || it.CornerRadius < 0 || it.Glow < 0 || it.OutlineWidth < 0 {
			return fmt.Errorf("item %d dimensions/effects cannot be negative", i)
		}
		if it.Glow > 24 || it.OutlineWidth > 8 || math.Abs(it.ShadowX) > 128 || math.Abs(it.ShadowY) > 128 {
			return fmt.Errorf("item %d effect size is too large", i)
		}
		if it.Align == "" {
			it.Align = "left"
		}
		if it.Align != "left" && it.Align != "center" && it.Align != "right" {
			return fmt.Errorf("item %d align must be left, center, or right", i)
		}
		if it.Opacity <= 0 {
			it.Opacity = 1
		}
		if it.Opacity > 1 {
			return fmt.Errorf("item %d opacity must be between 0 and 1", i)
		}
		it.Opacity *= scene.Opacity
		it.Dash = strings.ToLower(strings.TrimSpace(it.Dash))
		if it.Dash != "" && it.Dash != "solid" && it.Dash != "dash" && it.Dash != "dot" && it.Dash != "dashdot" {
			return fmt.Errorf("item %d dash must be solid, dash, dot, or dashdot", i)
		}
		for _, c := range []string{it.Color, it.Stroke, it.Fill, it.GlowColor, it.OutlineColor, it.ShadowColor} {
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

func normalizeThemeName(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = strings.ReplaceAll(s, "-", "")
	s = strings.ReplaceAll(s, "_", "")
	if s == "highcontrast" || s == "hc" {
		return "highcontrast"
	}
	return s
}

func applyOverlayRole(it *OverlayItem, t overlayTheme) {
	role := strings.ToLower(strings.TrimSpace(it.Role))
	if role == "" {
		if it.Type == "text" {
			role = "primary"
		} else {
			return
		}
	}
	pick := func() string {
		switch role {
		case "primary", "metric", "label", "heading":
			return t.Primary
		case "secondary":
			return t.Secondary
		case "muted":
			return t.Muted
		case "accent":
			return t.Accent
		case "success":
			return t.Success
		case "warning":
			return t.Warning
		case "danger":
			return t.Danger
		case "panel":
			return t.Panel
		default:
			return t.Primary
		}
	}()
	if it.Type == "text" {
		if it.Color == "" {
			it.Color = pick
		}
		if it.OutlineColor == "" && it.OutlineWidth > 0 {
			it.OutlineColor = t.Panel
		}
		if it.GlowColor == "" && it.Glow > 0 {
			it.GlowColor = pick
		}
		return
	}
	if it.Type == "rect" && role == "panel" {
		if it.Fill == "" {
			it.Fill = t.Panel
		}
		if it.Stroke == "" {
			it.Stroke = t.Outline
		}
		return
	}
	if it.Stroke == "" && it.Color == "" {
		it.Stroke = pick
	}
	if it.GlowColor == "" && it.Glow > 0 {
		it.GlowColor = pick
	}
}

func expandOverlayItem(scene OverlayScene, it OverlayItem) ([]OverlayItem, error) {
	kind := strings.ToLower(strings.TrimSpace(it.Type))
	if kind == "" {
		return nil, fmt.Errorf("type is required")
	}
	it.Type = kind
	switch kind {
	case "text", "line", "polyline", "polygon", "rect", "circle":
		return []OverlayItem{it}, nil
	case "ring":
		it.Type = "circle"
		return []OverlayItem{it}, nil
	case "arc":
		if it.R <= 0 {
			return nil, fmt.Errorf("arc radius must be positive")
		}
		sweep := it.SweepAngle
		if sweep == 0 {
			sweep = 360
		}
		steps := int(math.Ceil(math.Abs(sweep) / 6))
		if steps < 8 {
			steps = 8
		}
		if steps > 128 {
			steps = 128
		}
		pts := make([]OverlayPoint, steps+1)
		for i := 0; i <= steps; i++ {
			a := (it.StartAngle + sweep*float64(i)/float64(steps)) * math.Pi / 180
			pts[i] = OverlayPoint{X: it.X + math.Cos(a)*it.R, Y: it.Y + math.Sin(a)*it.R}
		}
		it.Type = "polyline"
		it.Points = pts
		return []OverlayItem{it}, nil
	case "chevron":
		w, h := it.W, it.H
		if w == 0 {
			w = sceneX(scene, 28)
		}
		if h == 0 {
			h = sceneY(scene, 18)
		}
		it.Type = "polyline"
		it.Points = []OverlayPoint{{X: it.X - w/2, Y: it.Y - h/2}, {X: it.X, Y: it.Y + h/2}, {X: it.X + w/2, Y: it.Y - h/2}}
		return []OverlayItem{it}, nil
	case "reticle":
		r := it.R
		if r == 0 {
			r = minFloat(sceneX(scene, 22), sceneY(scene, 22))
		}
		gap := r * 0.45
		outer := r * 1.65
		base := it
		base.Type = "line"
		items := []OverlayItem{
			{Type: "circle", X: it.X, Y: it.Y, R: r, Role: it.Role, Stroke: it.Stroke, Color: it.Color, LineWidth: it.LineWidth, Opacity: it.Opacity, Glow: it.Glow, GlowColor: it.GlowColor},
			base, base, base, base,
		}
		items[1].X, items[1].Y, items[1].X2, items[1].Y2 = it.X-outer, it.Y, it.X-gap, it.Y
		items[2].X, items[2].Y, items[2].X2, items[2].Y2 = it.X+gap, it.Y, it.X+outer, it.Y
		items[3].X, items[3].Y, items[3].X2, items[3].Y2 = it.X, it.Y-outer, it.X, it.Y-gap
		items[4].X, items[4].Y, items[4].X2, items[4].Y2 = it.X, it.Y+gap, it.X, it.Y+outer
		return items, nil
	case "bracket":
		if it.W <= 0 || it.H <= 0 {
			return nil, fmt.Errorf("bracket width and height must be positive")
		}
		segX, segY := it.W*0.22, it.H*0.22
		l, r, top, bot := it.X, it.X+it.W, it.Y, it.Y+it.H
		line := func(x, y, x2, y2 float64) OverlayItem {
			return OverlayItem{Type: "line", X: x, Y: y, X2: x2, Y2: y2, Role: it.Role, Color: it.Color, Stroke: it.Stroke, LineWidth: it.LineWidth, Opacity: it.Opacity, Glow: it.Glow, GlowColor: it.GlowColor, Dash: it.Dash}
		}
		return []OverlayItem{
			line(l, top, l+segX, top), line(l, top, l, top+segY), line(r, top, r-segX, top), line(r, top, r, top+segY),
			line(l, bot, l+segX, bot), line(l, bot, l, bot-segY), line(r, bot, r-segX, bot), line(r, bot, r, bot-segY),
		}, nil
	case "panel":
		if it.W <= 0 || it.H <= 0 {
			return nil, fmt.Errorf("panel width and height must be positive")
		}
		panel := it
		panel.Type = "rect"
		if panel.Role == "" {
			panel.Role = "panel"
		}
		items := []OverlayItem{panel}
		if it.Label != "" {
			items = append(items, OverlayItem{Type: "text", X: it.X + sceneX(scene, 12), Y: it.Y + sceneY(scene, 9), Text: it.Label, Role: "secondary", FontSize: maxFloat(11, it.FontSize*0.65), FontFamily: it.FontFamily, FontWeight: 600, Opacity: it.Opacity})
		}
		return items, nil
	case "progress":
		if it.W <= 0 || it.H <= 0 {
			return nil, fmt.Errorf("progress width and height must be positive")
		}
		p := it.Progress
		if it.Max > it.Min {
			p = (it.Progress - it.Min) / (it.Max - it.Min)
		}
		p = math.Max(0, math.Min(1, p))
		bg := it
		bg.Type = "rect"
		bg.Role = "muted"
		bg.Fill = "#FFFFFF18"
		bg.Stroke = "#FFFFFF35"
		fg := it
		fg.Type = "rect"
		fg.W = it.W * p
		fg.Stroke = "transparent"
		if fg.Fill == "" {
			fg.Fill = fg.Color
		}
		if fg.Fill == "" {
			fg.Role = firstNonEmpty(it.Role, "accent")
		}
		items := []OverlayItem{bg, fg}
		if it.Label != "" {
			items = append(items, OverlayItem{Type: "text", X: it.X, Y: it.Y - sceneY(scene, 18), Text: it.Label, Role: "secondary", FontSize: maxFloat(11, it.FontSize*0.65), FontFamily: it.FontFamily, FontWeight: 500, Opacity: it.Opacity})
		}
		return items, nil
	case "metric":
		fs := it.FontSize
		if fs <= 0 {
			fs = 24
		}
		role := firstNonEmpty(it.Role, "metric")
		items := make([]OverlayItem, 0, 3)
		if it.Label != "" {
			items = append(items, OverlayItem{Type: "text", X: it.X, Y: it.Y, Text: strings.ToUpper(it.Label), Role: "secondary", FontSize: maxFloat(10, fs*0.5), FontFamily: it.FontFamily, FontWeight: 600, Align: it.Align, Opacity: it.Opacity})
		}
		valueY := it.Y
		if it.Label != "" {
			valueY += sceneY(scene, fs*0.72)
		}
		value := it.Value
		if value == "" {
			value = it.Text
		}
		items = append(items, OverlayItem{Type: "text", X: it.X, Y: valueY, Text: value, Role: role, Color: it.Color, FontSize: fs, FontFamily: it.FontFamily, FontWeight: firstInt(it.FontWeight, 600), Italic: it.Italic, Align: it.Align, Opacity: it.Opacity, Glow: it.Glow, GlowColor: it.GlowColor, OutlineWidth: it.OutlineWidth, OutlineColor: it.OutlineColor, ShadowX: it.ShadowX, ShadowY: it.ShadowY, ShadowColor: it.ShadowColor})
		if it.Unit != "" {
			off := sceneX(scene, fs*0.62*float64(maxInt(1, len([]rune(value)))))
			if it.Align == "right" {
				off = -sceneX(scene, fs*0.3)
			}
			items = append(items, OverlayItem{Type: "text", X: it.X + off, Y: valueY + sceneY(scene, fs*0.28), Text: it.Unit, Role: "secondary", FontSize: maxFloat(10, fs*0.48), FontFamily: it.FontFamily, FontWeight: 500, Align: it.Align, Opacity: it.Opacity})
		}
		return items, nil
	default:
		return nil, fmt.Errorf("unsupported type %q", kind)
	}
}

func sceneX(scene OverlayScene, px float64) float64 {
	if scene.Space == "pixels" {
		return px
	}
	return px / 1920
}

func sceneY(scene OverlayScene, px float64) float64 {
	if scene.Space == "pixels" {
		return px
	}
	return px / 1080
}

func ParseOverlayColor(s string) (r, g, b, a uint8, err error) {
	s = strings.TrimSpace(s)
	if s == "" || strings.EqualFold(s, "none") || strings.EqualFold(s, "transparent") {
		return 0, 0, 0, 0, nil
	}
	if !strings.HasPrefix(s, "#") {
		return 0, 0, 0, 0, fmt.Errorf("overlay colors must use #RGB, #RGBA, #RRGGBB, or #RRGGBBAA")
	}
	hex := s[1:]
	if len(hex) == 3 || len(hex) == 4 {
		var b strings.Builder
		b.Grow(len(hex) * 2)
		for i := range hex {
			b.WriteByte(hex[i])
			b.WriteByte(hex[i])
		}
		hex = b.String()
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

func overlayColorWithOpacity(s string, opacity float64) (r, g, b, a uint8) {
	r, g, b, a, _ = ParseOverlayColor(s)
	if opacity < 0 {
		opacity = 0
	}
	if opacity > 1 {
		opacity = 1
	}
	a = uint8(math.Round(float64(a) * opacity))
	return
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

func firstNonEmpty(v, fallback string) string {
	if strings.TrimSpace(v) != "" {
		return v
	}
	return fallback
}
func firstInt(v, fallback int) int {
	if v != 0 {
		return v
	}
	return fallback
}
func minFloat(a, b float64) float64 {
	if a < b {
		return a
	}
	return b
}
func maxFloat(a, b float64) float64 {
	if a > b {
		return a
	}
	return b
}
func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
