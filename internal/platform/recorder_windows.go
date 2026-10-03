//go:build windows && !norecorder

package platform

import (
	"fmt"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"
)

const (
	whKeyboardLL         = 13
	wmKeyDown            = 0x0100
	wmKeyUp              = 0x0101
	wmSysKeyDown         = 0x0104
	wmSysKeyUp           = 0x0105
	wmQuit               = 0x0012
	llkhfLowerILInjected = 0x00000002
	llkhfInjected        = 0x00000010
)

type kbdLLHookStruct struct {
	VkCode      uint32
	ScanCode    uint32
	Flags       uint32
	Time        uint32
	DwExtraInfo uintptr
}

type winMSG struct {
	Hwnd    uintptr
	Message uint32
	WParam  uintptr
	LParam  uintptr
	Time    uint32
	PtX     int32
	PtY     int32
	Private uint32
}

type activePress struct {
	id        uint64
	key       string
	modifiers []string
	atMs      int64
	isMod     bool
}

type windowsRecorder struct {
	mu        sync.Mutex
	recording bool
	start     time.Time
	lastAtMs  int64
	events    []RecordedInputEvent
	callback  func(RecordedInputEvent)
	pressed   map[uint32]activePress
	mods      map[string]bool
	eliteHwnd uintptr
	hook      uintptr
	hookCB    uintptr
	threadID  uint32
	done      chan struct{}
	seq       uint64
}

var (
	procSetWindowsHookExW   = user32.NewProc("SetWindowsHookExW")
	procCallNextHookEx      = user32.NewProc("CallNextHookEx")
	procUnhookWindowsHookEx = user32.NewProc("UnhookWindowsHookEx")
	procGetMessageW         = user32.NewProc("GetMessageW")
	procPostThreadMessageW  = user32.NewProc("PostThreadMessageW")
)

func newInputRecorder() InputRecorder {
	r := &windowsRecorder{pressed: map[uint32]activePress{}, mods: map[string]bool{}}
	r.hookCB = syscall.NewCallback(r.hookProc)
	return r
}
func (r *windowsRecorder) Name() string    { return "windows-low-level-keyboard-hook" }
func (r *windowsRecorder) Available() bool { return true }

func (r *windowsRecorder) Status() RecorderStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RecorderStatus{Available: true, Driver: r.Name(), Recording: r.recording, EventCount: len(r.events), Scope: "Elite foreground keyboard only; JACoB-generated input ignored; Moonlight/Sunshine input accepted"}
}

func (r *windowsRecorder) Start(cb func(RecordedInputEvent)) error {
	r.mu.Lock()
	if r.recording {
		r.mu.Unlock()
		return fmt.Errorf("input recorder is already running")
	}
	hwnd, _, err := findEliteWindow()
	if err != nil || hwnd == 0 {
		r.mu.Unlock()
		if err != nil {
			return err
		}
		return fmt.Errorf("Elite Dangerous window was not found")
	}
	r.recording = true
	r.start = time.Now()
	r.lastAtMs = 0
	r.events = nil
	r.callback = cb
	r.pressed = map[uint32]activePress{}
	r.mods = map[string]bool{}
	r.eliteHwnd = hwnd
	r.done = make(chan struct{})
	r.seq = 0
	ready := make(chan error, 1)
	r.mu.Unlock()

	go r.runHook(ready)
	if err := <-ready; err != nil {
		r.mu.Lock()
		r.recording = false
		r.callback = nil
		r.mu.Unlock()
		return err
	}
	return nil
}

func (r *windowsRecorder) runHook(ready chan<- error) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	thread, _, _ := procGetCurrentThreadID.Call()
	r.mu.Lock()
	cb := r.hookCB
	r.mu.Unlock()
	hook, _, callErr := procSetWindowsHookExW.Call(whKeyboardLL, cb, 0, 0)
	if hook == 0 {
		if callErr != syscall.Errno(0) {
			ready <- fmt.Errorf("SetWindowsHookExW failed: %w", callErr)
		} else {
			ready <- fmt.Errorf("SetWindowsHookExW failed")
		}
		close(r.done)
		return
	}
	r.mu.Lock()
	r.threadID = uint32(thread)
	r.hook = hook
	r.hookCB = cb
	r.mu.Unlock()
	ready <- nil

	var msg winMSG
	for {
		ret, _, _ := procGetMessageW.Call(uintptr(unsafe.Pointer(&msg)), 0, 0, 0)
		if int32(ret) <= 0 {
			break
		}
	}
	procUnhookWindowsHookEx.Call(hook)
	r.mu.Lock()
	r.hook = 0
	r.threadID = 0
	r.mu.Unlock()
	close(r.done)
}

func (r *windowsRecorder) Stop() ([]RecordedInputEvent, error) {
	r.mu.Lock()
	if !r.recording {
		r.mu.Unlock()
		return nil, fmt.Errorf("input recorder is not running")
	}
	r.recording = false
	threadID := r.threadID
	done := r.done
	r.mu.Unlock()
	if threadID != 0 {
		procPostThreadMessageW.Call(uintptr(threadID), wmQuit, 0, 0)
	}
	if done != nil {
		select {
		case <-done:
		case <-time.After(2 * time.Second):
			return nil, fmt.Errorf("timed out stopping input recorder")
		}
	}
	r.mu.Lock()
	out := append([]RecordedInputEvent(nil), r.events...)
	r.callback = nil
	r.mu.Unlock()
	return out, nil
}

func (r *windowsRecorder) hookProc(nCode int, wParam uintptr, lParam uintptr) uintptr {
	if nCode >= 0 && lParam != 0 {
		k := (*kbdLLHookStruct)(unsafe.Pointer(lParam))
		if k.DwExtraInfo != jacobInputMarker {
			fg, _, _ := procGetForegroundWindow.Call()
			r.mu.Lock()
			target := r.eliteHwnd
			running := r.recording
			r.mu.Unlock()
			if running && fg == target {
				switch uint32(wParam) {
				case wmKeyDown, wmSysKeyDown:
					r.recordKey(k.VkCode, true)
				case wmKeyUp, wmSysKeyUp:
					r.recordKey(k.VkCode, false)
				}
			}
		}
	}
	ret, _, _ := procCallNextHookEx.Call(0, uintptr(nCode), wParam, lParam)
	return ret
}

func (r *windowsRecorder) recordKey(vk uint32, down bool) {
	key, ok := normalizedVK(vk)
	if !ok {
		return
	}
	isMod := isModifierKey(key)

	r.mu.Lock()
	if !r.recording {
		r.mu.Unlock()
		return
	}
	now := time.Since(r.start).Milliseconds()
	delta := now - r.lastAtMs
	if delta < 0 {
		delta = 0
	}

	if down {
		if _, exists := r.pressed[vk]; exists {
			r.mu.Unlock()
			return // auto-repeat; one logical press is enough for replay
		}
		mods := r.currentModifiersLocked()
		id := atomic.AddUint64(&r.seq, 1)
		p := activePress{id: id, key: key, modifiers: mods, atMs: now, isMod: isMod}
		r.pressed[vk] = p
		ev := RecordedInputEvent{PressID: id, Type: "down", Key: key, Modifiers: append([]string(nil), mods...), AtMs: now, DeltaMs: delta, IsModifier: isMod}
		if isMod {
			r.mods[key] = true
		}
		r.lastAtMs = now
		r.events = append(r.events, ev)
		cb := r.callback
		r.mu.Unlock()
		if cb != nil {
			cb(ev)
		}
		return
	}

	p, exists := r.pressed[vk]
	if !exists {
		if isMod {
			delete(r.mods, key)
		}
		r.mu.Unlock()
		return
	}
	delete(r.pressed, vk)
	if isMod {
		delete(r.mods, key)
	}
	dur := now - p.atMs
	if dur < 0 {
		dur = 0
	}
	ev := RecordedInputEvent{PressID: p.id, Type: "up", Key: p.key, Modifiers: append([]string(nil), p.modifiers...), AtMs: now, DeltaMs: delta, DurationMs: dur, IsModifier: p.isMod}
	r.lastAtMs = now
	r.events = append(r.events, ev)
	cb := r.callback
	r.mu.Unlock()
	if cb != nil {
		cb(ev)
	}
}

func (r *windowsRecorder) currentModifiersLocked() []string {
	mods := make([]string, 0, len(r.mods))
	for _, k := range []string{"CTRL", "RIGHTCONTROL", "ALT", "RIGHTALT", "SHIFT", "RIGHTSHIFT"} {
		if r.mods[k] {
			mods = append(mods, k)
		}
	}
	sort.Strings(mods)
	return mods
}

func isModifierKey(k string) bool {
	switch strings.ToUpper(k) {
	case "CTRL", "RIGHTCONTROL", "ALT", "RIGHTALT", "SHIFT", "RIGHTSHIFT":
		return true
	}
	return false
}

func normalizedVK(vk uint32) (string, bool) {
	if vk >= 'A' && vk <= 'Z' {
		return string(rune(vk)), true
	}
	if vk >= '0' && vk <= '9' {
		return string(rune(vk)), true
	}
	if vk >= 0x70 && vk <= 0x7B {
		return fmt.Sprintf("F%d", int(vk-0x70)+1), true
	}
	m := map[uint32]string{
		0x08: "BACKSPACE", 0x09: "TAB", 0x0D: "ENTER", 0x1B: "ESC", 0x20: "SPACE",
		0x21: "PAGEUP", 0x22: "PAGEDOWN", 0x23: "END", 0x24: "HOME", 0x25: "LEFT", 0x26: "UP", 0x27: "RIGHT", 0x28: "DOWN", 0x2D: "INSERT", 0x2E: "DELETE",
		0x11: "CTRL", 0xA2: "CTRL", 0xA3: "RIGHTCONTROL", 0x10: "SHIFT", 0xA0: "SHIFT", 0xA1: "RIGHTSHIFT", 0x12: "ALT", 0xA4: "ALT", 0xA5: "RIGHTALT",
		0x60: "NUMPAD0", 0x61: "NUMPAD1", 0x62: "NUMPAD2", 0x63: "NUMPAD3", 0x64: "NUMPAD4", 0x65: "NUMPAD5", 0x66: "NUMPAD6", 0x67: "NUMPAD7", 0x68: "NUMPAD8", 0x69: "NUMPAD9",
		0x6A: "MULTIPLY", 0x6B: "ADD", 0x6D: "SUBTRACT", 0x6E: "DECIMAL", 0x6F: "DIVIDE",
		0xBA: "SEMICOLON", 0xBB: "EQUALS", 0xBC: "COMMA", 0xBD: "MINUS", 0xBE: "PERIOD", 0xBF: "SLASH", 0xC0: "GRAVE", 0xDB: "LEFTBRACKET", 0xDC: "BACKSLASH", 0xDD: "RIGHTBRACKET", 0xDE: "APOSTROPHE",
	}
	v, ok := m[vk]
	return v, ok
}
