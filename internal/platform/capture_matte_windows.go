//go:build windows && !nocapture

package platform

import (
	"bytes"
	"image"
	"image/color"
	"image/jpeg"
	"unsafe"
)

const monitorDefaultToNearest = 2

type monitorInfo struct {
	Size    uint32
	Monitor rect
	Work    rect
	Flags   uint32
}

var (
	procMonitorFromWindow = user32.NewProc("MonitorFromWindow")
	procGetMonitorInfoW   = user32.NewProc("GetMonitorInfoW")
)

// CapturePrivacyMatteJPEG is deliberately separate from CaptureJPEG.
// CaptureJPEG remains the tight Elite-client frame used by Vision. The matte
// method is presentation-only: it places that verified Elite frame on a black
// monitor-sized canvas so Game View never exposes the desktop around a
// windowed Elite client.
func (w *windowsCapture) CapturePrivacyMatteJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	frame, info, err := w.CaptureJPEG(maxWidth, quality)
	if err != nil {
		return nil, CaptureInfo{}, err
	}
	if info.Blanked {
		return frame, info, nil
	}

	// CaptureJPEG already verified that Elite owns the foreground. Re-check the
	// same HWND before using its screen geometry so a focus/window race cannot
	// turn the matte into a desktop capture path.
	hwnd, _, _ := procGetForegroundWindow.Call()
	if hwnd == 0 {
		return encodeBlackJPEG(maxWidthOrDefault(maxWidth), blackHeight(maxWidthOrDefault(maxWidth)), quality, "no-foreground-window")
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	exe, exeErr := processBaseName(pid)
	if pid == 0 || exeErr != nil || !isEliteExecutable(exe) {
		return encodeBlackJPEG(maxWidthOrDefault(maxWidth), blackHeight(maxWidthOrDefault(maxWidth)), quality, "foreground-is-not-elite")
	}

	var client rect
	if r, _, e := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client))); r == 0 {
		return nil, CaptureInfo{}, syscallError("GetClientRect", e)
	}
	origin := point{0, 0}
	if r, _, e := procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&origin))); r == 0 {
		return nil, CaptureInfo{}, syscallError("ClientToScreen", e)
	}

	monitor, _, _ := procMonitorFromWindow.Call(hwnd, monitorDefaultToNearest)
	if monitor == 0 {
		// Tight Elite-only capture is still private; use it if monitor geometry is
		// unavailable rather than ever falling back to a desktop screenshot.
		return frame, info, nil
	}
	mi := monitorInfo{}
	mi.Size = uint32(unsafe.Sizeof(mi))
	if r, _, _ := procGetMonitorInfoW.Call(monitor, uintptr(unsafe.Pointer(&mi))); r == 0 {
		return frame, info, nil
	}

	monitorW := int(mi.Monitor.Right - mi.Monitor.Left)
	monitorH := int(mi.Monitor.Bottom - mi.Monitor.Top)
	clientW := int(client.Right - client.Left)
	clientH := int(client.Bottom - client.Top)
	if monitorW <= 0 || monitorH <= 0 || clientW <= 0 || clientH <= 0 {
		return frame, info, nil
	}

	outW := maxWidth
	if outW < 160 {
		outW = 960
	}
	if outW > monitorW {
		outW = monitorW
	}
	outH := int(float64(monitorH) * float64(outW) / float64(monitorW))
	if outH < 90 {
		outH = 90
	}

	src, err := jpeg.Decode(bytes.NewReader(frame))
	if err != nil {
		return nil, CaptureInfo{}, err
	}
	canvas := image.NewRGBA(image.Rect(0, 0, outW, outH))
	black := color.RGBA{0, 0, 0, 255}
	for y := 0; y < outH; y++ {
		for x := 0; x < outW; x++ {
			canvas.SetRGBA(x, y, black)
		}
	}

	sx := float64(outW) / float64(monitorW)
	sy := float64(outH) / float64(monitorH)
	dx := int(float64(int(origin.X)-int(mi.Monitor.Left)) * sx)
	dy := int(float64(int(origin.Y)-int(mi.Monitor.Top)) * sy)
	dw := int(float64(clientW) * sx)
	dh := int(float64(clientH) * sy)
	if dw < 1 || dh < 1 {
		return frame, info, nil
	}
	drawScaledNearest(canvas, src, dx, dy, dw, dh)

	if quality < 20 {
		quality = 20
	}
	if quality > 95 {
		quality = 95
	}
	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, canvas, &jpeg.Options{Quality: quality}); err != nil {
		return nil, CaptureInfo{}, err
	}
	info.Driver = w.Name() + "+privacy-matte"
	info.Width = outW
	info.Height = outH
	info.SourceWidth = monitorW
	info.SourceHeight = monitorH
	return buf.Bytes(), info, nil
}

func drawScaledNearest(dst *image.RGBA, src image.Image, dx, dy, dw, dh int) {
	if dst == nil || src == nil || dw <= 0 || dh <= 0 {
		return
	}
	sb := src.Bounds()
	sw, sh := sb.Dx(), sb.Dy()
	if sw <= 0 || sh <= 0 {
		return
	}
	for y := 0; y < dh; y++ {
		ty := dy + y
		if ty < dst.Rect.Min.Y || ty >= dst.Rect.Max.Y {
			continue
		}
		sy := sb.Min.Y + y*sh/dh
		for x := 0; x < dw; x++ {
			tx := dx + x
			if tx < dst.Rect.Min.X || tx >= dst.Rect.Max.X {
				continue
			}
			sx := sb.Min.X + x*sw/dw
			dst.Set(tx, ty, src.At(sx, sy))
		}
	}
}
