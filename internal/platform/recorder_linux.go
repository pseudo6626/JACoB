//go:build linux

package platform

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unsafe"
)

type linuxRecorder struct {
	mu        sync.Mutex
	recording bool
	started   time.Time
	lastAt    time.Time
	events    []RecordedInputEvent
	callback  func(RecordedInputEvent)
	files     []*os.File
	stop      chan struct{}
	wg        sync.WaitGroup
	active    map[string]linuxRecordedPress
	modifiers map[uint16]bool
	nextID    uint64
}

type linuxRecordedPress struct {
	id       uint64
	key      string
	physical string
	scan     uint32
	extended bool
	at       time.Time
	atMs     int64
	mods     []string
	modifier bool
}

func newInputRecorder() InputRecorder {
	return &linuxRecorder{active: map[string]linuxRecordedPress{}, modifiers: map[uint16]bool{}}
}

func (r *linuxRecorder) Name() string { return "linux-evdev" }

func (r *linuxRecorder) Available() bool {
	files := discoverLinuxRecorderDevices(false)
	return len(files) > 0
}

func (r *linuxRecorder) Start(callback func(RecordedInputEvent)) error {
	r.mu.Lock()
	if r.recording {
		r.mu.Unlock()
		return fmt.Errorf("input recorder is already running")
	}
	paths := discoverLinuxRecorderDevices(false)
	if len(paths) == 0 {
		r.mu.Unlock()
		return fmt.Errorf("no readable Linux keyboard event devices were found; reinstall JACoB's Steam Deck permissions helper or verify /dev/input access")
	}

	files := make([]*os.File, 0, len(paths))
	for _, path := range paths {
		f, err := os.Open(path)
		if err == nil {
			files = append(files, f)
		}
	}
	if len(files) == 0 {
		r.mu.Unlock()
		return fmt.Errorf("Linux keyboard devices were detected but none could be opened")
	}

	r.recording = true
	r.started = time.Now()
	r.lastAt = time.Time{}
	r.events = nil
	r.callback = callback
	r.files = files
	r.stop = make(chan struct{})
	r.active = map[string]linuxRecordedPress{}
	r.modifiers = map[uint16]bool{}
	r.mu.Unlock()

	for _, f := range files {
		r.wg.Add(1)
		go r.readDevice(f)
	}
	return nil
}

func (r *linuxRecorder) Stop() ([]RecordedInputEvent, error) {
	r.mu.Lock()
	if !r.recording {
		out := append([]RecordedInputEvent(nil), r.events...)
		r.mu.Unlock()
		return out, fmt.Errorf("input recorder is not running")
	}
	r.recording = false
	if r.stop != nil {
		close(r.stop)
		r.stop = nil
	}
	files := append([]*os.File(nil), r.files...)
	r.files = nil
	r.mu.Unlock()

	for _, f := range files {
		_ = f.Close()
	}
	r.wg.Wait()

	r.mu.Lock()
	defer r.mu.Unlock()
	out := append([]RecordedInputEvent(nil), r.events...)
	r.callback = nil
	r.active = map[string]linuxRecordedPress{}
	r.modifiers = map[uint16]bool{}
	return out, nil
}

func (r *linuxRecorder) Status() RecorderStatus {
	r.mu.Lock()
	defer r.mu.Unlock()
	return RecorderStatus{
		Available: r.recording || len(discoverLinuxRecorderDevices(false)) > 0,
		Driver:    r.Name(), Recording: r.recording, EventCount: len(r.events),
		Scope: "Elite foreground physical keyboard only; JACoB virtual keyboard excluded",
	}
}

func (r *linuxRecorder) readDevice(f *os.File) {
	defer r.wg.Done()
	path := f.Name()
	sz := int(unsafe.Sizeof(inputEvent{}))
	buf := make([]byte, sz)
	for {
		if _, err := io.ReadFull(f, buf); err != nil {
			return
		}
		ev := *(*inputEvent)(unsafe.Pointer(&buf[0]))
		if ev.Type != evKey || ev.Value == 2 {
			continue
		}
		key, ok := linuxRecorderKeyName(ev.Code)
		if !ok {
			continue
		}
		r.acceptKey(path, ev.Code, ev.Value, key)
	}
}

func (r *linuxRecorder) acceptKey(device string, code uint16, value int32, key string) {
	now := time.Now()
	pressKey := fmt.Sprintf("%s:%d", device, code)

	r.mu.Lock()
	if !r.recording {
		r.mu.Unlock()
		return
	}
	_, wasDown := r.active[pressKey]
	r.mu.Unlock()

	// New presses are accepted only while Elite is positively foreground. A key
	// release for a press that started in Elite is still accepted so the recorded
	// action cannot be stranded by a Steam overlay/focus transition.
	if value == 1 && !wasDown {
		if err := verifyLinuxEliteForeground(); err != nil {
			return
		}
	}
	if value == 0 && !wasDown {
		return
	}

	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.recording {
		return
	}

	isMod := linuxRecorderModifier(code)
	if value == 1 {
		if isMod {
			r.modifiers[code] = true
		}
		mods := r.modifierNamesLocked()
		id := atomic.AddUint64(&r.nextID, 1)
		physical, scan, extended := linuxRecorderPhysical(code)
		atMs := now.Sub(r.started).Milliseconds()
		p := linuxRecordedPress{id: id, key: key, physical: physical, scan: scan, extended: extended, at: now, atMs: atMs, mods: mods, modifier: isMod}
		r.active[pressKey] = p
		e := RecordedInputEvent{PressID: id, Type: "down", Key: key, Physical: physical, LocalizedName: key, ScanCode: scan, Extended: extended, Modifiers: mods, AtMs: atMs, DeltaMs: r.deltaLocked(now), IsModifier: isMod}
		r.appendLocked(e)
		return
	}

	p, ok := r.active[pressKey]
	if !ok {
		return
	}
	delete(r.active, pressKey)
	mods := r.modifierNamesLocked()
	e := RecordedInputEvent{PressID: p.id, Type: "up", Key: p.key, Physical: p.physical, LocalizedName: p.key, ScanCode: p.scan, Extended: p.extended, Modifiers: mods, AtMs: now.Sub(r.started).Milliseconds(), DeltaMs: r.deltaLocked(now), DurationMs: now.Sub(p.at).Milliseconds(), IsModifier: p.modifier}
	if e.DurationMs < 0 {
		e.DurationMs = 0
	}
	r.appendLocked(e)
	if isMod {
		delete(r.modifiers, code)
	}
}

func (r *linuxRecorder) deltaLocked(now time.Time) int64 {
	var delta int64
	if !r.lastAt.IsZero() {
		delta = now.Sub(r.lastAt).Milliseconds()
		if delta < 0 {
			delta = 0
		}
	}
	r.lastAt = now
	return delta
}

func (r *linuxRecorder) appendLocked(ev RecordedInputEvent) {
	r.events = append(r.events, ev)
	cb := r.callback
	if cb != nil {
		cb(ev)
	}
}

func (r *linuxRecorder) modifierNamesLocked() []string {
	out := make([]string, 0, 4)
	seen := map[string]bool{}
	for code, down := range r.modifiers {
		if !down {
			continue
		}
		name, ok := linuxRecorderKeyName(code)
		if !ok {
			continue
		}
		switch name {
		case "LEFTCONTROL", "RIGHTCONTROL":
			name = "CTRL"
		case "LEFTSHIFT", "RIGHTSHIFT":
			name = "SHIFT"
		case "LEFTALT", "RIGHTALT":
			name = "ALT"
		}
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	sort.Strings(out)
	return out
}

func discoverLinuxRecorderDevices(openableOnly bool) []string {
	paths, _ := filepath.Glob("/dev/input/event*")
	sort.Strings(paths)
	out := make([]string, 0, len(paths))
	for _, path := range paths {
		base := filepath.Base(path)
		nameBytes, _ := os.ReadFile(filepath.Join("/sys/class/input", base, "device", "name"))
		name := strings.ToLower(strings.TrimSpace(string(nameBytes)))
		if name == "" || strings.Contains(name, "jacob virtual keyboard") {
			continue
		}
		if strings.Contains(name, "mouse") || strings.Contains(name, "touch") || strings.Contains(name, "gamepad") || strings.Contains(name, "controller") || strings.Contains(name, "joystick") {
			continue
		}
		if openableOnly {
			f, err := os.Open(path)
			if err != nil {
				continue
			}
			_ = f.Close()
		} else {
			f, err := os.Open(path)
			if err != nil {
				continue
			}
			_ = f.Close()
		}
		out = append(out, path)
	}
	return out
}

func linuxRecorderModifier(code uint16) bool {
	switch code {
	case 29, 42, 54, 56, 97, 100:
		return true
	}
	return false
}

func linuxRecorderPhysical(code uint16) (string, uint32, bool) {
	if code >= 1 && code <= 88 {
		return FormatPhysicalKeyToken(uint32(code), false), uint32(code), false
	}
	reverse := map[uint16]uint32{96: 0x1c, 97: 0x1d, 98: 0x35, 99: 0x37, 100: 0x38, 102: 0x47, 103: 0x48, 104: 0x49, 105: 0x4b, 106: 0x4d, 107: 0x4f, 108: 0x50, 109: 0x51, 110: 0x52, 111: 0x53, 125: 0x5b, 126: 0x5c, 127: 0x5d}
	if scan, ok := reverse[code]; ok {
		return FormatPhysicalKeyToken(scan, true), scan, true
	}
	return "", 0, false
}

func linuxRecorderKeyName(code uint16) (string, bool) {
	m := map[uint16]string{
		1: "ESC", 2: "1", 3: "2", 4: "3", 5: "4", 6: "5", 7: "6", 8: "7", 9: "8", 10: "9", 11: "0", 12: "MINUS", 13: "EQUALS", 14: "BACKSPACE", 15: "TAB",
		16: "Q", 17: "W", 18: "E", 19: "R", 20: "T", 21: "Y", 22: "U", 23: "I", 24: "O", 25: "P", 26: "LEFTBRACKET", 27: "RIGHTBRACKET", 28: "ENTER", 29: "LEFTCONTROL",
		30: "A", 31: "S", 32: "D", 33: "F", 34: "G", 35: "H", 36: "J", 37: "K", 38: "L", 39: "SEMICOLON", 40: "APOSTROPHE", 41: "GRAVE", 42: "LEFTSHIFT", 43: "BACKSLASH",
		44: "Z", 45: "X", 46: "C", 47: "V", 48: "B", 49: "N", 50: "M", 51: "COMMA", 52: "PERIOD", 53: "SLASH", 54: "RIGHTSHIFT", 55: "MULTIPLY", 56: "LEFTALT", 57: "SPACE",
		59: "F1", 60: "F2", 61: "F3", 62: "F4", 63: "F5", 64: "F6", 65: "F7", 66: "F8", 67: "F9", 68: "F10", 71: "NUMPAD7", 72: "NUMPAD8", 73: "NUMPAD9", 74: "SUBTRACT", 75: "NUMPAD4", 76: "NUMPAD5", 77: "NUMPAD6", 78: "ADD", 79: "NUMPAD1", 80: "NUMPAD2", 81: "NUMPAD3", 82: "NUMPAD0", 83: "DECIMAL", 87: "F11", 88: "F12",
		96: "NUMPADENTER", 97: "RIGHTCONTROL", 98: "DIVIDE", 100: "RIGHTALT", 102: "HOME", 103: "UP", 104: "PAGEUP", 105: "LEFT", 106: "RIGHT", 107: "END", 108: "DOWN", 109: "PAGEDOWN", 110: "INSERT", 111: "DELETE",
	}
	v, ok := m[code]
	return v, ok
}
