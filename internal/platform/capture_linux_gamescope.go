//go:build linux && !nocapture && cgo

package platform

import (
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const pipewireCaptureDriverName = "linux-gamescope-pipewire-elite-v1"
const pipewireFrameMagic uint32 = 0x3152464a

type pipewireCapture struct {
	mu            sync.Mutex
	cmd           *exec.Cmd
	latest        GameFrame
	latestSeq     uint32
	starts        uint64
	restarts      uint64
	frames        uint64
	cacheHits     uint64
	focusRejects  uint64
	failures      uint64
	blankFrames   uint64
	lastError     string
	lastFrameAt   time.Time
	helperStarted time.Time
}

type linuxCaptureMux struct {
	pipewire *pipewireCapture
	x11      *linuxCapture
}

func newLinuxCaptureDriver() CaptureDriver {
	return &linuxCaptureMux{
		pipewire: &pipewireCapture{},
		x11:      &linuxCapture{},
	}
}

func (m *linuxCaptureMux) pipewireActive() bool {
	return m.pipewire != nil && m.pipewire.Available()
}

func (m *linuxCaptureMux) Name() string {
	if m.pipewireActive() {
		return m.pipewire.Name()
	}
	return m.x11.Name()
}

func (m *linuxCaptureMux) Available() bool {
	return m.pipewireActive() || m.x11.Available()
}

func (m *linuxCaptureMux) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	if m.pipewireActive() {
		return m.pipewire.CaptureFrame(maxWidth, intent)
	}
	return m.x11.CaptureFrame(maxWidth, intent)
}

func (m *linuxCaptureMux) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	if m.pipewireActive() {
		return m.pipewire.CaptureJPEG(maxWidth, quality)
	}
	return m.x11.CaptureJPEG(maxWidth, quality)
}

func (m *linuxCaptureMux) CaptureStats() map[string]any {
	if m.pipewireActive() {
		return map[string]any{
			"activeBackend": "gamescope-pipewire",
			"pipewire":      m.pipewire.CaptureStats(),
			"x11Fallback":   m.x11.CaptureStats(),
		}
	}
	return map[string]any{
		"activeBackend": "x11",
		"x11Fallback":   m.x11.CaptureStats(),
	}
}

func (p *pipewireCapture) Name() string { return pipewireCaptureDriverName }

func gamescopeRuntimeDir() string {
	if v := strings.TrimSpace(os.Getenv("XDG_RUNTIME_DIR")); v != "" {
		return v
	}
	return fmt.Sprintf("/run/user/%d", os.Getuid())
}

func gamescopeSocketName() string {
	if v := strings.TrimSpace(os.Getenv("GAMESCOPE_WAYLAND_DISPLAY")); v != "" {
		return filepath.Base(v)
	}
	return "gamescope-0"
}

func gamescopeSocketPath() string {
	return filepath.Join(gamescopeRuntimeDir(), gamescopeSocketName())
}

func gamescopeCaptureHelperPath() string {
	if v := strings.TrimSpace(os.Getenv("JACOB_GAMESCOPE_CAPTURE_HELPER")); v != "" {
		return v
	}
	if exe, err := os.Executable(); err == nil {
		p := filepath.Join(filepath.Dir(exe), "jacob-gamescope-stream")
		if st, err := os.Stat(p); err == nil && st.Mode().IsRegular() {
			return p
		}
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		return filepath.Join(home, ".local", "share", "JACoB", "jacob-gamescope-stream")
	}
	return ""
}

func executableFile(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode().IsRegular() && st.Mode().Perm()&0o111 != 0
}

func (p *pipewireCapture) Available() bool {
	helper := gamescopeCaptureHelperPath()
	if !executableFile(helper) {
		return false
	}
	st, err := os.Stat(gamescopeSocketPath())
	return err == nil && st.Mode()&os.ModeSocket != 0
}

func linuxGamescopeEliteDirectlyFocused() bool {
	displays, probes := linuxFocusSnapshot()
	if len(probes) == 0 {
		return false
	}
	for _, probe := range probes {
		if probe.focusedGFX == 359320 {
			_, _, ok := findEliteFocusedWindowTarget(probe.window, displays)
			return ok
		}
	}
	for _, probe := range probes {
		if probe.focusedGFX != 0 {
			return false
		}
	}
	for _, probe := range probes {
		if probe.focusedApp == 359320 {
			_, _, ok := findEliteFocusedWindowTarget(probe.window, displays)
			return ok
		}
	}
	return false
}

func (p *pipewireCapture) ensureRunning() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd != nil && p.cmd.Process != nil {
		return nil
	}

	helper := gamescopeCaptureHelperPath()
	if !executableFile(helper) {
		return fmt.Errorf("Gamescope PipeWire helper is unavailable at %s", helper)
	}

	cmd := exec.Command(
		helper,
		"359320",
		"1280",
		"800",
		"10",
		gamescopeSocketName(),
	)
	cmd.Env = append(os.Environ(),
		"GAMESCOPE_WAYLAND_DISPLAY="+gamescopeSocketName(),
		"WAYLAND_DISPLAY="+gamescopeSocketName(),
	)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open Gamescope PipeWire helper stdout: %w", err)
	}
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("start Gamescope PipeWire helper: %w", err)
	}

	if p.starts > 0 {
		p.restarts++
	}
	p.starts++
	p.cmd = cmd
	p.helperStarted = time.Now().UTC()
	p.lastError = ""

	go p.readLoop(cmd, stdout)
	go p.waitLoop(cmd)
	return nil
}

func (p *pipewireCapture) waitLoop(cmd *exec.Cmd) {
	err := cmd.Wait()
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.cmd == cmd {
		p.cmd = nil
		if err != nil {
			p.lastError = err.Error()
			p.failures++
		}
	}
}

func (p *pipewireCapture) readLoop(cmd *exec.Cmd, r io.Reader) {
	header := make([]byte, 20)
	for {
		if _, err := io.ReadFull(r, header); err != nil {
			return
		}
		magic := binary.LittleEndian.Uint32(header[0:4])
		width := binary.LittleEndian.Uint32(header[4:8])
		height := binary.LittleEndian.Uint32(header[8:12])
		payload := binary.LittleEndian.Uint32(header[12:16])
		seq := binary.LittleEndian.Uint32(header[16:20])

		if magic != pipewireFrameMagic ||
			width == 0 || height == 0 ||
			width > 8192 || height > 8192 ||
			payload != width*height*4 ||
			payload > 128*1024*1024 {
			p.mu.Lock()
			p.failures++
			p.lastError = "invalid frame header from Gamescope PipeWire helper"
			if p.cmd == cmd && cmd.Process != nil {
				_ = cmd.Process.Kill()
			}
			p.mu.Unlock()
			return
		}

		pix := make([]byte, int(payload))
		if _, err := io.ReadFull(r, pix); err != nil {
			return
		}

		now := time.Now().UTC()
		frame := GameFrame{
			Pix:          pix,
			Width:        int(width),
			Height:       int(height),
			Stride:       int(width) * 4,
			Format:       PixelFormatRGBA8,
			CapturedAt:   now,
			Driver:       p.Name(),
			SourceWidth:  int(width),
			SourceHeight: int(height),
			Verified:     true,
			Foreground:   true,
		}

		p.mu.Lock()
		p.latest = frame
		p.latestSeq = seq
		p.frames++
		p.lastFrameAt = now
		p.mu.Unlock()
	}
}

func (p *pipewireCapture) focusBlank(reason string) GameFrame {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.focusRejects++
	w, h := 1280, 800
	if p.latest.SourceWidth > 0 && p.latest.SourceHeight > 0 {
		w, h = p.latest.SourceWidth, p.latest.SourceHeight
	}
	return blankGameFrame(p.Name(), w, h, reason)
}

func (p *pipewireCapture) latestFrame(maxAge time.Duration) (GameFrame, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.latest.CapturedAt.IsZero() || time.Since(p.latest.CapturedAt) > maxAge {
		return GameFrame{}, false
	}
	return p.latest, true
}

func (p *pipewireCapture) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	if !linuxGamescopeEliteDirectlyFocused() {
		return p.focusBlank(
			"Elite is not the directly presented Gamescope CLIENT; PipeWire frame withheld while launcher, Steam UI, idle/saver UI, or another surface may be active"), nil
	}

	if err := p.ensureRunning(); err != nil {
		p.mu.Lock()
		p.failures++
		p.lastError = err.Error()
		p.mu.Unlock()
		return GameFrame{}, err
	}

	maxAge := 450 * time.Millisecond
	if intent == CaptureIntentCalibration {
		maxAge = 700 * time.Millisecond
	}
	if f, ok := p.latestFrame(maxAge); ok {
		if !linuxGamescopeEliteDirectlyFocused() {
			return p.focusBlank("Elite focus changed while selecting the PipeWire frame; frame discarded"), nil
		}
		if linuxFrameLooksBlank(f.Pix, f.Width, f.Height) {
			p.mu.Lock()
			p.blankFrames++
			p.mu.Unlock()
			return blankGameFrame(p.Name(), f.Width, f.Height,
				"Gamescope PipeWire frame was effectively black; Vision results withheld"), nil
		}
		frame, _ := linuxCachedFrameForWidth(f, maxWidth)
		p.mu.Lock()
		p.cacheHits++
		p.mu.Unlock()
		return frame, nil
	}

	deadline := time.Now().Add(900 * time.Millisecond)
	for time.Now().Before(deadline) {
		time.Sleep(15 * time.Millisecond)
		if f, ok := p.latestFrame(maxAge); ok {
			if !linuxGamescopeEliteDirectlyFocused() {
				return p.focusBlank("Elite focus changed during PipeWire capture; frame discarded"), nil
			}
			if linuxFrameLooksBlank(f.Pix, f.Width, f.Height) {
				p.mu.Lock()
				p.blankFrames++
				p.mu.Unlock()
				return blankGameFrame(p.Name(), f.Width, f.Height,
					"Gamescope PipeWire frame was effectively black; Vision results withheld"), nil
			}
			frame, _ := linuxCachedFrameForWidth(f, maxWidth)
			return frame, nil
		}
	}

	p.mu.Lock()
	p.failures++
	if p.lastError == "" {
		p.lastError = "timed out waiting for Gamescope PipeWire frame"
	}
	msg := p.lastError
	p.mu.Unlock()
	return GameFrame{}, fmt.Errorf("%s", msg)
}

func (p *pipewireCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	frame, err := p.CaptureFrame(maxWidth, CaptureIntentLive)
	if err != nil {
		return nil, CaptureInfo{Driver: p.Name(), Blanked: true, Reason: err.Error()}, err
	}
	return EncodeGameFrameJPEG(frame, quality)
}

func (p *pipewireCapture) CaptureStats() map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	age := int64(-1)
	if !p.lastFrameAt.IsZero() {
		age = time.Since(p.lastFrameAt).Milliseconds()
	}
	pid := 0
	if p.cmd != nil && p.cmd.Process != nil {
		pid = p.cmd.Process.Pid
	}
	return map[string]any{
		"driver":           p.Name(),
		"transport":        "gamescope-pipewire",
		"sourceAppID":      359320,
		"helper":           gamescopeCaptureHelperPath(),
		"helperPID":        pid,
		"socket":           gamescopeSocketName(),
		"frames":           p.frames,
		"latestSequence":   p.latestSeq,
		"latestFrameAgeMs": age,
		"starts":           p.starts,
		"restarts":         p.restarts,
		"cacheHits":        p.cacheHits,
		"focusRejects":     p.focusRejects,
		"failures":         p.failures,
		"blankFrames":      p.blankFrames,
		"lastError":        p.lastError,
		"requested":        "1280x800@10",
		"focusFilter":      strconv.FormatUint(359320, 10),
		"eliteFocusGuard":  "exact-client-on-read",
	}
}
