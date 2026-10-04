//go:build linux

package platform

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	evSyn        = 0
	evKey        = 1
	synReport    = 0
	uiSetEvbit   = uintptr(0x40045564)
	uiSetKeybit  = uintptr(0x40045565)
	uiDevSetup   = uintptr(0x405c5503)
	uiDevCreate  = uintptr(0x5501)
	uiDevDestroy = uintptr(0x5502)
	busUSB       = 0x03
)

type inputID struct{ BusType, Vendor, Product, Version uint16 }
type uinputSetup struct {
	ID           inputID
	Name         [80]byte
	FFEffectsMax uint32
}
type inputEvent struct {
	Time  syscall.Timeval
	Type  uint16
	Code  uint16
	Value int32
}

type linuxInput struct {
	mu    sync.Mutex
	fd    int
	ready bool
}

func newInputDriver() InputDriver  { return &linuxInput{fd: -1} }
func (l *linuxInput) Name() string { return "linux-uinput" }
func (l *linuxInput) Available() bool {
	fd, err := syscall.Open("/dev/uinput", syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return false
	}
	_ = syscall.Close(fd)
	return true
}
func (l *linuxInput) FocusGame() error {
	return nil
}
func (l *linuxInput) TapKey(key string) error { return l.TapChord(key, nil) }

func linuxHoldDuration() time.Duration {
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

func (l *linuxInput) TapChord(key string, modifiers []string) error {
	return l.holdChordFor(key, modifiers, linuxHoldDuration())
}

func (l *linuxInput) HoldChord(key string, modifiers []string, durationMs int) error {
	if durationMs < 20 {
		durationMs = 20
	}
	if durationMs > 10000 {
		durationMs = 10000
	}
	return l.holdChordFor(key, modifiers, time.Duration(durationMs)*time.Millisecond)
}

func (l *linuxInput) resolveChordLocked(key string, modifiers []string) (uint16, []uint16, error) {
	mods := make([]uint16, 0, len(modifiers))
	for _, m := range modifiers {
		code, ok := evdevKey(m)
		if !ok {
			return 0, nil, fmt.Errorf("unsupported Linux key %q", m)
		}
		mods = append(mods, code)
	}
	main, ok := evdevKey(key)
	if !ok {
		return 0, nil, fmt.Errorf("unsupported Linux key %q", key)
	}
	return main, mods, nil
}

func (l *linuxInput) ChordDown(key string, modifiers []string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureDeviceLocked(); err != nil {
		return err
	}
	main, mods, err := l.resolveChordLocked(key, modifiers)
	if err != nil {
		return err
	}
	for _, code := range mods {
		if err := l.emitKey(code, 1); err != nil {
			return err
		}
	}
	if err := l.emitKey(main, 1); err != nil {
		return err
	}
	return l.sync()
}

func (l *linuxInput) ChordUp(key string, modifiers []string) error {
	l.mu.Lock()
	defer l.mu.Unlock()
	if err := l.ensureDeviceLocked(); err != nil {
		return err
	}
	main, mods, err := l.resolveChordLocked(key, modifiers)
	if err != nil {
		return err
	}
	if err := l.emitKey(main, 0); err != nil {
		return err
	}
	for i := len(mods) - 1; i >= 0; i-- {
		if err := l.emitKey(mods[i], 0); err != nil {
			return err
		}
	}
	return l.sync()
}

func (l *linuxInput) holdChordFor(key string, modifiers []string, hold time.Duration) error {
	if err := l.ChordDown(key, modifiers); err != nil {
		return err
	}
	time.Sleep(hold)
	return l.ChordUp(key, modifiers)
}

func (l *linuxInput) ensureDeviceLocked() error {
	if l.ready && l.fd >= 0 {
		return nil
	}
	fd, err := syscall.Open("/dev/uinput", syscall.O_WRONLY|syscall.O_NONBLOCK, 0)
	if err != nil {
		return fmt.Errorf("open /dev/uinput: %w (install the JACoB udev rule or grant write access)", err)
	}
	fail := func(e error) error { syscall.Close(fd); return e }
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uiSetEvbit, uintptr(evSyn)); e != 0 {
		return fail(fmt.Errorf("uinput EV_SYN: %v", e))
	}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uiSetEvbit, uintptr(evKey)); e != 0 {
		return fail(fmt.Errorf("uinput EV_KEY: %v", e))
	}
	for code := 1; code <= 255; code++ {
		if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uiSetKeybit, uintptr(code)); e != 0 {
			return fail(fmt.Errorf("uinput key bit %d: %v", code, e))
		}
	}
	setup := uinputSetup{ID: inputID{BusType: busUSB, Vendor: 0x1209, Product: 0xEDB2, Version: 1}}
	copy(setup.Name[:], []byte("JACoB Virtual Keyboard"))
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uiDevSetup, uintptr(unsafe.Pointer(&setup))); e != 0 {
		return fail(fmt.Errorf("uinput device setup: %v", e))
	}
	if _, _, e := syscall.Syscall(syscall.SYS_IOCTL, uintptr(fd), uiDevCreate, 0); e != 0 {
		return fail(fmt.Errorf("uinput device create: %v", e))
	}
	l.fd, l.ready = fd, true
	time.Sleep(200 * time.Millisecond)
	return nil
}

func (l *linuxInput) writeEvent(ev inputEvent) error {
	b := unsafe.Slice((*byte)(unsafe.Pointer(&ev)), int(unsafe.Sizeof(ev)))
	n, err := syscall.Write(l.fd, b)
	if err != nil {
		return err
	}
	if n != len(b) {
		return fmt.Errorf("short uinput write %d/%d", n, len(b))
	}
	return nil
}
func (l *linuxInput) emitKey(code uint16, value int32) error {
	return l.writeEvent(inputEvent{Type: evKey, Code: code, Value: value})
}
func (l *linuxInput) sync() error {
	return l.writeEvent(inputEvent{Type: evSyn, Code: synReport, Value: 0})
}

func evdevKey(key string) (uint16, bool) {
	k := strings.ToUpper(strings.TrimSpace(key))
	m := map[string]uint16{
		"ESC": 1, "ESCAPE": 1, "1": 2, "2": 3, "3": 4, "4": 5, "5": 6, "6": 7, "7": 8, "8": 9, "9": 10, "0": 11, "MINUS": 12, "EQUALS": 13, "BACKSPACE": 14, "TAB": 15,
		"Q": 16, "W": 17, "E": 18, "R": 19, "T": 20, "Y": 21, "U": 22, "I": 23, "O": 24, "P": 25, "LEFTBRACKET": 26, "RIGHTBRACKET": 27, "ENTER": 28, "RETURN": 28, "CTRL": 29, "CONTROL": 29, "LEFTCONTROL": 29,
		"A": 30, "S": 31, "D": 32, "F": 33, "G": 34, "H": 35, "J": 36, "K": 37, "L": 38, "SEMICOLON": 39, "APOSTROPHE": 40, "GRAVE": 41, "SHIFT": 42, "LEFTSHIFT": 42, "BACKSLASH": 43,
		"Z": 44, "X": 45, "C": 46, "V": 47, "B": 48, "N": 49, "M": 50, "COMMA": 51, "PERIOD": 52, "SLASH": 53, "RIGHTSHIFT": 54, "MULTIPLY": 55, "ALT": 56, "LEFTALT": 56, "SPACE": 57,
		"F1": 59, "F2": 60, "F3": 61, "F4": 62, "F5": 63, "F6": 64, "F7": 65, "F8": 66, "F9": 67, "F10": 68, "NUMPAD7": 71, "NUMPAD8": 72, "NUMPAD9": 73, "SUBTRACT": 74, "NUMPAD4": 75, "NUMPAD5": 76, "NUMPAD6": 77, "ADD": 78, "NUMPAD1": 79, "NUMPAD2": 80, "NUMPAD3": 81, "NUMPAD0": 82, "DECIMAL": 83, "F11": 87, "F12": 88,
		"RIGHTCONTROL": 97, "DIVIDE": 98, "RIGHTALT": 100, "HOME": 102, "UP": 103, "PAGEUP": 104, "LEFT": 105, "RIGHT": 106, "END": 107, "DOWN": 108, "PAGEDOWN": 109, "INSERT": 110, "DELETE": 111,
	}
	v, ok := m[k]
	return v, ok
}
