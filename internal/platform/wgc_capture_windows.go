//go:build windows && !nocapture

package platform

import (
	"fmt"
	"math"
	"runtime"
	"strings"
	"syscall"
	"time"
	"unsafe"

	"github.com/TKMAX777/winapi/dx11"
	"github.com/TKMAX777/winapi/winrt"
	"github.com/go-ole/go-ole"
	"github.com/lxn/win"
)

type wgcCaptureRequest struct {
	hwnd     uintptr
	maxWidth int
	timeout  time.Duration
	reply    chan wgcCaptureResult
}

type wgcCaptureResult struct {
	frame GameFrame
	err   error
}

type wgcCaptureWorker struct {
	requests chan wgcCaptureRequest
}

func newWGCCaptureWorker() *wgcCaptureWorker {
	w := &wgcCaptureWorker{requests: make(chan wgcCaptureRequest)}
	go w.run()
	return w
}

func (w *wgcCaptureWorker) Capture(hwnd uintptr, maxWidth int, timeout time.Duration) (GameFrame, error) {
	if hwnd == 0 {
		return GameFrame{}, fmt.Errorf("WGC target HWND is unavailable")
	}
	if timeout < 150*time.Millisecond {
		timeout = 150 * time.Millisecond
	}
	req := wgcCaptureRequest{
		hwnd:     hwnd,
		maxWidth: maxWidth,
		timeout:  timeout,
		reply:    make(chan wgcCaptureResult, 1),
	}
	select {
	case w.requests <- req:
	case <-time.After(timeout + 500*time.Millisecond):
		return GameFrame{}, fmt.Errorf("Windows Graphics Capture worker is not responding")
	}
	select {
	case result := <-req.reply:
		return result.frame, result.err
	case <-time.After(timeout + 750*time.Millisecond):
		return GameFrame{}, fmt.Errorf("Windows Graphics Capture timed out")
	}
}

func (w *wgcCaptureWorker) run() {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	initErr := winrt.RoInitialize(winrt.RO_INIT_MULTITHREADED)
	if initErr == nil {
		defer winrt.RoUninitialize()
	}

	var source *wgcWindowSource
	defer func() {
		if source != nil {
			source.Close()
		}
	}()

	for req := range w.requests {
		if initErr != nil {
			req.reply <- wgcCaptureResult{err: fmt.Errorf("RoInitialize: %w", initErr)}
			continue
		}

		cw, ch, geomErr := wgcClientSize(req.hwnd)
		if geomErr != nil {
			req.reply <- wgcCaptureResult{err: geomErr}
			continue
		}

		if source == nil || source.hwnd != req.hwnd || source.clientWidth != cw || source.clientHeight != ch {
			if source != nil {
				source.Close()
				source = nil
			}
			source, geomErr = newWGCWindowSource(req.hwnd, cw, ch)
			if geomErr != nil {
				req.reply <- wgcCaptureResult{err: geomErr}
				continue
			}
		}

		frame, err := source.CaptureCanonicalClient(req.maxWidth, req.timeout)
		if err != nil && !strings.Contains(strings.ToLower(err.Error()), "timed out") {
			// A stale WinRT session can occur after display/device changes. Rebuild
			// the HWND source once. There is still no desktop capture fallback.
			source.Close()
			source = nil
			source, geomErr = newWGCWindowSource(req.hwnd, cw, ch)
			if geomErr == nil {
				frame, err = source.CaptureCanonicalClient(req.maxWidth, req.timeout)
			}
		}
		req.reply <- wgcCaptureResult{frame: frame, err: err}
	}
}

type wgcWindowSource struct {
	hwnd         uintptr
	clientWidth  int
	clientHeight int

	device    *winrt.IDirect3DDevice
	deviceDX  *dx11.ID3D11Device
	item      *winrt.IGraphicsCaptureItem
	framePool *winrt.IDirect3D11CaptureFramePool
	session   *winrt.IGraphicsCaptureSession

	skippedFirst bool
}

func newWGCWindowSource(hwnd uintptr, clientWidth, clientHeight int) (*wgcWindowSource, error) {
	s := &wgcWindowSource{hwnd: hwnd, clientWidth: clientWidth, clientHeight: clientHeight}
	ok := false
	defer func() {
		if !ok {
			s.Close()
		}
	}()

	featureLevels := []dx11.D3D_FEATURE_LEVEL{
		dx11.D3D_FEATURE_LEVEL_11_0,
		dx11.D3D_FEATURE_LEVEL_10_1,
		dx11.D3D_FEATURE_LEVEL_10_0,
		dx11.D3D_FEATURE_LEVEL_9_3,
		dx11.D3D_FEATURE_LEVEL_9_2,
		dx11.D3D_FEATURE_LEVEL_9_1,
	}
	if err := dx11.D3D11CreateDevice(
		nil,
		dx11.D3D_DRIVER_TYPE_HARDWARE,
		0,
		dx11.D3D11_CREATE_DEVICE_BGRA_SUPPORT,
		&featureLevels[0],
		len(featureLevels),
		dx11.D3D11_SDK_VERSION,
		&s.deviceDX,
		nil,
		nil,
	); err != nil {
		return nil, fmt.Errorf("D3D11CreateDevice: %w", err)
	}

	var dxgiDevice *dx11.IDXGIDevice
	if err := s.deviceDX.PutQueryInterface(dx11.IDXGIDeviceID, &dxgiDevice); err != nil {
		return nil, fmt.Errorf("query IDXGIDevice: %w", err)
	}
	defer dxgiDevice.Release()

	var deviceRT *ole.IInspectable
	if err := dx11.CreateDirect3D11DeviceFromDXGIDevice(dxgiDevice, &deviceRT); err != nil {
		return nil, fmt.Errorf("CreateDirect3D11DeviceFromDXGIDevice: %w", err)
	}
	defer deviceRT.Release()

	if err := deviceRT.PutQueryInterface(winrt.IDirect3DDeviceID, &s.device); err != nil {
		return nil, fmt.Errorf("query IDirect3DDevice: %w", err)
	}

	factory, err := ole.RoGetActivationFactory(winrt.GraphicsCaptureItemClass, winrt.IGraphicsCaptureItemInteropID)
	if err != nil {
		return nil, fmt.Errorf("GraphicsCaptureItem activation factory: %w", err)
	}
	defer factory.Release()

	var interop *winrt.IGraphicsCaptureItemInterop
	if err := factory.PutQueryInterface(winrt.IGraphicsCaptureItemInteropID, &interop); err != nil {
		return nil, fmt.Errorf("query IGraphicsCaptureItemInterop: %w", err)
	}
	defer interop.Release()

	var itemInspectable *ole.IInspectable
	if err := interop.CreateForWindow(win.HWND(hwnd), winrt.IGraphicsCaptureItemID, &itemInspectable); err != nil {
		return nil, fmt.Errorf("CreateForWindow(Elite HWND): %w", err)
	}
	defer itemInspectable.Release()

	if err := itemInspectable.PutQueryInterface(winrt.IGraphicsCaptureItemID, &s.item); err != nil {
		return nil, fmt.Errorf("query IGraphicsCaptureItem: %w", err)
	}

	size, err := s.item.Size()
	if err != nil {
		return nil, fmt.Errorf("GraphicsCaptureItem.Size: %w", err)
	}
	if size == nil || size.Width <= 0 || size.Height <= 0 {
		return nil, fmt.Errorf("Windows Graphics Capture returned invalid window size")
	}

	factoryPool, err := ole.RoGetActivationFactory(winrt.Direct3D11CaptureFramePoolClass, winrt.IDirect3D11CaptureFramePoolStaticsID)
	if err != nil {
		return nil, fmt.Errorf("Direct3D11CaptureFramePool activation factory: %w", err)
	}
	defer factoryPool.Release()

	var poolStatics *winrt.IDirect3D11CaptureFramePoolStatics2
	if err := factoryPool.PutQueryInterface(winrt.IDirect3D11CaptureFramePoolStatics2ID, &poolStatics); err != nil {
		return nil, fmt.Errorf("query IDirect3D11CaptureFramePoolStatics2: %w", err)
	}
	defer poolStatics.Release()

	// TKMAX777/winapi v2023 packs SizeInt32 reversed on amd64. Supplying the
	// fields reversed here produces the correct Width/Height ABI value.
	swapped := &winrt.SizeInt32{Width: size.Height, Height: size.Width}
	s.framePool, err = poolStatics.CreateFreeThreaded(
		s.device,
		winrt.DirectXPixelFormat_B8G8R8A8UIntNormalized,
		2,
		swapped,
	)
	if err != nil {
		return nil, fmt.Errorf("CreateFreeThreaded: %w", err)
	}

	s.session, err = s.framePool.CreateCaptureSession(s.item)
	if err != nil {
		return nil, fmt.Errorf("CreateCaptureSession: %w", err)
	}
	if err := s.session.StartCapture(); err != nil {
		return nil, fmt.Errorf("StartCapture: %w", err)
	}

	ok = true
	return s, nil
}

func (s *wgcWindowSource) CaptureCanonicalClient(maxWidth int, timeout time.Duration) (GameFrame, error) {
	if s == nil || s.framePool == nil {
		return GameFrame{}, fmt.Errorf("Windows Graphics Capture session is unavailable")
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		frame, err := s.framePool.TryGetNextFrame()
		if err != nil {
			return GameFrame{}, fmt.Errorf("TryGetNextFrame: %w", err)
		}
		if frame == nil {
			time.Sleep(8 * time.Millisecond)
			continue
		}

		if !s.skippedFirst {
			s.skippedFirst = true
			wgcCloseFrame(frame)
			time.Sleep(12 * time.Millisecond)
			continue
		}

		out, err := s.frameToCanonicalClient(frame, maxWidth)
		wgcCloseFrame(frame)
		if err != nil {
			return GameFrame{}, err
		}
		return out, nil
	}
	return GameFrame{}, fmt.Errorf("Windows Graphics Capture timed out waiting for an Elite window frame")
}

func (s *wgcWindowSource) frameToCanonicalClient(frame *winrt.IDirect3D11CaptureFrame, maxWidth int) (GameFrame, error) {
	surface, err := wgcFrameSurface(frame)
	if err != nil {
		return GameFrame{}, err
	}
	defer surface.Release()

	var access *wgcDXGIInterfaceAccess
	if err := surface.PutQueryInterface(wgcDXGIInterfaceAccessGUID, &access); err != nil {
		return GameFrame{}, fmt.Errorf("query IDirect3DDxgiInterfaceAccess: %w", err)
	}
	defer access.Release()

	var gpuTexture *wgcID3D11Texture2D
	if err := access.GetInterface(wgcID3D11Texture2DGUID, unsafe.Pointer(&gpuTexture)); err != nil {
		return GameFrame{}, fmt.Errorf("get ID3D11Texture2D: %w", err)
	}
	defer gpuTexture.Release()

	desc := gpuTexture.Desc()
	frameW, frameH := int(desc.Width), int(desc.Height)
	if frameW <= 0 || frameH <= 0 {
		return GameFrame{}, fmt.Errorf("WGC frame has invalid dimensions %dx%d", frameW, frameH)
	}

	x, y, cropW, cropH, err := wgcClientCrop(s.hwnd, frameW, frameH)
	if err != nil {
		return GameFrame{}, err
	}

	stagingDesc := desc
	stagingDesc.Width = uint32(cropW)
	stagingDesc.Height = uint32(cropH)
	stagingDesc.Usage = wgcD3D11UsageStaging
	stagingDesc.CPUAccessFlags = wgcD3D11CPUAccessRead
	stagingDesc.BindFlags = 0
	stagingDesc.MiscFlags = 0
	stagingDesc.MipLevels = 1

	staging, err := wgcCreateTexture2D(s.deviceDX, &stagingDesc)
	if err != nil {
		return GameFrame{}, fmt.Errorf("create WGC staging texture: %w", err)
	}
	defer staging.Release()

	ctx := s.deviceDX.GetImmediateContext()
	if ctx == nil {
		return GameFrame{}, fmt.Errorf("D3D11 immediate context is unavailable")
	}
	defer ctx.Release()

	box := wgcD3D11Box{
		Left: uint32(x), Top: uint32(y), Front: 0,
		Right: uint32(x + cropW), Bottom: uint32(y + cropH), Back: 1,
	}
	wgcCopySubresourceRegion(ctx, staging, 0, 0, 0, 0, gpuTexture, 0, &box)

	mapped, err := wgcMap(ctx, staging)
	if err != nil {
		return GameFrame{}, fmt.Errorf("map WGC staging texture: %w", err)
	}
	defer wgcUnmap(ctx, staging)

	rowPitch := int(mapped.RowPitch)
	if mapped.PData == nil || rowPitch < cropW*4 {
		return GameFrame{}, fmt.Errorf("WGC staging texture returned invalid mapped memory")
	}
	raw := unsafe.Slice((*byte)(mapped.PData), rowPitch*cropH)
	pix := make([]byte, cropW*cropH*4)
	for row := 0; row < cropH; row++ {
		srcRow := row * rowPitch
		dstRow := row * cropW * 4
		for col := 0; col < cropW; col++ {
			si := srcRow + col*4
			di := dstRow + col*4
			pix[di+0] = raw[si+2]
			pix[di+1] = raw[si+1]
			pix[di+2] = raw[si+0]
			pix[di+3] = 0xff
		}
	}

	outPix, outW, outH := wgcScaleRGBA(pix, cropW, cropH, maxWidth)
	return GameFrame{
		Pix:          outPix,
		Width:        outW,
		Height:       outH,
		Stride:       outW * 4,
		Format:       PixelFormatRGBA8,
		CapturedAt:   time.Now().UTC(),
		SourceWidth:  cropW,
		SourceHeight: cropH,
	}, nil
}

type wgcDirect3DSurface struct {
	ole.IInspectable
}

func wgcFrameSurface(frame *winrt.IDirect3D11CaptureFrame) (*wgcDirect3DSurface, error) {
	var surface *wgcDirect3DSurface
	r1, _, _ := syscall.SyscallN(
		frame.VTable().Surface,
		uintptr(unsafe.Pointer(frame)),
		uintptr(unsafe.Pointer(&surface)),
	)
	if r1 != 0 {
		return nil, ole.NewError(r1)
	}
	if surface == nil {
		return nil, fmt.Errorf("WGC frame did not provide a Direct3D surface")
	}
	return surface, nil
}

func wgcCloseFrame(frame *winrt.IDirect3D11CaptureFrame) {
	if frame == nil {
		return
	}
	var closable *winrt.IClosable
	if err := frame.PutQueryInterface(winrt.IClosableID, &closable); err == nil && closable != nil {
		_ = closable.Close()
		closable.Release()
	}
	frame.Release()
}

func (s *wgcWindowSource) Close() {
	if s == nil {
		return
	}
	if s.session != nil {
		var closable *winrt.IClosable
		if err := s.session.PutQueryInterface(winrt.IClosableID, &closable); err == nil && closable != nil {
			_ = closable.Close()
			closable.Release()
		}
		s.session.Release()
		s.session = nil
	}
	if s.framePool != nil {
		var closable *winrt.IClosable
		if err := s.framePool.PutQueryInterface(winrt.IClosableID, &closable); err == nil && closable != nil {
			_ = closable.Close()
			closable.Release()
		}
		s.framePool.Release()
		s.framePool = nil
	}
	if s.item != nil {
		s.item.Release()
		s.item = nil
	}
	if s.device != nil {
		s.device.Release()
		s.device = nil
	}
	if s.deviceDX != nil {
		s.deviceDX.Release()
		s.deviceDX = nil
	}
}

var procGetWindowRectWGC = user32.NewProc("GetWindowRect")

func wgcClientSize(hwnd uintptr) (int, int, error) {
	var client rect
	if r, _, e := procGetClientRect.Call(hwnd, uintptr(unsafe.Pointer(&client))); r == 0 {
		return 0, 0, syscallError("GetClientRect", e)
	}
	w := int(client.Right - client.Left)
	h := int(client.Bottom - client.Top)
	if w <= 0 || h <= 0 {
		return 0, 0, fmt.Errorf("Elite client area is %dx%d", w, h)
	}
	return w, h, nil
}

func wgcClientCrop(hwnd uintptr, frameW, frameH int) (int, int, int, int, error) {
	clientW, clientH, err := wgcClientSize(hwnd)
	if err != nil {
		return 0, 0, 0, 0, err
	}

	// Borderless/windowed Elite often gives WGC exactly the client dimensions.
	if absInt(frameW-clientW) <= 3 && absInt(frameH-clientH) <= 3 {
		return 0, 0, frameW, frameH, nil
	}

	var wr rect
	if r, _, e := procGetWindowRectWGC.Call(hwnd, uintptr(unsafe.Pointer(&wr))); r == 0 {
		return 0, 0, 0, 0, syscallError("GetWindowRect", e)
	}
	windowW := int(wr.Right - wr.Left)
	windowH := int(wr.Bottom - wr.Top)
	if windowW <= 0 || windowH <= 0 {
		return 0, 0, 0, 0, fmt.Errorf("Elite window has invalid bounds")
	}

	origin := point{0, 0}
	if r, _, e := procClientToScreen.Call(hwnd, uintptr(unsafe.Pointer(&origin))); r == 0 {
		return 0, 0, 0, 0, syscallError("ClientToScreen", e)
	}

	// GetWindowRect and ClientToScreen share the caller's coordinate space.
	// Scale their relative offset into WGC's pixel dimensions so DPI
	// virtualization cannot shift the crop.
	scaleX := float64(frameW) / float64(windowW)
	scaleY := float64(frameH) / float64(windowH)
	x := int(math.Round(float64(int(origin.X)-int(wr.Left)) * scaleX))
	y := int(math.Round(float64(int(origin.Y)-int(wr.Top)) * scaleY))
	w := int(math.Round(float64(clientW) * scaleX))
	h := int(math.Round(float64(clientH) * scaleY))

	if x < 0 {
		w += x
		x = 0
	}
	if y < 0 {
		h += y
		y = 0
	}
	if x+w > frameW {
		w = frameW - x
	}
	if y+h > frameH {
		h = frameH - y
	}
	if w <= 0 || h <= 0 {
		return 0, 0, 0, 0, fmt.Errorf("could not isolate Elite client area inside WGC frame")
	}
	return x, y, w, h, nil
}

func wgcScaleRGBA(src []byte, sw, sh, maxWidth int) ([]byte, int, int) {
	if sw <= 0 || sh <= 0 || len(src) < sw*sh*4 {
		return src, sw, sh
	}
	if maxWidth < 160 {
		maxWidth = 960
	}
	if maxWidth >= sw {
		return src, sw, sh
	}
	dw := maxWidth
	dh := int(math.Round(float64(sh) * float64(dw) / float64(sw)))
	if dh < 1 {
		dh = 1
	}
	dst := make([]byte, dw*dh*4)
	for y := 0; y < dh; y++ {
		sy := y * sh / dh
		for x := 0; x < dw; x++ {
			sx := x * sw / dw
			si := (sy*sw + sx) * 4
			di := (y*dw + x) * 4
			copy(dst[di:di+4], src[si:si+4])
		}
	}
	return dst, dw, dh
}

func absInt(v int) int {
	if v < 0 {
		return -v
	}
	return v
}
