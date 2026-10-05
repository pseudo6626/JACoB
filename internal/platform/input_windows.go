//go:build windows

package platform

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const (
	inputKeyboard                = 1
	keyeventfExtendedKey         = 0x0001
	keyeventfKeyUp               = 0x0002
	keyeventfScanCode            = 0x0008
	mapvkVkToVsc                 = 0
	jacobInputMarker     uintptr = 0x4A41434F
)

type keyboardInput struct {
	Vk        uint16
	Scan      uint16
	Flags     uint32
	Time      uint32
	ExtraInfo uintptr
}

type winInput struct {
	Type uint32
	_    uint32
	Data [32]byte
}

type windowsInput struct{}

func newInputDriver() InputDriver       { return &windowsInput{} }
func (w *windowsInput) Name() string    { return "windows-sendinput-scancode" }
func (w *windowsInput) Available() bool { return true }

var user32 = syscall.NewLazyDLL("user32.dll")
var procSendInput = user32.NewProc("SendInput")
var procMapVirtualKeyW = user32.NewProc("MapVirtualKeyW")

func (w *windowsInput) TapKey(key string) error { return w.TapChord(key, nil) }

func keyHoldDuration() time.Duration {
	ms := 70
	if raw := strings.TrimSpace(func() string {
		if v := os.Getenv("JACOB_KEY_HOLD_MS"); v != "" {
			return v
		}
		return os.Getenv("EDBRIDGE_KEY_HOLD_MS")
	}()); raw != "" {
		if v, err := strconv.Atoi(raw); err == nil && v >= 20 && v <= 1000 {
			ms = v
		}
	}
	return time.Duration(ms) * time.Millisecond
}

func (w *windowsInput) TapChord(key string, modifiers []string) error {
	return w.holdChordFor(key, modifiers, keyHoldDuration())
}

func (w *windowsInput) HoldChord(key string, modifiers []string, durationMs int) error {
	if durationMs < 20 {
		durationMs = 20
	}
	if durationMs > 10000 {
		durationMs = 10000
	}
	return w.holdChordFor(key, modifiers, time.Duration(durationMs)*time.Millisecond)
}

type resolvedWindowsKey struct {
	vk       uint16
	scan     uint16
	extended bool
	name     string
}

func resolveWindowsKey(k string) (resolvedWindowsKey, error) {
	vk, ext, ok := virtualKey(k)
	if !ok {
		return resolvedWindowsKey{}, fmt.Errorf("unsupported key %q", k)
	}
	scan, _, callErr := procMapVirtualKeyW.Call(uintptr(vk), mapvkVkToVsc)
	if scan == 0 {
		if callErr != syscall.Errno(0) {
			return resolvedWindowsKey{}, fmt.Errorf("MapVirtualKeyW(%s) failed: %w", k, callErr)
		}
		return resolvedWindowsKey{}, fmt.Errorf("MapVirtualKeyW(%s) returned scan code 0", k)
	}
	return resolvedWindowsKey{vk: vk, scan: uint16(scan & 0xff), extended: ext, name: k}, nil
}

func makeWindowsKeyEvent(r resolvedWindowsKey, up bool) winInput {
	var in winInput
	in.Type = inputKeyboard
	ki := (*keyboardInput)(unsafe.Pointer(&in.Data[0]))
	ki.Vk = 0
	ki.Scan = r.scan
	ki.ExtraInfo = jacobInputMarker
	ki.Flags = keyeventfScanCode
	if r.extended {
		ki.Flags |= keyeventfExtendedKey
	}
	if up {
		ki.Flags |= keyeventfKeyUp
	}
	return in
}

func resolveWindowsChord(key string, modifiers []string) (resolvedWindowsKey, []resolvedWindowsKey, error) {
	if err := ValidateSafeChord(key, modifiers); err != nil {
		return resolvedWindowsKey{}, nil, err
	}
	mods := make([]resolvedWindowsKey, 0, len(modifiers))
	for _, m := range modifiers {
		r, err := resolveWindowsKey(m)
		if err != nil {
			return resolvedWindowsKey{}, nil, err
		}
		mods = append(mods, r)
	}
	main, err := resolveWindowsKey(key)
	if err != nil {
		return resolvedWindowsKey{}, nil, err
	}
	return main, mods, nil
}

func (w *windowsInput) ChordDown(key string, modifiers []string) error {
	main, mods, err := resolveWindowsChord(key, modifiers)
	if err != nil {
		return err
	}
	down := make([]winInput, 0, len(mods)+1)
	for _, m := range mods {
		down = append(down, makeWindowsKeyEvent(m, false))
	}
	down = append(down, makeWindowsKeyEvent(main, false))
	if err := sendInputBatch(down); err != nil {
		return fmt.Errorf("key-down phase failed: %w", err)
	}
	return nil
}

func (w *windowsInput) ChordUp(key string, modifiers []string) error {
	main, mods, err := resolveWindowsChord(key, modifiers)
	if err != nil {
		return err
	}
	up := make([]winInput, 0, len(mods)+1)
	up = append(up, makeWindowsKeyEvent(main, true))
	for i := len(mods) - 1; i >= 0; i-- {
		up = append(up, makeWindowsKeyEvent(mods[i], true))
	}
	if err := sendInputBatch(up); err != nil {
		return fmt.Errorf("key-up phase failed: %w", err)
	}
	return nil
}

func (w *windowsInput) holdChordFor(key string, modifiers []string, hold time.Duration) error {
	if err := w.ChordDown(key, modifiers); err != nil {
		return err
	}
	time.Sleep(hold)
	return w.ChordUp(key, modifiers)
}

func sendInputBatch(inputs []winInput) error {
	if len(inputs) == 0 {
		return nil
	}
	r1, _, callErr := procSendInput.Call(
		uintptr(len(inputs)),
		uintptr(unsafe.Pointer(&inputs[0])),
		unsafe.Sizeof(inputs[0]),
	)
	if int(r1) != len(inputs) {
		if callErr != syscall.Errno(0) {
			return fmt.Errorf("SendInput inserted %d/%d events: %w", r1, len(inputs), callErr)
		}
		return fmt.Errorf("SendInput inserted %d/%d events", r1, len(inputs))
	}
	return nil
}

func virtualKey(key string) (vk uint16, extended bool, ok bool) {
	k := strings.ToUpper(strings.TrimSpace(key))
	if len(k) == 1 {
		c := k[0]
		if (c >= 'A' && c <= 'Z') || (c >= '0' && c <= '9') {
			return uint16(c), false, true
		}
	}
	keys := map[string]struct {
		vk       uint16
		extended bool
	}{
		"ENTER": {0x0D, false}, "RETURN": {0x0D, false}, "ESC": {0x1B, false}, "ESCAPE": {0x1B, false}, "TAB": {0x09, false}, "SPACE": {0x20, false}, "BACKSPACE": {0x08, false},
		"PAGEUP": {0x21, true}, "PAGEDOWN": {0x22, true}, "END": {0x23, true}, "HOME": {0x24, true},
		"UP": {0x26, true}, "DOWN": {0x28, true}, "LEFT": {0x25, true}, "RIGHT": {0x27, true}, "INSERT": {0x2D, true}, "DELETE": {0x2E, true},
		"CTRL": {0x11, false}, "CONTROL": {0x11, false}, "LEFTCONTROL": {0xA2, false}, "RIGHTCONTROL": {0xA3, true},
		"SHIFT": {0x10, false}, "LEFTSHIFT": {0xA0, false}, "RIGHTSHIFT": {0xA1, false},
		"ALT": {0x12, false}, "LEFTALT": {0xA4, false}, "RIGHTALT": {0xA5, true},
		"NUMPAD0": {0x60, false}, "NUMPAD1": {0x61, false}, "NUMPAD2": {0x62, false}, "NUMPAD3": {0x63, false}, "NUMPAD4": {0x64, false},
		"NUMPAD5": {0x65, false}, "NUMPAD6": {0x66, false}, "NUMPAD7": {0x67, false}, "NUMPAD8": {0x68, false}, "NUMPAD9": {0x69, false},
		"MULTIPLY": {0x6A, false}, "ADD": {0x6B, false}, "SUBTRACT": {0x6D, false}, "DECIMAL": {0x6E, false}, "DIVIDE": {0x6F, true},
		"SEMICOLON": {0xBA, false}, "EQUALS": {0xBB, false}, "COMMA": {0xBC, false}, "MINUS": {0xBD, false}, "PERIOD": {0xBE, false},
		"SLASH": {0xBF, false}, "GRAVE": {0xC0, false}, "LEFTBRACKET": {0xDB, false}, "BACKSLASH": {0xDC, false}, "RIGHTBRACKET": {0xDD, false}, "APOSTROPHE": {0xDE, false},
		"F1": {0x70, false}, "F2": {0x71, false}, "F3": {0x72, false}, "F4": {0x73, false}, "F5": {0x74, false}, "F6": {0x75, false},
		"F7": {0x76, false}, "F8": {0x77, false}, "F9": {0x78, false}, "F10": {0x79, false}, "F11": {0x7A, false}, "F12": {0x7B, false},
	}
	v, found := keys[k]
	if !found {
		return 0, false, false
	}
	return v.vk, v.extended, true
}
