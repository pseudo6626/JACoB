//go:build windows && !nocapture

package platform

import (
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

type windowsCapture struct {
	worker *wgcCaptureWorker
}

var procIsIconicCapture = user32.NewProc("IsIconic")

func newCaptureDriver() CaptureDriver {
	return &windowsCapture{worker: newWGCCaptureWorker()}
}

func (w *windowsCapture) Name() string {
	return "windows-wgc-elite-client"
}

func (w *windowsCapture) Available() bool {
	return w != nil && w.worker != nil
}

func (w *windowsCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	frame, err := w.CaptureFrame(maxWidth, CaptureIntentLive)
	if err != nil {
		return nil, CaptureInfo{}, err
	}
	return EncodeGameFrameJPEG(frame, quality)
}

func (w *windowsCapture) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	if w == nil || w.worker == nil {
		return GameFrame{}, fmt.Errorf("Windows Graphics Capture backend is unavailable")
	}

	previous, _, _ := procGetForegroundWindow.Call()
	var hwnd uintptr

	if intent == CaptureIntentCalibration {
		found, exe, err := findEliteWindow()
		if err != nil {
			return GameFrame{}, err
		}
		if found == 0 || !isEliteExecutable(exe) {
			return GameFrame{}, fmt.Errorf("verified Elite Dangerous window was not found")
		}
		input := &windowsInput{}
		if err := input.FocusGame(); err != nil {
			return GameFrame{}, err
		}
		hwnd = found
		if previous != 0 && previous != hwnd {
			defer restoreCalibrationForeground(previous)
		}
		// Give Elite and DWM a moment to present after focus changes. WGC still
		// captures the HWND surface directly; this wait is only for a fresh frame.
		time.Sleep(90 * time.Millisecond)
	} else {
		hwnd, _, _ = procGetForegroundWindow.Call()
		if hwnd == 0 {
			return blankGameFrame(w.Name(), maxWidthOrDefault(maxWidth), blackHeight(maxWidthOrDefault(maxWidth)), "no-foreground-window"), nil
		}
	}

	pid, reason := verifyEliteCaptureWindow(hwnd)
	if reason != "" {
		return blankGameFrame(w.Name(), maxWidthOrDefault(maxWidth), blackHeight(maxWidthOrDefault(maxWidth)), reason), nil
	}

	timeout := 650 * time.Millisecond
	if intent == CaptureIntentCalibration {
		timeout = 1400 * time.Millisecond
	}
	frame, err := w.worker.Capture(hwnd, maxWidth, timeout)
	if err != nil {
		return GameFrame{}, err
	}

	// The WGC source is the Elite HWND, but foreground/process state is checked
	// again before any pixels are released to the caller.
	after, _, _ := procGetForegroundWindow.Call()
	if after != hwnd {
		return blankGameFrame(w.Name(), frame.Width, frame.Height, "elite-capture-state-changed"), nil
	}
	var afterPID uint32
	procGetWindowThreadProcessId.Call(after, uintptr(unsafe.Pointer(&afterPID)))
	if afterPID != pid {
		return blankGameFrame(w.Name(), frame.Width, frame.Height, "elite-process-changed"), nil
	}

	frame.Driver = w.Name()
	frame.Verified = true
	frame.Foreground = true
	frame.Blanked = false
	frame.Reason = ""
	return frame, nil
}

func verifyEliteCaptureWindow(hwnd uintptr) (uint32, string) {
	if hwnd == 0 {
		return 0, "no-foreground-window"
	}
	fg, _, _ := procGetForegroundWindow.Call()
	if fg != hwnd {
		return 0, "foreground-is-not-elite"
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return 0, "foreground-process-unknown"
	}
	exe, err := processBaseName(pid)
	if err != nil || !isEliteExecutable(exe) {
		return 0, "foreground-is-not-elite"
	}
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	iconic, _, _ := procIsIconicCapture.Call(hwnd)
	if visible == 0 {
		return 0, "elite-not-visible"
	}
	if iconic != 0 {
		return 0, "elite-minimized"
	}
	return pid, ""
}

func restoreCalibrationForeground(hwnd uintptr) {
	if hwnd == 0 {
		return
	}
	valid, _, _ := procIsWindow.Call(hwnd)
	if valid == 0 {
		return
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	currentThread, _, _ := procGetCurrentThreadID.Call()
	fg, _, _ := procGetForegroundWindow.Call()
	fgThread := windowThreadID(fg)
	targetThread := windowThreadID(hwnd)

	attachedFG := false
	attachedTarget := false
	if fgThread != 0 && fgThread != currentThread {
		if r, _, _ := procAttachThreadInput.Call(currentThread, fgThread, 1); r != 0 {
			attachedFG = true
		}
	}
	if targetThread != 0 && targetThread != currentThread && targetThread != fgThread {
		if r, _, _ := procAttachThreadInput.Call(currentThread, targetThread, 1); r != 0 {
			attachedTarget = true
		}
	}
	defer func() {
		if attachedTarget {
			procAttachThreadInput.Call(currentThread, targetThread, 0)
		}
		if attachedFG {
			procAttachThreadInput.Call(currentThread, fgThread, 0)
		}
	}()

	procShowWindow.Call(hwnd, swRestore)
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)

	deadline := time.Now().Add(650 * time.Millisecond)
	for time.Now().Before(deadline) {
		now, _, _ := procGetForegroundWindow.Call()
		if now == hwnd {
			return
		}
		time.Sleep(15 * time.Millisecond)
	}
}

func maxWidthOrDefault(v int) int {
	if v < 160 {
		return 960
	}
	if v > 1920 {
		return 1920
	}
	return v
}

func blackHeight(width int) int {
	h := width * 9 / 16
	if h < 90 {
		h = 90
	}
	return h
}

func encodeBlackJPEG(dw, dh, quality int, reason string) ([]byte, CaptureInfo, error) {
	return EncodeGameFrameJPEG(blankGameFrame("windows-wgc-elite-client", dw, dh, reason), quality)
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
