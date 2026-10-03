//go:build windows

package platform

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"
	"syscall"
	"unsafe"
)

const (
	biRGB           = 0
	dibRGBColors    = 0
	srcCopy         = 0x00CC0020
	stretchHalftone = 4
)

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }
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
type rgbQuad struct{ Blue, Green, Red, Reserved byte }
type bitmapInfo struct {
	Header bitmapInfoHeader
	Colors [1]rgbQuad
}

type windowsCapture struct{}

var (
	gdi32                  = syscall.NewLazyDLL("gdi32.dll")
	procGetDC              = user32.NewProc("GetDC")
	procReleaseDC          = user32.NewProc("ReleaseDC")
	procGetClientRect      = user32.NewProc("GetClientRect")
	procClientToScreen     = user32.NewProc("ClientToScreen")
	procCreateCompatibleDC = gdi32.NewProc("CreateCompatibleDC")
	procDeleteDC           = gdi32.NewProc("DeleteDC")
	procCreateDIBSection   = gdi32.NewProc("CreateDIBSection")
	procSelectObject       = gdi32.NewProc("SelectObject")
	procDeleteObject       = gdi32.NewProc("DeleteObject")
	procStretchBlt         = gdi32.NewProc("StretchBlt")
	procSetStretchBltMode  = gdi32.NewProc("SetStretchBltMode")
)

func newCaptureDriver() CaptureDriver     { return &windowsCapture{} }
func (w *windowsCapture) Name() string    { return "windows-gdi-visible-region" }
func (w *windowsCapture) Available() bool { return true }

func (w *windowsCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	hwnd, _, err := findEliteWindow()
	if err != nil {
		return nil, CaptureInfo{}, err
	}
	var rc rect
	if r, _, e := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&rc))); r == 0 {
		return nil, CaptureInfo{}, syscallError("GetClientRect", e)
	}
	p := point{0, 0}
	if r, _, e := procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&p))); r == 0 {
		return nil, CaptureInfo{}, syscallError("ClientToScreen", e)
	}
	sw, sh := int(rc.Right-rc.Left), int(rc.Bottom-rc.Top)
	if sw <= 0 || sh <= 0 {
		return nil, CaptureInfo{}, fmt.Errorf("Elite client area is %dx%d", sw, sh)
	}
	if maxWidth < 160 {
		maxWidth = 160
	}
	if maxWidth > sw {
		maxWidth = sw
	}
	dw := maxWidth
	dh := int(float64(sh) * float64(dw) / float64(sw))
	if dh < 90 {
		dh = 90
	}
	if quality < 20 {
		quality = 20
	}
	if quality > 95 {
		quality = 95
	}

	screenDC, _, e := procGetDC.Call(0)
	if screenDC == 0 {
		return nil, CaptureInfo{}, syscallError("GetDC", e)
	}
	defer procReleaseDC.Call(0, screenDC)
	memDC, _, e := procCreateCompatibleDC.Call(screenDC)
	if memDC == 0 {
		return nil, CaptureInfo{}, syscallError("CreateCompatibleDC", e)
	}
	defer procDeleteDC.Call(memDC)

	bmi := bitmapInfo{}
	bmi.Header.Size = uint32(unsafe.Sizeof(bmi.Header))
	bmi.Header.Width = int32(dw)
	bmi.Header.Height = -int32(dh)
	bmi.Header.Planes = 1
	bmi.Header.BitCount = 32
	bmi.Header.Compression = biRGB
	var bits unsafe.Pointer
	hbmp, _, e := procCreateDIBSection.Call(screenDC, uintptr(unsafe.Pointer(&bmi)), dibRGBColors, uintptr(unsafe.Pointer(&bits)), 0, 0)
	if hbmp == 0 || bits == nil {
		return nil, CaptureInfo{}, syscallError("CreateDIBSection", e)
	}
	defer procDeleteObject.Call(hbmp)
	old, _, _ := procSelectObject.Call(memDC, hbmp)
	defer procSelectObject.Call(memDC, old)
	procSetStretchBltMode.Call(memDC, stretchHalftone)
	if r, _, e := procStretchBlt.Call(memDC, 0, 0, uintptr(dw), uintptr(dh), screenDC, uintptr(int32(p.X)), uintptr(int32(p.Y)), uintptr(sw), uintptr(sh), srcCopy); r == 0 {
		return nil, CaptureInfo{}, syscallError("StretchBlt", e)
	}

	n := dw * dh * 4
	raw := unsafe.Slice((*byte)(bits), n)
	img := image.NewRGBA(image.Rect(0, 0, dw, dh))
	for y := 0; y < dh; y++ {
		for x := 0; x < dw; x++ {
			i := (y*dw + x) * 4
			o := y*img.Stride + x*4
			img.Pix[o+0] = raw[i+2]
			img.Pix[o+1] = raw[i+1]
			img.Pix[o+2] = raw[i+0]
			img.Pix[o+3] = 0xff
		}
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality}); err != nil {
		return nil, CaptureInfo{}, err
	}
	return buf.Bytes(), CaptureInfo{Driver: w.Name(), Width: dw, Height: dh, SourceWidth: sw, SourceHeight: sh}, nil
}

func syscallError(name string, e error) error {
	if errno, ok := e.(syscall.Errno); ok && errno == 0 {
		return fmt.Errorf("%s failed", name)
	}
	if e != nil {
		return fmt.Errorf("%s failed: %w", name, e)
	}
	return fmt.Errorf("%s failed", name)
}
