//go:build linux && !nocapture && cgo

package platform

import (
	"encoding/binary"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	vulkanCaptureMagic   uint32 = 0x3246434a // JCF2
	vulkanCaptureVersion uint32 = 2
	vulkanCaptureRGBA    uint32 = 1
	vulkanCaptureBGRA    uint32 = 2
)

type vulkanCleanCapture struct {
	mu          sync.Mutex
	nextSeq     uint64
	requests    uint64
	frames      uint64
	failures    uint64
	cacheHits   uint64
	lastLatency time.Duration
	lastError   string
	cached      GameFrame
}

type vulkanPreferredCapture struct {
	vulkan   *vulkanCleanCapture
	fallback CaptureDriver
}

func newVulkanPreferredCapture(fallback CaptureDriver) CaptureDriver {
	return &vulkanPreferredCapture{vulkan: &vulkanCleanCapture{}, fallback: fallback}
}

func (m *vulkanPreferredCapture) Name() string {
	if m.vulkan.Available() {
		return m.vulkan.Name()
	}
	return m.fallback.Name()
}

func (m *vulkanPreferredCapture) Available() bool {
	return m.vulkan.Available() || m.fallback.Available()
}

func (m *vulkanPreferredCapture) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	if m.vulkan.Available() {
		return m.vulkan.CaptureFrame(maxWidth, intent)
	}
	return m.fallback.CaptureFrame(maxWidth, intent)
}

func (m *vulkanPreferredCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	if m.vulkan.Available() {
		return m.vulkan.CaptureJPEG(maxWidth, quality)
	}
	return m.fallback.CaptureJPEG(maxWidth, quality)
}

func (m *vulkanPreferredCapture) CaptureStats() map[string]any {
	out := map[string]any{"activeBackend": m.Name()}
	out["vulkanPreHUD"] = m.vulkan.CaptureStats()
	if s, ok := m.fallback.(CaptureStatsProvider); ok {
		out["fallback"] = s.CaptureStats()
	}
	return out
}

func (v *vulkanCleanCapture) Name() string { return "linux-vulkan-prehud-elite-v2" }

func (v *vulkanCleanCapture) Available() bool {
	ok, _ := vulkanRendererReady()
	return ok
}

func (v *vulkanCleanCapture) requestPath() string {
	return filepath.Join(vulkanRuntimeDir(), "capture.request")
}
func (v *vulkanCleanCapture) framePath() string {
	return filepath.Join(vulkanRuntimeDir(), "capture.frame")
}

func (v *vulkanCleanCapture) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	if !linuxGamescopeEliteDirectlyFocused() {
		return blankGameFrame(v.Name(), 1280, 800,
			"Elite is not the directly presented Gamescope client; pre-HUD frame withheld"), nil
	}
	if !v.Available() {
		return GameFrame{}, fmt.Errorf("Vulkan pre-HUD capture is not connected")
	}

	v.mu.Lock()
	if intent == CaptureIntentLive && !v.cached.CapturedAt.IsZero() && time.Since(v.cached.CapturedAt) <= 90*time.Millisecond {
		if f, ok := linuxCachedFrameForWidth(v.cached, maxWidth); ok {
			v.cacheHits++
			v.mu.Unlock()
			return f, nil
		}
	}
	v.nextSeq++
	if v.nextSeq == 0 {
		v.nextSeq++
	}
	seq := uint64(time.Now().UnixNano()) ^ v.nextSeq
	v.requests++
	v.mu.Unlock()

	started := time.Now()
	if err := os.MkdirAll(vulkanRuntimeDir(), 0o700); err != nil {
		v.fail(err)
		return GameFrame{}, err
	}
	req := []byte(strconv.FormatUint(seq, 10) + "\n")
	reqPath := v.requestPath()
	tmp := fmt.Sprintf("%s.tmp.%d", reqPath, os.Getpid())
	if err := os.WriteFile(tmp, req, 0o600); err != nil {
		v.fail(err)
		return GameFrame{}, err
	}
	if err := os.Rename(tmp, reqPath); err != nil {
		_ = os.Remove(tmp)
		v.fail(err)
		return GameFrame{}, err
	}

	timeout := 900 * time.Millisecond
	if intent == CaptureIntentCalibration {
		timeout = 1200 * time.Millisecond
	}
	deadline := time.Now().Add(timeout)
	var lastErr error
	for time.Now().Before(deadline) {
		frame, gotSeq, err := readVulkanCaptureFrame(v.framePath())
		if err == nil && gotSeq == seq {
			if !linuxGamescopeEliteDirectlyFocused() {
				return blankGameFrame(v.Name(), frame.Width, frame.Height,
					"Elite focus changed while acquiring the pre-HUD frame; frame discarded"), nil
			}
			frame.Driver = v.Name()
			frame.CapturedAt = time.Now().UTC()
			frame.Verified = true
			frame.Foreground = true
			frame.SourceWidth = frame.Width
			frame.SourceHeight = frame.Height
			if linuxFrameLooksBlank(frame.Pix, frame.Width, frame.Height) {
				return blankGameFrame(v.Name(), frame.Width, frame.Height,
					"Vulkan pre-HUD frame was effectively black; Vision results withheld"), nil
			}
			v.mu.Lock()
			v.frames++
			v.lastLatency = time.Since(started)
			v.lastError = ""
			v.cached = frame
			v.mu.Unlock()
			scaled, _ := linuxCachedFrameForWidth(frame, maxWidth)
			return scaled, nil
		}
		if err != nil && !os.IsNotExist(err) {
			lastErr = err
		}
		time.Sleep(8 * time.Millisecond)
	}

	if lastErr == nil {
		lastErr = fmt.Errorf("timed out waiting for Vulkan pre-HUD frame sequence %d", seq)
	}
	v.fail(lastErr)
	return GameFrame{}, lastErr
}

func (v *vulkanCleanCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	frame, err := v.CaptureFrame(maxWidth, CaptureIntentLive)
	if err != nil {
		return nil, CaptureInfo{Driver: v.Name(), Blanked: true, Reason: err.Error()}, err
	}
	return EncodeGameFrameJPEG(frame, quality)
}

func (v *vulkanCleanCapture) fail(err error) {
	v.mu.Lock()
	defer v.mu.Unlock()
	v.failures++
	if err != nil {
		v.lastError = err.Error()
	}
}

func (v *vulkanCleanCapture) CaptureStats() map[string]any {
	v.mu.Lock()
	defer v.mu.Unlock()
	ready, detail := vulkanRendererReady()
	age := int64(-1)
	if !v.cached.CapturedAt.IsZero() {
		age = time.Since(v.cached.CapturedAt).Milliseconds()
	}
	return map[string]any{
		"driver":           v.Name(),
		"transport":        "vulkan-pre-hud-runtime-file",
		"ready":            ready,
		"renderer":         detail,
		"requests":         v.requests,
		"frames":           v.frames,
		"failures":         v.failures,
		"cacheHits":        v.cacheHits,
		"cachedFrameAgeMs": age,
		"lastLatencyMs":    v.lastLatency.Milliseconds(),
		"lastError":        v.lastError,
		"eliteOnly":        true,
		"preHUD":           true,
	}
}

func readVulkanCaptureFrame(path string) (GameFrame, uint64, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return GameFrame{}, 0, err
	}
	if len(b) < 32 {
		return GameFrame{}, 0, fmt.Errorf("Vulkan capture frame header is truncated")
	}
	magic := binary.LittleEndian.Uint32(b[0:4])
	version := binary.LittleEndian.Uint32(b[4:8])
	seq := binary.LittleEndian.Uint64(b[8:16])
	width := binary.LittleEndian.Uint32(b[16:20])
	height := binary.LittleEndian.Uint32(b[20:24])
	format := binary.LittleEndian.Uint32(b[24:28])
	payload := binary.LittleEndian.Uint32(b[28:32])
	if magic != vulkanCaptureMagic || version != vulkanCaptureVersion {
		return GameFrame{}, seq, fmt.Errorf("Vulkan capture frame has invalid magic/version")
	}
	if width == 0 || height == 0 || width > 8192 || height > 8192 || payload != width*height*4 {
		return GameFrame{}, seq, fmt.Errorf("Vulkan capture frame dimensions are invalid")
	}
	if len(b) != 32+int(payload) {
		return GameFrame{}, seq, fmt.Errorf("Vulkan capture frame payload is truncated")
	}
	pix := append([]byte(nil), b[32:]...)
	switch format {
	case vulkanCaptureRGBA:
	case vulkanCaptureBGRA:
		for i := 0; i+3 < len(pix); i += 4 {
			pix[i], pix[i+2] = pix[i+2], pix[i]
		}
	default:
		return GameFrame{}, seq, fmt.Errorf("Vulkan capture frame uses unsupported pixel format %d", format)
	}
	return GameFrame{
		Pix:          pix,
		Width:        int(width),
		Height:       int(height),
		Stride:       int(width) * 4,
		Format:       PixelFormatRGBA8,
		SourceWidth:  int(width),
		SourceHeight: int(height),
		Verified:     true,
		Foreground:   true,
	}, seq, nil
}

func vulkanCaptureReadyDetail() string {
	_, detail := vulkanRendererReady()
	return strings.TrimSpace(detail)
}
