//go:build linux && cgo

package platform

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode"
)

const (
	vulkanHUDMagic       uint32 = 0x3244484a // JHD2
	vulkanHUDVersion     uint32 = 2
	vulkanHUDCanvasW            = 1280
	vulkanHUDCanvasH            = 800
	vulkanHUDMaxVertices        = 1_000_000
)

type vulkanHUDVertex struct {
	X float32
	Y float32
	R float32
	G float32
	B float32
	A float32
}

type vulkanOverlay struct {
	state      *overlayState
	mu         sync.Mutex
	generation uint64
	lastErr    string
}

func newVulkanOverlayDriver() OverlayDriver {
	o := &vulkanOverlay{state: newOverlayState()}
	_ = o.publish()
	return o
}

func (o *vulkanOverlay) Name() string    { return "linux-vulkan-hud2-v10" }
func (o *vulkanOverlay) Available() bool { return true }

func (o *vulkanOverlay) SetLayer(layer string, scene OverlayScene) error {
	if layer == "" {
		return fmt.Errorf("overlay layer is required")
	}
	if err := ValidateOverlayScene(&scene); err != nil {
		return err
	}
	o.state.set(layer, scene)
	return o.publish()
}

func (o *vulkanOverlay) ClearLayer(layer string) error {
	o.state.clear(layer)
	return o.publish()
}

func (o *vulkanOverlay) ClearPrefix(prefix string) error {
	o.state.clearPrefix(prefix)
	return o.publish()
}

func (o *vulkanOverlay) ClearAll() error {
	o.state.clearAll()
	return o.publish()
}

func (o *vulkanOverlay) SetVisible(v bool) error {
	o.state.setVisible(v)
	return o.publish()
}

func vulkanRuntimeDir() string {
	if v := strings.TrimSpace(os.Getenv("JACOB_VK_RUNTIME_DIR")); v != "" {
		return v
	}
	if home, err := os.UserHomeDir(); err == nil && strings.TrimSpace(home) != "" {
		return filepath.Join(home, ".local", "share", "JACoB", "vulkan-runtime")
	}
	base := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR"))
	if base == "" {
		base = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	return filepath.Join(base, "jacob-vk")
}

func vulkanRendererReady() (bool, string) {
	p := filepath.Join(vulkanRuntimeDir(), "renderer.ready")
	st, err := os.Stat(p)
	if err != nil || time.Since(st.ModTime()) > 3*time.Second {
		return false, ""
	}
	b, err := os.ReadFile(p)
	if err != nil {
		return false, ""
	}
	s := string(b)
	return strings.Contains(s, "hud=1") && strings.Contains(s, "capture=1"), strings.TrimSpace(s)
}

func (o *vulkanOverlay) Info() OverlayInfo {
	_, visible := o.state.snapshot()
	ready, detail := vulkanRendererReady()
	note := "Vulkan HUD transport is ready; Elite has not connected the renderer yet."
	if ready {
		note = "Vulkan HUD renderer connected inside Elite; Vision uses the pre-HUD capture path."
	}
	if o.lastErr != "" {
		note += " Last transport error: " + o.lastErr
	}
	if detail != "" && !ready {
		note += " Renderer: " + detail
	}
	return HUD2Info(OverlayInfo{
		Available:           true,
		Driver:              o.Name(),
		Visible:             visible,
		Layers:              o.state.count(),
		Width:               vulkanHUDCanvasW,
		Height:              vulkanHUDCanvasH,
		SupportsAlpha:       true,
		SupportsText:        true,
		SupportsShapes:      []string{"text", "line", "polyline", "polygon", "rect", "circle"},
		CoordinateSpace:     []string{"normalized", "pixels"},
		SurfaceCreated:      ready,
		Mapped:              ready && visible && o.state.count() > 0,
		CompositionVerified: ready,
		Note:                note,
	})
}

func (o *vulkanOverlay) publish() error {
	o.mu.Lock()
	defer o.mu.Unlock()

	o.generation++
	scenes := o.state.orderedScenes()
	_, visible := o.state.snapshot()
	vertices := make([]vulkanHUDVertex, 0, 4096)
	if visible {
		for _, scene := range scenes {
			vulkanTessellateScene(&vertices, scene)
			if len(vertices) > vulkanHUDMaxVertices {
				o.lastErr = "overlay vertex limit exceeded"
				return fmt.Errorf("%s", o.lastErr)
			}
		}
	}

	var buf bytes.Buffer
	_ = binary.Write(&buf, binary.LittleEndian, vulkanHUDMagic)
	_ = binary.Write(&buf, binary.LittleEndian, vulkanHUDVersion)
	_ = binary.Write(&buf, binary.LittleEndian, o.generation)
	if visible {
		_ = binary.Write(&buf, binary.LittleEndian, uint32(1))
	} else {
		_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	}
	_ = binary.Write(&buf, binary.LittleEndian, uint32(len(vertices)))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(vulkanHUDCanvasW))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(vulkanHUDCanvasH))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	_ = binary.Write(&buf, binary.LittleEndian, uint32(0))
	for i := range vertices {
		if err := binary.Write(&buf, binary.LittleEndian, vertices[i]); err != nil {
			o.lastErr = err.Error()
			return err
		}
	}

	dir := vulkanRuntimeDir()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		o.lastErr = err.Error()
		return err
	}
	dst := filepath.Join(dir, "overlay.vertices")
	tmp := fmt.Sprintf("%s.tmp.%d", dst, os.Getpid())
	if err := os.WriteFile(tmp, buf.Bytes(), 0o600); err != nil {
		o.lastErr = err.Error()
		return err
	}
	if err := os.Rename(tmp, dst); err != nil {
		_ = os.Remove(tmp)
		o.lastErr = err.Error()
		return err
	}
	o.lastErr = ""
	return nil
}

func vkPxX(scene OverlayScene, v float64) float64 {
	if scene.Space == "normalized" {
		return v * vulkanHUDCanvasW
	}
	return v
}

func vkPxY(scene OverlayScene, v float64) float64 {
	if scene.Space == "normalized" {
		return v * vulkanHUDCanvasH
	}
	return v
}

func vkPxR(scene OverlayScene, v float64) float64 {
	if scene.Space == "normalized" {
		return v * math.Min(vulkanHUDCanvasW, vulkanHUDCanvasH)
	}
	return v
}

func vkColor(s string, opacity float64) (float32, float32, float32, float32) {
	r, g, b, a := overlayColorWithOpacity(s, opacity)
	const d = float32(1.0 / 255.0)
	return float32(r) * d, float32(g) * d, float32(b) * d, float32(a) * d
}

func vkAppendVertex(dst *[]vulkanHUDVertex, x, y float64, r, g, b, a float32) {
	if a <= 0 {
		return
	}
	*dst = append(*dst, vulkanHUDVertex{
		X: float32(x / vulkanHUDCanvasW),
		Y: float32(y / vulkanHUDCanvasH),
		R: r, G: g, B: b, A: a,
	})
}

func vkTri(dst *[]vulkanHUDVertex, ax, ay, bx, by, cx, cy float64, r, g, b, a float32) {
	vkAppendVertex(dst, ax, ay, r, g, b, a)
	vkAppendVertex(dst, bx, by, r, g, b, a)
	vkAppendVertex(dst, cx, cy, r, g, b, a)
}

func vkQuad(dst *[]vulkanHUDVertex, ax, ay, bx, by, cx, cy, dx, dy float64, r, g, b, a float32) {
	vkTri(dst, ax, ay, bx, by, cx, cy, r, g, b, a)
	vkTri(dst, ax, ay, cx, cy, dx, dy, r, g, b, a)
}

func vkLineSolid(dst *[]vulkanHUDVertex, x1, y1, x2, y2, width float64, r, g, b, a float32) {
	if a <= 0 || width <= 0 {
		return
	}
	dx, dy := x2-x1, y2-y1
	l := math.Hypot(dx, dy)
	if l < 0.001 {
		h := width / 2
		vkQuad(dst, x1-h, y1-h, x1+h, y1-h, x1+h, y1+h, x1-h, y1+h, r, g, b, a)
		return
	}
	nx, ny := -dy/l*width/2, dx/l*width/2
	vkQuad(dst, x1+nx, y1+ny, x2+nx, y2+ny, x2-nx, y2-ny, x1-nx, y1-ny, r, g, b, a)
}

func vkLine(dst *[]vulkanHUDVertex, x1, y1, x2, y2, width float64, dash string, r, g, b, a float32) {
	dash = strings.ToLower(strings.TrimSpace(dash))
	if dash == "" || dash == "solid" {
		vkLineSolid(dst, x1, y1, x2, y2, width, r, g, b, a)
		return
	}
	dx, dy := x2-x1, y2-y1
	length := math.Hypot(dx, dy)
	if length < 0.001 {
		return
	}
	ux, uy := dx/length, dy/length
	unit := math.Max(1, width)
	pattern := []float64{6 * unit, 4 * unit}
	if dash == "dot" {
		pattern = []float64{unit, 2.5 * unit}
	} else if dash == "dashdot" {
		pattern = []float64{6 * unit, 3 * unit, unit, 3 * unit}
	}
	pos, pi, draw := 0.0, 0, true
	for pos < length {
		n := pattern[pi%len(pattern)]
		end := math.Min(length, pos+n)
		if draw {
			vkLineSolid(dst, x1+ux*pos, y1+uy*pos, x1+ux*end, y1+uy*end, width, r, g, b, a)
		}
		pos = end
		pi++
		draw = !draw
	}
}

func vulkanTessellateScene(dst *[]vulkanHUDVertex, scene OverlayScene) {
	for _, it := range scene.Items {
		x := vkPxX(scene, it.X)
		y := vkPxY(scene, it.Y)
		x2 := vkPxX(scene, it.X2)
		y2 := vkPxY(scene, it.Y2)
		w := vkPxX(scene, it.W)
		h := vkPxY(scene, it.H)
		radius := vkPxR(scene, it.R)
		lw := math.Max(1, it.LineWidth)

		stroke := it.Stroke
		if stroke == "" {
			stroke = it.Color
		}
		sr, sg, sb, sa := vkColor(stroke, it.Opacity)
		fr, fg, fb, fa := vkColor(it.Fill, it.Opacity)

		if it.Glow > 0 && stroke != "" {
			glow := it.GlowColor
			if glow == "" {
				glow = stroke
			}
			gr, gg, gb, ga := vkColor(glow, it.Opacity*0.22)
			if ga > 0 {
				switch it.Type {
				case "line":
					vkLine(dst, x, y, x2, y2, lw+it.Glow*2, "solid", gr, gg, gb, ga)
				case "polyline", "polygon":
					pts := vulkanPoints(scene, it.Points)
					for i := 1; i < len(pts); i++ {
						vkLine(dst, pts[i-1][0], pts[i-1][1], pts[i][0], pts[i][1], lw+it.Glow*2, "solid", gr, gg, gb, ga)
					}
				}
			}
		}

		switch it.Type {
		case "line":
			vkLine(dst, x, y, x2, y2, lw, it.Dash, sr, sg, sb, sa)
		case "polyline":
			pts := vulkanPoints(scene, it.Points)
			for i := 1; i < len(pts); i++ {
				vkLine(dst, pts[i-1][0], pts[i-1][1], pts[i][0], pts[i][1], lw, it.Dash, sr, sg, sb, sa)
			}
		case "polygon":
			pts := vulkanPoints(scene, it.Points)
			if fa > 0 && len(pts) >= 3 {
				for i := 1; i+1 < len(pts); i++ {
					vkTri(dst, pts[0][0], pts[0][1], pts[i][0], pts[i][1], pts[i+1][0], pts[i+1][1], fr, fg, fb, fa)
				}
			}
			if sa > 0 && len(pts) >= 2 {
				for i := 1; i < len(pts); i++ {
					vkLine(dst, pts[i-1][0], pts[i-1][1], pts[i][0], pts[i][1], lw, it.Dash, sr, sg, sb, sa)
				}
				vkLine(dst, pts[len(pts)-1][0], pts[len(pts)-1][1], pts[0][0], pts[0][1], lw, it.Dash, sr, sg, sb, sa)
			}
		case "rect":
			if fa > 0 && w > 0 && h > 0 {
				vkQuad(dst, x, y, x+w, y, x+w, y+h, x, y+h, fr, fg, fb, fa)
			}
			if sa > 0 && w > 0 && h > 0 {
				vkLineSolid(dst, x, y, x+w, y, lw, sr, sg, sb, sa)
				vkLineSolid(dst, x+w, y, x+w, y+h, lw, sr, sg, sb, sa)
				vkLineSolid(dst, x+w, y+h, x, y+h, lw, sr, sg, sb, sa)
				vkLineSolid(dst, x, y+h, x, y, lw, sr, sg, sb, sa)
			}
		case "circle":
			if radius <= 0 {
				continue
			}
			const steps = 64
			if fa > 0 {
				for i := 0; i < steps; i++ {
					a1 := float64(i) * 2 * math.Pi / steps
					a2 := float64(i+1) * 2 * math.Pi / steps
					vkTri(dst, x, y, x+math.Cos(a1)*radius, y+math.Sin(a1)*radius, x+math.Cos(a2)*radius, y+math.Sin(a2)*radius, fr, fg, fb, fa)
				}
			}
			if sa > 0 {
				for i := 0; i < steps; i++ {
					a1 := float64(i) * 2 * math.Pi / steps
					a2 := float64(i+1) * 2 * math.Pi / steps
					vkLineSolid(dst, x+math.Cos(a1)*radius, y+math.Sin(a1)*radius, x+math.Cos(a2)*radius, y+math.Sin(a2)*radius, lw, sr, sg, sb, sa)
				}
			}
		case "text":
			vulkanText(dst, scene, it)
		}
	}
}

func vulkanPoints(scene OverlayScene, in []OverlayPoint) [][2]float64 {
	out := make([][2]float64, len(in))
	for i := range in {
		out[i] = [2]float64{vkPxX(scene, in[i].X), vkPxY(scene, in[i].Y)}
	}
	return out
}

func vulkanText(dst *[]vulkanHUDVertex, scene OverlayScene, it OverlayItem) {
	if it.Text == "" {
		return
	}
	size := math.Max(7, it.FontSize)
	cell := size / 7.0
	if cell < 1 {
		cell = 1
	}
	lines := strings.Split(it.Text, "\n")
	baseX, baseY := vkPxX(scene, it.X), vkPxY(scene, it.Y)

	textColor := it.Color
	if textColor == "" {
		textColor = it.Stroke
	}
	r, g, b, a := vkColor(textColor, it.Opacity)
	if a <= 0 {
		return
	}

	for li, line := range lines {
		runes := []rune(line)
		width := float64(len(runes)) * 6 * cell
		if len(runes) > 0 {
			width -= cell
		}
		x := baseX
		if it.Align == "center" {
			x -= width / 2
		} else if it.Align == "right" {
			x -= width
		}
		y := baseY + float64(li)*size*1.25

		if it.ShadowColor != "" && (it.ShadowX != 0 || it.ShadowY != 0) {
			rr, gg, bb, aa := vkColor(it.ShadowColor, it.Opacity)
			vulkanGlyphRun(dst, runes, x+it.ShadowX, y+it.ShadowY, cell, it, rr, gg, bb, aa)
		}
		if it.OutlineWidth > 0 && it.OutlineColor != "" {
			rr, gg, bb, aa := vkColor(it.OutlineColor, it.Opacity)
			d := math.Max(1, it.OutlineWidth)
			offsets := [][2]float64{{-d, 0}, {d, 0}, {0, -d}, {0, d}, {-d, -d}, {d, -d}, {-d, d}, {d, d}}
			for _, off := range offsets {
				vulkanGlyphRun(dst, runes, x+off[0], y+off[1], cell, it, rr, gg, bb, aa)
			}
		}
		if it.Glow > 0 && it.GlowColor != "" {
			rr, gg, bb, aa := vkColor(it.GlowColor, it.Opacity*0.18)
			d := math.Max(1, it.Glow*0.5)
			offsets := [][2]float64{{-d, 0}, {d, 0}, {0, -d}, {0, d}}
			for _, off := range offsets {
				vulkanGlyphRun(dst, runes, x+off[0], y+off[1], cell, it, rr, gg, bb, aa)
			}
		}
		vulkanGlyphRun(dst, runes, x, y, cell, it, r, g, b, a)
	}
}

func vulkanGlyphRun(dst *[]vulkanHUDVertex, runes []rune, x, y, cell float64, it OverlayItem, r, g, b, a float32) {
	if a <= 0 {
		return
	}
	for _, ch := range runes {
		glyph := vulkanGlyph(ch)
		italic := 0.0
		if it.Italic {
			italic = cell * 0.55
		}
		bold := 0.0
		if it.FontWeight >= 600 {
			bold = math.Max(0.45, cell*0.14)
		}
		for row := 0; row < 7; row++ {
			bits := glyph[row]
			skew := italic * float64(6-row) / 6
			for col := 0; col < 5; col++ {
				if bits&(1<<uint(4-col)) == 0 {
					continue
				}
				x0 := x + float64(col)*cell + skew
				y0 := y + float64(row)*cell
				vkQuad(dst, x0, y0, x0+cell+bold, y0, x0+cell+bold, y0+cell, x0, y0+cell, r, g, b, a)
			}
		}
		x += 6 * cell
	}
}

func vulkanGlyph(ch rune) [7]uint8 {
	ch = unicode.ToUpper(ch)
	if g, ok := vulkanGlyphTable[ch]; ok {
		return g
	}
	return vulkanGlyphTable['?']
}

var vulkanGlyphTable = map[rune][7]uint8{
	' ':  {0, 0, 0, 0, 0, 0, 0},
	'A':  {0x0E, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'B':  {0x1E, 0x11, 0x11, 0x1E, 0x11, 0x11, 0x1E},
	'C':  {0x0E, 0x11, 0x10, 0x10, 0x10, 0x11, 0x0E},
	'D':  {0x1E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x1E},
	'E':  {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x1F},
	'F':  {0x1F, 0x10, 0x10, 0x1E, 0x10, 0x10, 0x10},
	'G':  {0x0E, 0x11, 0x10, 0x17, 0x11, 0x11, 0x0E},
	'H':  {0x11, 0x11, 0x11, 0x1F, 0x11, 0x11, 0x11},
	'I':  {0x0E, 0x04, 0x04, 0x04, 0x04, 0x04, 0x0E},
	'J':  {0x07, 0x02, 0x02, 0x02, 0x12, 0x12, 0x0C},
	'K':  {0x11, 0x12, 0x14, 0x18, 0x14, 0x12, 0x11},
	'L':  {0x10, 0x10, 0x10, 0x10, 0x10, 0x10, 0x1F},
	'M':  {0x11, 0x1B, 0x15, 0x15, 0x11, 0x11, 0x11},
	'N':  {0x11, 0x19, 0x15, 0x13, 0x11, 0x11, 0x11},
	'O':  {0x0E, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'P':  {0x1E, 0x11, 0x11, 0x1E, 0x10, 0x10, 0x10},
	'Q':  {0x0E, 0x11, 0x11, 0x11, 0x15, 0x12, 0x0D},
	'R':  {0x1E, 0x11, 0x11, 0x1E, 0x14, 0x12, 0x11},
	'S':  {0x0F, 0x10, 0x10, 0x0E, 0x01, 0x01, 0x1E},
	'T':  {0x1F, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'U':  {0x11, 0x11, 0x11, 0x11, 0x11, 0x11, 0x0E},
	'V':  {0x11, 0x11, 0x11, 0x11, 0x11, 0x0A, 0x04},
	'W':  {0x11, 0x11, 0x11, 0x15, 0x15, 0x15, 0x0A},
	'X':  {0x11, 0x11, 0x0A, 0x04, 0x0A, 0x11, 0x11},
	'Y':  {0x11, 0x11, 0x0A, 0x04, 0x04, 0x04, 0x04},
	'Z':  {0x1F, 0x01, 0x02, 0x04, 0x08, 0x10, 0x1F},
	'0':  {0x0E, 0x11, 0x13, 0x15, 0x19, 0x11, 0x0E},
	'1':  {0x04, 0x0C, 0x14, 0x04, 0x04, 0x04, 0x1F},
	'2':  {0x0E, 0x11, 0x01, 0x02, 0x04, 0x08, 0x1F},
	'3':  {0x1E, 0x01, 0x01, 0x0E, 0x01, 0x01, 0x1E},
	'4':  {0x02, 0x06, 0x0A, 0x12, 0x1F, 0x02, 0x02},
	'5':  {0x1F, 0x10, 0x10, 0x1E, 0x01, 0x01, 0x1E},
	'6':  {0x0E, 0x10, 0x10, 0x1E, 0x11, 0x11, 0x0E},
	'7':  {0x1F, 0x01, 0x02, 0x04, 0x08, 0x08, 0x08},
	'8':  {0x0E, 0x11, 0x11, 0x0E, 0x11, 0x11, 0x0E},
	'9':  {0x0E, 0x11, 0x11, 0x0F, 0x01, 0x01, 0x0E},
	'.':  {0, 0, 0, 0, 0, 0x0C, 0x0C},
	',':  {0, 0, 0, 0, 0x0C, 0x0C, 0x08},
	':':  {0, 0x0C, 0x0C, 0, 0x0C, 0x0C, 0},
	';':  {0, 0x0C, 0x0C, 0, 0x0C, 0x0C, 0x08},
	'!':  {0x04, 0x04, 0x04, 0x04, 0x04, 0, 0x04},
	'?':  {0x0E, 0x11, 0x01, 0x02, 0x04, 0, 0x04},
	'-':  {0, 0, 0, 0x1F, 0, 0, 0},
	'_':  {0, 0, 0, 0, 0, 0, 0x1F},
	'+':  {0, 0x04, 0x04, 0x1F, 0x04, 0x04, 0},
	'=':  {0, 0, 0x1F, 0, 0x1F, 0, 0},
	'/':  {0x01, 0x02, 0x02, 0x04, 0x08, 0x08, 0x10},
	'\\': {0x10, 0x08, 0x08, 0x04, 0x02, 0x02, 0x01},
	'(':  {0x02, 0x04, 0x08, 0x08, 0x08, 0x04, 0x02},
	')':  {0x08, 0x04, 0x02, 0x02, 0x02, 0x04, 0x08},
	'[':  {0x0E, 0x08, 0x08, 0x08, 0x08, 0x08, 0x0E},
	']':  {0x0E, 0x02, 0x02, 0x02, 0x02, 0x02, 0x0E},
	'<':  {0x02, 0x04, 0x08, 0x10, 0x08, 0x04, 0x02},
	'>':  {0x08, 0x04, 0x02, 0x01, 0x02, 0x04, 0x08},
	'*':  {0, 0x15, 0x0E, 0x1F, 0x0E, 0x15, 0},
	'#':  {0x0A, 0x1F, 0x0A, 0x0A, 0x1F, 0x0A, 0x0A},
	'%':  {0x19, 0x19, 0x02, 0x04, 0x08, 0x13, 0x13},
	'|':  {0x04, 0x04, 0x04, 0x04, 0x04, 0x04, 0x04},
	'\'': {0x04, 0x04, 0x08, 0, 0, 0, 0},
	'"':  {0x0A, 0x0A, 0x14, 0, 0, 0, 0},
}
