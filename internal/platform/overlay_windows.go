//go:build windows

package platform

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"unsafe"
)

const (
	jacobOverlayClass = "JACoBOverlayWindow"
	wsPopup           = 0x80000000
	wsExTopmost       = 0x00000008
	wsExTransparent   = 0x00000020
	wsExToolWindow    = 0x00000080
	wsExLayered       = 0x00080000
	wsExNoActivate    = 0x08000000
	swHide            = 0
	swpNoActivate     = 0x0010
	swpShowWindow     = 0x0040
	lwaColorKey       = 0x00000001
	wmPaint           = 0x000F
	wmEraseBkgnd      = 0x0014
	wmTimer           = 0x0113
	wmDestroy         = 0x0002
	wmAppRender       = 0x8000 + 47
	psSolid           = 0
	transparentBk     = 1
	nullBrush         = 5
	nullPen           = 8
	nonAntialiasedQ   = 3
	fwNormal          = 400
)

var (
	overlayUser32 = syscall.NewLazyDLL("user32.dll")
	overlayGDI32  = syscall.NewLazyDLL("gdi32.dll")
	overlayKernel = syscall.NewLazyDLL("kernel32.dll")

	procRegisterClassW        = overlayUser32.NewProc("RegisterClassW")
	procCreateWindowExW       = overlayUser32.NewProc("CreateWindowExW")
	procDefWindowProcW        = overlayUser32.NewProc("DefWindowProcW")
	procPostMessageW          = overlayUser32.NewProc("PostMessageW")
	overlayProcGetMessageW    = overlayUser32.NewProc("GetMessageW")
	procTranslateMessage      = overlayUser32.NewProc("TranslateMessage")
	procDispatchMessageW      = overlayUser32.NewProc("DispatchMessageW")
	procSetLayeredWindowAttrs = overlayUser32.NewProc("SetLayeredWindowAttributes")
	procSetWindowPos          = overlayUser32.NewProc("SetWindowPos")
	procInvalidateRect        = overlayUser32.NewProc("InvalidateRect")
	procBeginPaint            = overlayUser32.NewProc("BeginPaint")
	procEndPaint              = overlayUser32.NewProc("EndPaint")
	procFillRect              = overlayUser32.NewProc("FillRect")
	procSetTimer              = overlayUser32.NewProc("SetTimer")
	procPostQuitMessage       = overlayUser32.NewProc("PostQuitMessage")
	procGetModuleHandleW      = overlayKernel.NewProc("GetModuleHandleW")
	procCreateSolidBrush      = overlayGDI32.NewProc("CreateSolidBrush")
	procCreatePen             = overlayGDI32.NewProc("CreatePen")
	overlayProcSelectObject   = overlayGDI32.NewProc("SelectObject")
	overlayProcDeleteObject   = overlayGDI32.NewProc("DeleteObject")
	procMoveToEx              = overlayGDI32.NewProc("MoveToEx")
	procLineTo                = overlayGDI32.NewProc("LineTo")
	procPolyline              = overlayGDI32.NewProc("Polyline")
	procPolygon               = overlayGDI32.NewProc("Polygon")
	procRectangle             = overlayGDI32.NewProc("Rectangle")
	procEllipse               = overlayGDI32.NewProc("Ellipse")
	procSetBkMode             = overlayGDI32.NewProc("SetBkMode")
	procSetTextColor          = overlayGDI32.NewProc("SetTextColor")
	procTextOutW              = overlayGDI32.NewProc("TextOutW")
	procGetTextExtentPoint32W = overlayGDI32.NewProc("GetTextExtentPoint32W")
	procCreateFontW           = overlayGDI32.NewProc("CreateFontW")
	procGetStockObject        = overlayGDI32.NewProc("GetStockObject")
)

type wndClassW struct {
	Style         uint32
	LpfnWndProc   uintptr
	CbClsExtra    int32
	CbWndExtra    int32
	HInstance     uintptr
	HIcon         uintptr
	HCursor       uintptr
	HbrBackground uintptr
	LpszMenuName  *uint16
	LpszClassName *uint16
}

type msg struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	Pt      point
}

type paintStruct struct {
	Hdc         uintptr
	Erase       int32
	RcPaint     rect
	Restore     int32
	IncUpdate   int32
	RGBReserved [32]byte
}

type size struct{ CX, CY int32 }

type winPoint struct{ X, Y int32 }

type windowsOverlay struct {
	state    *overlayState
	mu       sync.RWMutex
	hwnd     uintptr
	gameHwnd uintptr
	ready    chan struct{}
	err      error
	width    int
	height   int
}

var activeWindowsOverlay *windowsOverlay
var activeWindowsOverlayMu sync.RWMutex
var overlayWndProcCallback = syscall.NewCallback(overlayWndProc)

func newOverlayDriver() OverlayDriver {
	o := &windowsOverlay{state: newOverlayState(), ready: make(chan struct{})}
	activeWindowsOverlayMu.Lock()
	activeWindowsOverlay = o
	activeWindowsOverlayMu.Unlock()
	go o.run()
	<-o.ready
	return o
}

func (o *windowsOverlay) Name() string    { return "windows-layered-gdi" }
func (o *windowsOverlay) Available() bool { return o.err == nil && o.hwnd != 0 }

func (o *windowsOverlay) SetLayer(layer string, scene OverlayScene) error {
	if layer == "" {
		return fmt.Errorf("overlay layer is required")
	}
	if err := ValidateOverlayScene(&scene); err != nil {
		return err
	}
	if !o.Available() {
		return fmt.Errorf("overlay unavailable: %v", o.err)
	}
	o.state.set(layer, scene)
	o.requestRender()
	return nil
}
func (o *windowsOverlay) ClearLayer(layer string) error {
	o.state.clear(layer)
	o.requestRender()
	return nil
}
func (o *windowsOverlay) ClearPrefix(prefix string) error {
	o.state.clearPrefix(prefix)
	o.requestRender()
	return nil
}
func (o *windowsOverlay) ClearAll() error { o.state.clearAll(); o.requestRender(); return nil }
func (o *windowsOverlay) SetVisible(v bool) error {
	o.state.setVisible(v)
	o.requestRender()
	return nil
}
func (o *windowsOverlay) Info() OverlayInfo {
	_, visible := o.state.snapshot()
	o.mu.RLock()
	w, h := o.width, o.height
	o.mu.RUnlock()
	note := "Legacy-compatible color-key overlay; item alpha is treated as on/off. Works best with Elite in borderless/windowed fullscreen."
	if o.err != nil {
		note = o.err.Error()
	}
	return OverlayInfo{Available: o.Available(), Driver: o.Name(), Visible: visible, Layers: o.state.count(), Width: w, Height: h, SupportsAlpha: false, SupportsText: true, SupportsShapes: []string{"text", "line", "polyline", "polygon", "rect", "circle"}, CoordinateSpace: []string{"normalized", "pixels"}, Note: note}
}

func (o *windowsOverlay) requestRender() {
	o.mu.RLock()
	hwnd := o.hwnd
	o.mu.RUnlock()
	if hwnd != 0 {
		procPostMessageW.Call(hwnd, wmAppRender, 0, 0)
	}
}

func (o *windowsOverlay) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	instance, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString(jacobOverlayClass)
	wc := wndClassW{LpfnWndProc: overlayWndProcCallback, HInstance: instance, LpszClassName: className}
	r, _, e := procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 && e != syscall.Errno(0) && e != syscall.Errno(1410) { // class already exists is fine
		o.err = fmt.Errorf("RegisterClassW failed: %w", e)
		close(o.ready)
		return
	}
	title, _ := syscall.UTF16PtrFromString("JACoB HUD")
	hwnd, _, e := procCreateWindowExW.Call(wsExTopmost|wsExTransparent|wsExToolWindow|wsExLayered|wsExNoActivate, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(title)), wsPopup, 0, 0, 1, 1, 0, 0, instance, 0)
	if hwnd == 0 {
		o.err = fmt.Errorf("CreateWindowExW failed: %w", e)
		close(o.ready)
		return
	}
	procSetLayeredWindowAttrs.Call(hwnd, colorRef(255, 0, 255), 255, lwaColorKey)
	o.mu.Lock()
	o.hwnd = hwnd
	o.mu.Unlock()
	procSetTimer.Call(hwnd, 1, 200, 0)
	close(o.ready)
	o.syncWindow()
	var m msg
	for {
		gr, _, _ := overlayProcGetMessageW.Call(uintptr(unsafe.Pointer(&m)), 0, 0, 0)
		if int32(gr) <= 0 {
			break
		}
		procTranslateMessage.Call(uintptr(unsafe.Pointer(&m)))
		procDispatchMessageW.Call(uintptr(unsafe.Pointer(&m)))
	}
}

func overlayWndProc(hwnd uintptr, message uint32, wparam, lparam uintptr) uintptr {
	activeWindowsOverlayMu.RLock()
	o := activeWindowsOverlay
	activeWindowsOverlayMu.RUnlock()
	if o == nil {
		r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wparam, lparam)
		return r
	}
	switch message {
	case wmEraseBkgnd:
		return 1
	case wmTimer:
		o.syncWindow()
		return 0
	case wmAppRender:
		procInvalidateRect.Call(hwnd, 0, 0)
		return 0
	case wmPaint:
		o.paint(hwnd)
		return 0
	case wmDestroy:
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wparam, lparam)
	return r
}

func (o *windowsOverlay) syncWindow() {
	o.mu.RLock()
	hwnd := o.hwnd
	o.mu.RUnlock()
	if hwnd == 0 {
		return
	}
	layers, visible := o.state.snapshot()
	if !visible || len(layers) == 0 {
		procShowWindow.Call(hwnd, swHide)
		return
	}
	o.mu.RLock()
	game := o.gameHwnd
	o.mu.RUnlock()
	if game != 0 {
		valid, _, _ := procIsWindow.Call(game)
		if valid == 0 {
			game = 0
		}
	}
	if game == 0 {
		found, _, err := findEliteWindow()
		if err != nil || found == 0 {
			o.mu.Lock()
			o.gameHwnd = 0
			o.mu.Unlock()
			procShowWindow.Call(hwnd, swHide)
			return
		}
		game = found
		o.mu.Lock()
		o.gameHwnd = game
		o.mu.Unlock()
	}
	fg, _, _ := procGetForegroundWindow.Call()
	if fg != game {
		procShowWindow.Call(hwnd, swHide)
		return
	}
	var rc rect
	if r, _, _ := procGetClientRect.Call(game, uintptr(unsafe.Pointer(&rc))); r == 0 {
		procShowWindow.Call(hwnd, swHide)
		return
	}
	p := point{0, 0}
	if r, _, _ := procClientToScreen.Call(game, uintptr(unsafe.Pointer(&p))); r == 0 {
		procShowWindow.Call(hwnd, swHide)
		return
	}
	w, h := int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	if w <= 0 || h <= 0 {
		procShowWindow.Call(hwnd, swHide)
		return
	}
	const hwndTopmost = ^uintptr(0) // (HWND)-1
	procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(int32(p.X)), uintptr(int32(p.Y)), uintptr(w), uintptr(h), swpNoActivate|swpShowWindow)
	o.mu.Lock()
	o.width = w
	o.height = h
	o.mu.Unlock()
}

func (o *windowsOverlay) paint(hwnd uintptr) {
	var ps paintStruct
	hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	if hdc == 0 {
		return
	}
	defer procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
	o.mu.RLock()
	w, h := o.width, o.height
	o.mu.RUnlock()
	if w <= 0 || h <= 0 {
		return
	}
	bg, _, _ := procCreateSolidBrush.Call(colorRef(255, 0, 255))
	if bg != 0 {
		rc := rect{0, 0, int32(w), int32(h)}
		procFillRect.Call(hdc, uintptr(unsafe.Pointer(&rc)), bg)
		overlayProcDeleteObject.Call(bg)
	}
	procSetBkMode.Call(hdc, transparentBk)
	_, visible := o.state.snapshot()
	if !visible {
		return
	}
	for _, scene := range o.state.orderedScenes() {
		drawSceneWindows(hdc, w, h, scene)
	}
}

func drawSceneWindows(hdc uintptr, width, height int, scene OverlayScene) {
	for _, it := range scene.Items {
		x := overlayCoord(it.X, width, scene.Space)
		y := overlayCoord(it.Y, height, scene.Space)
		x2 := overlayCoord(it.X2, width, scene.Space)
		y2 := overlayCoord(it.Y2, height, scene.Space)
		iw := overlayCoord(it.W, width, scene.Space)
		ih := overlayCoord(it.H, height, scene.Space)
		stroke := it.Stroke
		if stroke == "" {
			stroke = it.Color
		}
		if stroke == "" {
			stroke = "#ffffff"
		}
		sr, sg, sb, sa, _ := ParseOverlayColor(stroke)
		fr, fg, fb, fa, _ := ParseOverlayColor(it.Fill)
		pen := uintptr(0)
		brush := uintptr(0)
		if sa > 0 {
			pen, _, _ = procCreatePen.Call(psSolid, uintptr(int(math.Max(1, it.LineWidth))), colorRef(sr, sg, sb))
		} else {
			pen, _, _ = procGetStockObject.Call(nullPen)
		}
		if fa > 0 {
			brush, _, _ = procCreateSolidBrush.Call(colorRef(fr, fg, fb))
		} else {
			brush, _, _ = procGetStockObject.Call(nullBrush)
		}
		oldPen, _, _ := overlayProcSelectObject.Call(hdc, pen)
		oldBrush, _, _ := overlayProcSelectObject.Call(hdc, brush)
		switch it.Type {
		case "line":
			if sa > 0 {
				procMoveToEx.Call(hdc, uintptr(x), uintptr(y), 0)
				procLineTo.Call(hdc, uintptr(x2), uintptr(y2))
			}
		case "polyline", "polygon":
			if len(it.Points) >= 2 {
				pts := make([]winPoint, len(it.Points))
				for i, p := range it.Points {
					pts[i] = winPoint{int32(overlayCoord(p.X, width, scene.Space)), int32(overlayCoord(p.Y, height, scene.Space))}
				}
				if it.Type == "polygon" {
					procPolygon.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
				} else if sa > 0 {
					procPolyline.Call(hdc, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
				}
			}
		case "rect":
			procRectangle.Call(hdc, uintptr(x), uintptr(y), uintptr(x+iw), uintptr(y+ih))
		case "circle":
			r := overlayRadius(it.R, width, height, scene.Space)
			procEllipse.Call(hdc, uintptr(x-r), uintptr(y-r), uintptr(x+r), uintptr(y+r))
		case "text":
			cr, cg, cb, ca, _ := ParseOverlayColor(func() string {
				if it.Color != "" {
					return it.Color
				}
				return "#ffffff"
			}())
			if ca > 0 {
				drawTextWindows(hdc, x, y, it.Text, int(math.Round(it.FontSize)), it.Align, colorRef(cr, cg, cb))
			}
		}
		overlayProcSelectObject.Call(hdc, oldPen)
		overlayProcSelectObject.Call(hdc, oldBrush)
		if sa > 0 && pen != 0 {
			overlayProcDeleteObject.Call(pen)
		}
		if fa > 0 && brush != 0 {
			overlayProcDeleteObject.Call(brush)
		}
	}
}

func drawTextWindows(hdc uintptr, x, y int, text string, fontSize int, align string, color uintptr) {
	if fontSize < 6 {
		fontSize = 6
	}
	if fontSize > 256 {
		fontSize = 256
	}
	face, _ := syscall.UTF16PtrFromString("Segoe UI")
	font, _, _ := procCreateFontW.Call(uintptr(int32(-fontSize)), 0, 0, 0, fwNormal, 0, 0, 0, 1, 0, 0, nonAntialiasedQ, 0, uintptr(unsafe.Pointer(face)))
	if font == 0 {
		return
	}
	defer overlayProcDeleteObject.Call(font)
	old, _, _ := overlayProcSelectObject.Call(hdc, font)
	defer overlayProcSelectObject.Call(hdc, old)
	procSetTextColor.Call(hdc, color)
	lines := strings.Split(text, "\n")
	for i, line := range lines {
		u, _ := syscall.UTF16FromString(line)
		if len(u) <= 1 {
			continue
		}
		n := len(u) - 1
		var sz size
		procGetTextExtentPoint32W.Call(hdc, uintptr(unsafe.Pointer(&u[0])), uintptr(n), uintptr(unsafe.Pointer(&sz)))
		tx := x
		if align == "center" {
			tx -= int(sz.CX) / 2
		} else if align == "right" {
			tx -= int(sz.CX)
		}
		ty := y + i*(fontSize+2)
		procTextOutW.Call(hdc, uintptr(tx), uintptr(ty), uintptr(unsafe.Pointer(&u[0])), uintptr(n))
	}
}

func colorRef(r, g, b uint8) uintptr { return uintptr(uint32(r) | uint32(g)<<8 | uint32(b)<<16) }
