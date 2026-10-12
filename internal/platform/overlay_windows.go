//go:build windows

package platform

import (
	"fmt"
	"math"
	"runtime"
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
	wmPaint           = 0x000F
	wmEraseBkgnd      = 0x0014
	wmTimer           = 0x0113
	wmDestroy         = 0x0002
	wmAppRender       = 0x8000 + 47
	ulwAlpha          = 0x00000002
	acSrcOver         = 0x00
	acSrcAlpha        = 0x01
	dibRGBColors      = 0
	biRGB             = 0
	pixelFormatPARGB  = 0x000E200B
	unitPixel         = 2
	fontStyleRegular  = 0
	fontStyleBold     = 1
	fontStyleItalic   = 2
	fillModeAlternate = 0
	lineCapRound      = 2
	dashCapRound      = 2
)

var (
	overlayUser32  = syscall.NewLazyDLL("user32.dll")
	overlayGDI32   = syscall.NewLazyDLL("gdi32.dll")
	overlayKernel  = syscall.NewLazyDLL("kernel32.dll")
	overlayGDIPlus = syscall.NewLazyDLL("gdiplus.dll")

	procRegisterClassW      = overlayUser32.NewProc("RegisterClassW")
	procCreateWindowExW     = overlayUser32.NewProc("CreateWindowExW")
	procDefWindowProcW      = overlayUser32.NewProc("DefWindowProcW")
	procPostMessageW        = overlayUser32.NewProc("PostMessageW")
	overlayProcGetMessageW  = overlayUser32.NewProc("GetMessageW")
	procTranslateMessage    = overlayUser32.NewProc("TranslateMessage")
	procDispatchMessageW    = overlayUser32.NewProc("DispatchMessageW")
	procSetWindowPos        = overlayUser32.NewProc("SetWindowPos")
	procBeginPaint          = overlayUser32.NewProc("BeginPaint")
	procEndPaint            = overlayUser32.NewProc("EndPaint")
	procSetTimer            = overlayUser32.NewProc("SetTimer")
	procPostQuitMessage     = overlayUser32.NewProc("PostQuitMessage")
	procUpdateLayeredWindow = overlayUser32.NewProc("UpdateLayeredWindow")

	procGetModuleHandleW    = overlayKernel.NewProc("GetModuleHandleW")
	procCreateCompatibleDC  = overlayGDI32.NewProc("CreateCompatibleDC")
	procDeleteDC            = overlayGDI32.NewProc("DeleteDC")
	procCreateDIBSection    = overlayGDI32.NewProc("CreateDIBSection")
	overlayProcSelectObject = overlayGDI32.NewProc("SelectObject")
	overlayProcDeleteObject = overlayGDI32.NewProc("DeleteObject")

	procGdiplusStartup              = overlayGDIPlus.NewProc("GdiplusStartup")
	procGdiplusShutdown             = overlayGDIPlus.NewProc("GdiplusShutdown")
	procGdipCreateBitmapFromScan0   = overlayGDIPlus.NewProc("GdipCreateBitmapFromScan0")
	procGdipDisposeImage            = overlayGDIPlus.NewProc("GdipDisposeImage")
	procGdipGetImageGraphicsContext = overlayGDIPlus.NewProc("GdipGetImageGraphicsContext")
	procGdipDeleteGraphics          = overlayGDIPlus.NewProc("GdipDeleteGraphics")
	procGdipGraphicsClear           = overlayGDIPlus.NewProc("GdipGraphicsClear")
	procGdipSetSmoothingMode        = overlayGDIPlus.NewProc("GdipSetSmoothingMode")
	procGdipSetTextRenderingHint    = overlayGDIPlus.NewProc("GdipSetTextRenderingHint")
	procGdipSetPixelOffsetMode      = overlayGDIPlus.NewProc("GdipSetPixelOffsetMode")
	procGdipSetCompositingQuality   = overlayGDIPlus.NewProc("GdipSetCompositingQuality")
	procGdipCreatePen1              = overlayGDIPlus.NewProc("GdipCreatePen1")
	procGdipDeletePen               = overlayGDIPlus.NewProc("GdipDeletePen")
	procGdipSetPenDashStyle         = overlayGDIPlus.NewProc("GdipSetPenDashStyle")
	procGdipSetPenLineCap           = overlayGDIPlus.NewProc("GdipSetPenLineCap197819")
	procGdipDrawLineI               = overlayGDIPlus.NewProc("GdipDrawLineI")
	procGdipDrawLinesI              = overlayGDIPlus.NewProc("GdipDrawLinesI")
	procGdipDrawPolygonI            = overlayGDIPlus.NewProc("GdipDrawPolygonI")
	procGdipFillPolygonI            = overlayGDIPlus.NewProc("GdipFillPolygonI")
	procGdipDrawRectangleI          = overlayGDIPlus.NewProc("GdipDrawRectangleI")
	procGdipFillRectangleI          = overlayGDIPlus.NewProc("GdipFillRectangleI")
	procGdipDrawEllipseI            = overlayGDIPlus.NewProc("GdipDrawEllipseI")
	procGdipFillEllipseI            = overlayGDIPlus.NewProc("GdipFillEllipseI")
	procGdipCreateSolidFill         = overlayGDIPlus.NewProc("GdipCreateSolidFill")
	procGdipDeleteBrush             = overlayGDIPlus.NewProc("GdipDeleteBrush")
	procGdipCreatePath              = overlayGDIPlus.NewProc("GdipCreatePath")
	procGdipDeletePath              = overlayGDIPlus.NewProc("GdipDeletePath")
	procGdipAddPathArcI             = overlayGDIPlus.NewProc("GdipAddPathArcI")
	procGdipClosePathFigure         = overlayGDIPlus.NewProc("GdipClosePathFigure")
	procGdipDrawPath                = overlayGDIPlus.NewProc("GdipDrawPath")
	procGdipFillPath                = overlayGDIPlus.NewProc("GdipFillPath")
	procGdipCreateFontFamily        = overlayGDIPlus.NewProc("GdipCreateFontFamilyFromName")
	procGdipDeleteFontFamily        = overlayGDIPlus.NewProc("GdipDeleteFontFamily")
	procGdipCreateFont              = overlayGDIPlus.NewProc("GdipCreateFont")
	procGdipDeleteFont              = overlayGDIPlus.NewProc("GdipDeleteFont")
	procGdipDrawString              = overlayGDIPlus.NewProc("GdipDrawString")
	procGdipMeasureString           = overlayGDIPlus.NewProc("GdipMeasureString")
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

type gdiSize struct{ CX, CY int32 }
type gdipPoint struct{ X, Y int32 }
type rectF struct{ X, Y, Width, Height float32 }

type blendFunction struct {
	BlendOp             byte
	BlendFlags          byte
	SourceConstantAlpha byte
	AlphaFormat         byte
}

type bitmapInfoHeader struct {
	Size          uint32
	Width         int32
	Height        int32
	Planes        uint16
	BitCount      uint16
	Compression   uint32
	SizeImage     uint32
	XPelsPerMeter int32
	YPelsPerMeter int32
	ClrUsed       uint32
	ClrImportant  uint32
}

type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]uint32
}

type gdiplusStartupInput struct {
	GdiplusVersion           uint32
	DebugEventCallback       uintptr
	SuppressBackgroundThread int32
	SuppressExternalCodecs   int32
}

type windowsOverlay struct {
	state    *overlayState
	mu       sync.RWMutex
	hwnd     uintptr
	gameHwnd uintptr
	ready    chan struct{}
	err      error
	width    int
	height   int
	dst      point

	gdiToken  uintptr
	memDC     uintptr
	hbitmap   uintptr
	oldBitmap uintptr
	bits      uintptr
	image     uintptr
	graphics  uintptr
	surfaceW  int
	surfaceH  int
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

func (o *windowsOverlay) Name() string    { return "windows-layered-gdiplus-hud2" }
func (o *windowsOverlay) Available() bool { return o.err == nil && o.hwnd != 0 && o.gdiToken != 0 }

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
	note := "HUD2 per-pixel alpha renderer with GDI+ antialiasing. Elite should run borderless/windowed fullscreen for reliable overlay compositing."
	if o.err != nil {
		note = o.err.Error()
	}
	return HUD2Info(OverlayInfo{Available: o.Available(), Driver: o.Name(), Visible: visible, Layers: o.state.count(), Width: w, Height: h, SupportsAlpha: true, SupportsText: true, SupportsShapes: []string{"text", "line", "polyline", "polygon", "rect", "circle"}, CoordinateSpace: []string{"normalized", "pixels"}, Note: note})
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
	if err := o.startGDIPlus(); err != nil {
		o.err = err
		close(o.ready)
		return
	}
	defer o.stopGDIPlus()

	instance, _, _ := procGetModuleHandleW.Call(0)
	className, _ := syscall.UTF16PtrFromString(jacobOverlayClass)
	wc := wndClassW{LpfnWndProc: overlayWndProcCallback, HInstance: instance, LpszClassName: className}
	r, _, e := procRegisterClassW.Call(uintptr(unsafe.Pointer(&wc)))
	if r == 0 && e != syscall.Errno(0) && e != syscall.Errno(1410) {
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
	o.mu.Lock()
	o.hwnd = hwnd
	o.mu.Unlock()
	procSetTimer.Call(hwnd, 1, 200, 0)
	close(o.ready)
	o.syncWindow()
	o.render()

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
		changed := o.syncWindow()
		if changed {
			o.render()
		}
		return 0
	case wmAppRender:
		o.syncWindow()
		o.render()
		return 0
	case wmPaint:
		var ps paintStruct
		hdc, _, _ := procBeginPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		if hdc != 0 {
			procEndPaint.Call(hwnd, uintptr(unsafe.Pointer(&ps)))
		}
		return 0
	case wmDestroy:
		o.releaseSurface()
		procPostQuitMessage.Call(0)
		return 0
	}
	r, _, _ := procDefWindowProcW.Call(hwnd, uintptr(message), wparam, lparam)
	return r
}

func (o *windowsOverlay) startGDIPlus() error {
	input := gdiplusStartupInput{GdiplusVersion: 1}
	status, _, _ := procGdiplusStartup.Call(uintptr(unsafe.Pointer(&o.gdiToken)), uintptr(unsafe.Pointer(&input)), 0)
	if status != 0 || o.gdiToken == 0 {
		return fmt.Errorf("GdiplusStartup failed with status %d", status)
	}
	return nil
}

func (o *windowsOverlay) stopGDIPlus() {
	o.releaseSurface()
	if o.gdiToken != 0 {
		procGdiplusShutdown.Call(o.gdiToken)
		o.gdiToken = 0
	}
}

func (o *windowsOverlay) syncWindow() bool {
	o.mu.RLock()
	hwnd := o.hwnd
	oldW, oldH, oldDst := o.width, o.height, o.dst
	o.mu.RUnlock()
	if hwnd == 0 {
		return false
	}
	layers, visible := o.state.snapshot()
	if !visible || len(layers) == 0 {
		procShowWindow.Call(hwnd, swHide)
		return false
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
			return false
		}
		game = found
		o.mu.Lock()
		o.gameHwnd = game
		o.mu.Unlock()
	}
	fg, _, _ := procGetForegroundWindow.Call()
	if fg != game {
		procShowWindow.Call(hwnd, swHide)
		return false
	}
	var rc rect
	if r, _, _ := procGetClientRect.Call(game, uintptr(unsafe.Pointer(&rc))); r == 0 {
		procShowWindow.Call(hwnd, swHide)
		return false
	}
	p := point{0, 0}
	if r, _, _ := procClientToScreen.Call(game, uintptr(unsafe.Pointer(&p))); r == 0 {
		procShowWindow.Call(hwnd, swHide)
		return false
	}
	w, h := int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	if w <= 0 || h <= 0 {
		procShowWindow.Call(hwnd, swHide)
		return false
	}
	const hwndTopmost = ^uintptr(0)
	procSetWindowPos.Call(hwnd, hwndTopmost, uintptr(p.X), uintptr(p.Y), uintptr(w), uintptr(h), swpNoActivate|swpShowWindow)
	o.mu.Lock()
	o.width, o.height, o.dst = w, h, p
	o.mu.Unlock()
	return oldW != w || oldH != h || oldDst != p
}

func (o *windowsOverlay) ensureSurface(w, h int) error {
	if w == o.surfaceW && h == o.surfaceH && o.graphics != 0 && o.memDC != 0 {
		return nil
	}
	o.releaseSurface()
	memDC, _, e := procCreateCompatibleDC.Call(0)
	if memDC == 0 {
		return fmt.Errorf("CreateCompatibleDC failed: %w", e)
	}
	bmi := bitmapInfo{Header: bitmapInfoHeader{Size: uint32(unsafe.Sizeof(bitmapInfoHeader{})), Width: int32(w), Height: int32(-h), Planes: 1, BitCount: 32, Compression: biRGB}}
	var bits uintptr
	hbitmap, _, e := procCreateDIBSection.Call(memDC, uintptr(unsafe.Pointer(&bmi)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbitmap == 0 || bits == 0 {
		procDeleteDC.Call(memDC)
		return fmt.Errorf("CreateDIBSection failed: %w", e)
	}
	old, _, _ := overlayProcSelectObject.Call(memDC, hbitmap)
	var image uintptr
	status, _, _ := procGdipCreateBitmapFromScan0.Call(uintptr(w), uintptr(h), uintptr(w*4), pixelFormatPARGB, bits, uintptr(unsafe.Pointer(&image)))
	if status != 0 || image == 0 {
		overlayProcSelectObject.Call(memDC, old)
		overlayProcDeleteObject.Call(hbitmap)
		procDeleteDC.Call(memDC)
		return fmt.Errorf("GdipCreateBitmapFromScan0 failed with status %d", status)
	}
	var graphics uintptr
	status, _, _ = procGdipGetImageGraphicsContext.Call(image, uintptr(unsafe.Pointer(&graphics)))
	if status != 0 || graphics == 0 {
		procGdipDisposeImage.Call(image)
		overlayProcSelectObject.Call(memDC, old)
		overlayProcDeleteObject.Call(hbitmap)
		procDeleteDC.Call(memDC)
		return fmt.Errorf("GdipGetImageGraphicsContext failed with status %d", status)
	}
	procGdipSetSmoothingMode.Call(graphics, 6)
	procGdipSetTextRenderingHint.Call(graphics, 3)
	procGdipSetPixelOffsetMode.Call(graphics, 4)
	procGdipSetCompositingQuality.Call(graphics, 4)
	o.memDC, o.hbitmap, o.oldBitmap, o.bits, o.image, o.graphics = memDC, hbitmap, old, bits, image, graphics
	o.surfaceW, o.surfaceH = w, h
	return nil
}

func (o *windowsOverlay) releaseSurface() {
	if o.graphics != 0 {
		procGdipDeleteGraphics.Call(o.graphics)
		o.graphics = 0
	}
	if o.image != 0 {
		procGdipDisposeImage.Call(o.image)
		o.image = 0
	}
	if o.memDC != 0 && o.oldBitmap != 0 {
		overlayProcSelectObject.Call(o.memDC, o.oldBitmap)
	}
	if o.hbitmap != 0 {
		overlayProcDeleteObject.Call(o.hbitmap)
	}
	if o.memDC != 0 {
		procDeleteDC.Call(o.memDC)
	}
	o.memDC, o.hbitmap, o.oldBitmap, o.bits = 0, 0, 0, 0
	o.surfaceW, o.surfaceH = 0, 0
}

func (o *windowsOverlay) render() {
	if !o.Available() {
		return
	}
	layers, visible := o.state.snapshot()
	if !visible || len(layers) == 0 {
		return
	}
	o.mu.RLock()
	w, h, dst, hwnd := o.width, o.height, o.dst, o.hwnd
	o.mu.RUnlock()
	if w <= 0 || h <= 0 || hwnd == 0 {
		return
	}
	if err := o.ensureSurface(w, h); err != nil {
		o.mu.Lock()
		o.err = err
		o.mu.Unlock()
		return
	}
	procGdipGraphicsClear.Call(o.graphics, 0x00000000)
	for _, scene := range o.state.orderedScenes() {
		drawSceneWindowsHUD2(o.graphics, w, h, scene)
	}
	src := point{0, 0}
	sz := gdiSize{int32(w), int32(h)}
	blend := blendFunction{BlendOp: acSrcOver, SourceConstantAlpha: 255, AlphaFormat: acSrcAlpha}
	ok, _, e := procUpdateLayeredWindow.Call(hwnd, 0, uintptr(unsafe.Pointer(&dst)), uintptr(unsafe.Pointer(&sz)), o.memDC, uintptr(unsafe.Pointer(&src)), 0, uintptr(unsafe.Pointer(&blend)), ulwAlpha)
	if ok == 0 && e != syscall.Errno(0) {
		o.mu.Lock()
		o.err = fmt.Errorf("UpdateLayeredWindow failed: %w", e)
		o.mu.Unlock()
	}
}

func drawSceneWindowsHUD2(graphics uintptr, width, height int, scene OverlayScene) {
	for _, it := range scene.Items {
		drawWindowsItem(graphics, width, height, scene, it)
	}
}

func drawWindowsItem(graphics uintptr, width, height int, scene OverlayScene, it OverlayItem) {
	x := overlayCoord(it.X, width, scene.Space)
	y := overlayCoord(it.Y, height, scene.Space)
	x2 := overlayCoord(it.X2, width, scene.Space)
	y2 := overlayCoord(it.Y2, height, scene.Space)
	iw := overlayCoord(it.W, width, scene.Space)
	ih := overlayCoord(it.H, height, scene.Space)

	if it.Type == "text" {
		drawWindowsTextHUD2(graphics, x, y, it)
		return
	}

	stroke := it.Stroke
	if stroke == "" {
		stroke = it.Color
	}
	if stroke == "" {
		stroke = "#ffffff"
	}
	fill := it.Fill
	if it.Glow > 0 {
		glowColor := it.GlowColor
		if glowColor == "" {
			glowColor = stroke
		}
		for pass := 3; pass >= 1; pass-- {
			alpha := it.Opacity * (0.08 + float64(4-pass)*0.03)
			pen := createWindowsPen(glowColor, alpha, it.LineWidth+it.Glow*float64(pass)*0.8, "solid")
			if pen != 0 {
				drawWindowsShape(graphics, width, height, scene, it, x, y, x2, y2, iw, ih, pen, 0)
				procGdipDeletePen.Call(pen)
			}
		}
	}
	pen := createWindowsPen(stroke, it.Opacity, it.LineWidth, it.Dash)
	brush := createWindowsBrush(fill, it.Opacity)
	drawWindowsShape(graphics, width, height, scene, it, x, y, x2, y2, iw, ih, pen, brush)
	if pen != 0 {
		procGdipDeletePen.Call(pen)
	}
	if brush != 0 {
		procGdipDeleteBrush.Call(brush)
	}
}

func drawWindowsShape(graphics uintptr, width, height int, scene OverlayScene, it OverlayItem, x, y, x2, y2, iw, ih int, pen, brush uintptr) {
	switch it.Type {
	case "line":
		if pen != 0 {
			procGdipDrawLineI.Call(graphics, pen, uintptr(x), uintptr(y), uintptr(x2), uintptr(y2))
		}
	case "polyline", "polygon":
		if len(it.Points) < 2 {
			return
		}
		pts := make([]gdipPoint, len(it.Points))
		for i, p := range it.Points {
			pts[i] = gdipPoint{int32(overlayCoord(p.X, width, scene.Space)), int32(overlayCoord(p.Y, height, scene.Space))}
		}
		if it.Type == "polygon" {
			if brush != 0 {
				procGdipFillPolygonI.Call(graphics, brush, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)), fillModeAlternate)
			}
			if pen != 0 {
				procGdipDrawPolygonI.Call(graphics, pen, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
			}
		} else if pen != 0 {
			procGdipDrawLinesI.Call(graphics, pen, uintptr(unsafe.Pointer(&pts[0])), uintptr(len(pts)))
		}
	case "rect":
		cr := overlayCoord(it.CornerRadius, minInt(width, height), scene.Space)
		if cr <= 0 {
			if brush != 0 {
				procGdipFillRectangleI.Call(graphics, brush, uintptr(x), uintptr(y), uintptr(iw), uintptr(ih))
			}
			if pen != 0 {
				procGdipDrawRectangleI.Call(graphics, pen, uintptr(x), uintptr(y), uintptr(iw), uintptr(ih))
			}
			return
		}
		path := createRoundedRectPath(x, y, iw, ih, cr)
		if path != 0 {
			if brush != 0 {
				procGdipFillPath.Call(graphics, brush, path)
			}
			if pen != 0 {
				procGdipDrawPath.Call(graphics, pen, path)
			}
			procGdipDeletePath.Call(path)
		}
	case "circle":
		r := overlayRadius(it.R, width, height, scene.Space)
		if brush != 0 {
			procGdipFillEllipseI.Call(graphics, brush, uintptr(x-r), uintptr(y-r), uintptr(r*2), uintptr(r*2))
		}
		if pen != 0 {
			procGdipDrawEllipseI.Call(graphics, pen, uintptr(x-r), uintptr(y-r), uintptr(r*2), uintptr(r*2))
		}
	}
}

func createRoundedRectPath(x, y, w, h, radius int) uintptr {
	if radius*2 > w {
		radius = w / 2
	}
	if radius*2 > h {
		radius = h / 2
	}
	if radius <= 0 {
		return 0
	}
	var path uintptr
	if status, _, _ := procGdipCreatePath.Call(fillModeAlternate, uintptr(unsafe.Pointer(&path))); status != 0 || path == 0 {
		return 0
	}
	d := radius * 2
	procGdipAddPathArcI.Call(path, uintptr(x), uintptr(y), uintptr(d), uintptr(d), mathFloat32Bits(180), mathFloat32Bits(90))
	procGdipAddPathArcI.Call(path, uintptr(x+w-d), uintptr(y), uintptr(d), uintptr(d), mathFloat32Bits(270), mathFloat32Bits(90))
	procGdipAddPathArcI.Call(path, uintptr(x+w-d), uintptr(y+h-d), uintptr(d), uintptr(d), mathFloat32Bits(0), mathFloat32Bits(90))
	procGdipAddPathArcI.Call(path, uintptr(x), uintptr(y+h-d), uintptr(d), uintptr(d), mathFloat32Bits(90), mathFloat32Bits(90))
	procGdipClosePathFigure.Call(path)
	return path
}

func createWindowsPen(color string, opacity, width float64, dash string) uintptr {
	r, g, b, a := overlayColorWithOpacity(color, opacity)
	if a == 0 {
		return 0
	}
	var pen uintptr
	status, _, _ := procGdipCreatePen1.Call(uintptr(argb(r, g, b, a)), mathFloat32Bits(float32(math.Max(0.5, width))), unitPixel, uintptr(unsafe.Pointer(&pen)))
	if status != 0 || pen == 0 {
		return 0
	}
	procGdipSetPenLineCap.Call(pen, lineCapRound, lineCapRound, dashCapRound)
	switch dash {
	case "dash":
		procGdipSetPenDashStyle.Call(pen, 1)
	case "dot":
		procGdipSetPenDashStyle.Call(pen, 2)
	case "dashdot":
		procGdipSetPenDashStyle.Call(pen, 3)
	default:
		procGdipSetPenDashStyle.Call(pen, 0)
	}
	return pen
}

func createWindowsBrush(color string, opacity float64) uintptr {
	r, g, b, a := overlayColorWithOpacity(color, opacity)
	if a == 0 {
		return 0
	}
	var brush uintptr
	status, _, _ := procGdipCreateSolidFill.Call(uintptr(argb(r, g, b, a)), uintptr(unsafe.Pointer(&brush)))
	if status != 0 {
		return 0
	}
	return brush
}

func drawWindowsTextHUD2(graphics uintptr, x, y int, it OverlayItem) {
	familyName := it.FontFamily
	if familyName == "" {
		familyName = "Segoe UI"
	}
	family := createWindowsFontFamily(familyName)
	if family == 0 && familyName != "Segoe UI" {
		family = createWindowsFontFamily("Segoe UI")
	}
	if family == 0 {
		return
	}
	defer procGdipDeleteFontFamily.Call(family)
	style := fontStyleRegular
	if it.FontWeight >= 600 {
		style |= fontStyleBold
	}
	if it.Italic {
		style |= fontStyleItalic
	}
	var font uintptr
	status, _, _ := procGdipCreateFont.Call(family, mathFloat32Bits(float32(it.FontSize)), uintptr(style), unitPixel, uintptr(unsafe.Pointer(&font)))
	if status != 0 || font == 0 {
		return
	}
	defer procGdipDeleteFont.Call(font)
	u, err := syscall.UTF16FromString(it.Text)
	if err != nil || len(u) <= 1 {
		return
	}
	n := len(u) - 1
	measure := rectF{X: 0, Y: 0, Width: 4096, Height: 4096}
	var bounds rectF
	procGdipMeasureString.Call(graphics, uintptr(unsafe.Pointer(&u[0])), uintptr(n), font, uintptr(unsafe.Pointer(&measure)), 0, uintptr(unsafe.Pointer(&bounds)), 0, 0)
	tw := float64(bounds.Width)
	tx := float64(x)
	if it.Align == "center" {
		tx -= tw / 2
	} else if it.Align == "right" {
		tx -= tw
	}

	if it.ShadowX != 0 || it.ShadowY != 0 {
		color := it.ShadowColor
		if color == "" {
			color = "#000000AA"
		}
		drawWindowsStringPass(graphics, font, u, tx+it.ShadowX, float64(y)+it.ShadowY, color, it.Opacity)
	}
	if it.Glow > 0 {
		color := it.GlowColor
		if color == "" {
			color = it.Color
		}
		radius := int(math.Ceil(math.Min(12, it.Glow)))
		for r := radius; r >= 1; r-- {
			alpha := it.Opacity * 0.055
			for _, d := range [][2]float64{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-0.7, -0.7}, {0.7, -0.7}, {-0.7, 0.7}, {0.7, 0.7}} {
				drawWindowsStringPass(graphics, font, u, tx+d[0]*float64(r), float64(y)+d[1]*float64(r), color, alpha)
			}
		}
	}
	if it.OutlineWidth > 0 {
		color := it.OutlineColor
		if color == "" {
			color = "#000000CC"
		}
		radius := int(math.Ceil(math.Min(8, it.OutlineWidth)))
		for r := radius; r >= 1; r-- {
			for _, d := range [][2]float64{{-1, 0}, {1, 0}, {0, -1}, {0, 1}, {-0.7, -0.7}, {0.7, -0.7}, {-0.7, 0.7}, {0.7, 0.7}} {
				drawWindowsStringPass(graphics, font, u, tx+d[0]*float64(r), float64(y)+d[1]*float64(r), color, it.Opacity)
			}
		}
	}
	drawWindowsStringPass(graphics, font, u, tx, float64(y), it.Color, it.Opacity)
}

func createWindowsFontFamily(name string) uintptr {
	u, err := syscall.UTF16FromString(name)
	if err != nil || len(u) <= 1 {
		return 0
	}
	var family uintptr
	status, _, _ := procGdipCreateFontFamily.Call(uintptr(unsafe.Pointer(&u[0])), 0, uintptr(unsafe.Pointer(&family)))
	if status != 0 {
		return 0
	}
	return family
}

func drawWindowsStringPass(graphics, font uintptr, text []uint16, x, y float64, color string, opacity float64) {
	brush := createWindowsBrush(color, opacity)
	if brush == 0 {
		return
	}
	defer procGdipDeleteBrush.Call(brush)
	layout := rectF{X: float32(x), Y: float32(y), Width: 4096, Height: 4096}
	procGdipDrawString.Call(graphics, uintptr(unsafe.Pointer(&text[0])), uintptr(len(text)-1), font, uintptr(unsafe.Pointer(&layout)), 0, brush)
}

func argb(r, g, b, a uint8) uint32 {
	return uint32(a)<<24 | uint32(r)<<16 | uint32(g)<<8 | uint32(b)
}

func mathFloat32Bits(v float32) uintptr { return uintptr(math.Float32bits(v)) }

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
