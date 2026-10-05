from pathlib import Path
import re

ROOT = Path.cwd()


def read(path):
    return (ROOT / path).read_text(encoding="utf-8")


def write(path, content):
    p = ROOT / path
    p.parent.mkdir(parents=True, exist_ok=True)
    p.write_text(content, encoding="utf-8", newline="\n")
    print(f"wrote {path}")


def replace_once(path, old, new):
    text = read(path)
    count = text.count(old)
    if count != 1:
        raise SystemExit(f"{path}: expected exactly one match, found {count}: {old[:100]!r}")
    write(path, text.replace(old, new, 1))


def regex_replace_once(path, pattern, replacement, flags=0):
    text = read(path)
    new, count = re.subn(pattern, replacement, text, count=1, flags=flags)
    if count != 1:
        raise SystemExit(f"{path}: regex expected exactly one match, found {count}: {pattern[:100]!r}")
    write(path, new)


# ---------------------------------------------------------------------------
# Build identity and version comparison.
# ---------------------------------------------------------------------------
write("internal/buildinfo/version.go", r'''package buildinfo

import (
    "strconv"
    "strings"
)

const (
    Version    = "0.2.10-alpha"
    Display    = "Alpha 0.2.10 SDK8 Recovery"
    Repository = "pseudo6626/JACoB"
)

// CompareVersion compares dotted semantic-ish versions used by JACoB.
// Numeric core components are compared left to right, so future recovery
// versions such as 0.2.10.1 remain meaningful. A release without a prerelease
// suffix sorts after the same numeric version with a suffix.
func CompareVersion(a, b string) int {
    acore, apre := parseVersion(a)
    bcore, bpre := parseVersion(b)
    n := len(acore)
    if len(bcore) > n {
        n = len(bcore)
    }
    for i := 0; i < n; i++ {
        av, bv := 0, 0
        if i < len(acore) {
            av = acore[i]
        }
        if i < len(bcore) {
            bv = bcore[i]
        }
        if av > bv {
            return 1
        }
        if av < bv {
            return -1
        }
    }
    if apre == bpre {
        return 0
    }
    if apre == "" {
        return 1
    }
    if bpre == "" {
        return -1
    }
    if apre > bpre {
        return 1
    }
    return -1
}

func parseVersion(v string) ([]int, string) {
    v = strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(v, "v"), "V"))
    core, pre, _ := strings.Cut(v, "-")
    parts := strings.Split(core, ".")
    values := make([]int, 0, len(parts))
    for _, part := range parts {
        n, _ := strconv.Atoi(part)
        values = append(values, n)
    }
    for len(values) > 1 && values[len(values)-1] == 0 {
        values = values[:len(values)-1]
    }
    return values, strings.ToLower(pre)
}
''')

write("internal/buildinfo/version_test.go", r'''package buildinfo

import "testing"

func TestCompareVersion(t *testing.T) {
    cases := []struct {
        a, b string
        want int
    }{
        {"v0.2.8-alpha", "0.2.3-alpha", 1},
        {"0.2.10-alpha", "0.2.9-alpha", 1},
        {"0.2.8", "0.2.8-alpha", 1},
        {"0.2.8-alpha", "0.2.8-alpha", 0},
        {"0.2.3-alpha", "0.2.8-alpha", -1},
        {"0.2.9.1-alpha", "0.2.9-alpha", 1},
        {"0.2.9-alpha", "0.2.9.1-alpha", -1},
        {"0.2.10.0-alpha", "0.2.10-alpha", 0},
    }
    for _, c := range cases {
        if got := CompareVersion(c.a, c.b); got != c.want {
            t.Fatalf("CompareVersion(%q,%q)=%d want %d", c.a, c.b, got, c.want)
        }
    }
}
''')

# ---------------------------------------------------------------------------
# Physical-key-first input contract. SC tokens are PC/AT set-1 scan codes.
# Examples: SC:29, SC:56, SC:E0:38.
# ---------------------------------------------------------------------------
write("internal/platform/input.go", r'''package platform

import (
    "fmt"
    "sort"
    "strconv"
    "strings"
)

type InputDriver interface {
    Name() string
    Available() bool
    FocusGame() error
    TapKey(key string) error
    TapChord(key string, modifiers []string) error
    HoldChord(key string, modifiers []string, durationMs int) error
    ChordDown(key string, modifiers []string) error
    ChordUp(key string, modifiers []string) error
}

func NewInputDriver() InputDriver { return newInputDriver() }

type PhysicalKey struct {
    Scan     uint16
    Extended bool
}

// ParsePhysicalKeyToken accepts canonical physical-key identifiers such as
// SC:29, SC:56, and SC:E0:38. Scan-code bytes are hexadecimal because that is
// how Windows and Elite binding diagnostics conventionally present them.
func ParsePhysicalKeyToken(raw string) (PhysicalKey, bool) {
    parts := strings.Split(strings.ToUpper(strings.TrimSpace(raw)), ":")
    if len(parts) < 2 || parts[0] != "SC" {
        return PhysicalKey{}, false
    }
    extended := false
    scanPart := ""
    switch len(parts) {
    case 2:
        scanPart = parts[1]
    case 3:
        if parts[1] != "E0" {
            return PhysicalKey{}, false
        }
        extended = true
        scanPart = parts[2]
    default:
        return PhysicalKey{}, false
    }
    n, err := strconv.ParseUint(scanPart, 16, 8)
    if err != nil || n == 0 {
        return PhysicalKey{}, false
    }
    return PhysicalKey{Scan: uint16(n), Extended: extended}, true
}

func FormatPhysicalKeyToken(scan uint32, extended bool) string {
    if scan == 0 || scan > 0xff {
        return ""
    }
    if extended {
        return fmt.Sprintf("SC:E0:%02X", scan)
    }
    return fmt.Sprintf("SC:%02X", scan)
}

func securityKeyName(key string) string {
    k := strings.ToUpper(strings.TrimSpace(key))
    switch k {
    case "CONTROL", "LEFTCONTROL":
        return "CTRL"
    case "LEFTALT":
        return "ALT"
    case "LEFTSHIFT":
        return "SHIFT"
    case "ESCAPE":
        return "ESC"
    }
    if physical, ok := ParsePhysicalKeyToken(k); ok {
        switch {
        case !physical.Extended && physical.Scan == 0x01:
            return "ESC"
        case !physical.Extended && physical.Scan == 0x0f:
            return "TAB"
        case !physical.Extended && physical.Scan == 0x3e:
            return "F4"
        case !physical.Extended && physical.Scan == 0x1d:
            return "CTRL"
        case physical.Extended && physical.Scan == 0x1d:
            return "CTRL"
        case !physical.Extended && physical.Scan == 0x38:
            return "ALT"
        case physical.Extended && physical.Scan == 0x38:
            return "ALT"
        case !physical.Extended && (physical.Scan == 0x2a || physical.Scan == 0x36):
            return "SHIFT"
        }
    }
    return k
}

// ValidateSafeChord rejects operating-system/global shortcuts that can escape
// the Elite Dangerous window even when Elite is foreground. Physical SC tokens
// are normalized before this test so scan-code input cannot bypass the guard.
func ValidateSafeChord(key string, modifiers []string) error {
    k := securityKeyName(key)
    mods := map[string]bool{}
    for _, m := range modifiers {
        if s := securityKeyName(m); s != "" {
            mods[s] = true
        }
    }
    unsafe := false
    switch {
    case mods["ALT"] && (k == "TAB" || k == "ESC" || k == "F4"):
        unsafe = true
    case mods["CTRL"] && k == "ESC":
        unsafe = true
    case mods["CTRL"] && mods["SHIFT"] && k == "ESC":
        unsafe = true
    }
    if unsafe {
        names := make([]string, 0, len(mods))
        for m := range mods {
            names = append(names, m)
        }
        sort.Strings(names)
        chord := strings.Join(names, "+")
        if chord != "" {
            chord += "+"
        }
        return fmt.Errorf("OS-global shortcut %s%s is blocked by JACoB", chord, k)
    }
    return nil
}
''')

write("internal/platform/input_windows.go", r'''//go:build windows

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
    keyeventfUnicode             = 0x0004
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
    if physical, ok := ParsePhysicalKeyToken(k); ok {
        return resolvedWindowsKey{scan: physical.Scan, extended: physical.Extended, name: FormatPhysicalKeyToken(uint32(physical.Scan), physical.Extended)}, nil
    }
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

func makeWindowsUnicodeEvent(unit uint16, up bool) winInput {
    var in winInput
    in.Type = inputKeyboard
    ki := (*keyboardInput)(unsafe.Pointer(&in.Data[0]))
    ki.Vk = 0
    ki.Scan = unit
    ki.ExtraInfo = jacobInputMarker
    ki.Flags = keyeventfUnicode
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
        "CAPSLOCK": {0x14, false}, "PAUSE": {0x13, false}, "PRINTSCREEN": {0x2C, true}, "SCROLLLOCK": {0x91, false}, "NUMLOCK": {0x90, true},
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
        "OEM102": {0xE2, false}, "KANA": {0x15, false}, "KANJI": {0x19, false}, "CONVERT": {0x1C, false}, "NONCONVERT": {0x1D, false},
        "F1": {0x70, false}, "F2": {0x71, false}, "F3": {0x72, false}, "F4": {0x73, false}, "F5": {0x74, false}, "F6": {0x75, false},
        "F7": {0x76, false}, "F8": {0x77, false}, "F9": {0x78, false}, "F10": {0x79, false}, "F11": {0x7A, false}, "F12": {0x7B, false},
    }
    v, found := keys[k]
    if !found {
        return 0, false, false
    }
    return v.vk, v.extended, true
}
''')

# Linux keeps its mature focus/uinput implementation; only key resolution needs
# to learn the canonical SC token format.
replace_once(
    "internal/platform/input_linux.go",
    '''\tfor _, m := range modifiers {\n\t\tcode, ok := evdevKey(m)\n\t\tif !ok {\n\t\t\treturn 0, nil, fmt.Errorf("unsupported Linux key %q", m)\n\t\t}\n\t\tmods = append(mods, code)\n\t}\n\tmain, ok := evdevKey(key)\n\tif !ok {\n\t\treturn 0, nil, fmt.Errorf("unsupported Linux key %q", key)\n\t}\n''',
    '''\tfor _, m := range modifiers {\n\t\tcode, ok := resolveLinuxKey(m)\n\t\tif !ok {\n\t\t\treturn 0, nil, fmt.Errorf("unsupported Linux key %q", m)\n\t\t}\n\t\tmods = append(mods, code)\n\t}\n\tmain, ok := resolveLinuxKey(key)\n\tif !ok {\n\t\treturn 0, nil, fmt.Errorf("unsupported Linux key %q", key)\n\t}\n''',
)

linux_append = r'''

func resolveLinuxKey(key string) (uint16, bool) {
    if physical, ok := ParsePhysicalKeyToken(key); ok {
        return set1ScanToEvdev(physical)
    }
    return evdevKey(key)
}

// set1ScanToEvdev translates the canonical PC/AT set-1 physical identity used
// by JACoB into Linux input-event key codes. The base keyboard block largely
// retains the same numeric positions; extended/navigation and international
// keys need explicit translation.
func set1ScanToEvdev(k PhysicalKey) (uint16, bool) {
    if !k.Extended {
        if k.Scan >= 0x01 && k.Scan <= 0x58 {
            return k.Scan, true
        }
        switch k.Scan {
        case 0x70: // JIS Kana / Katakana-Hiragana
            return 93, true
        case 0x73: // JIS Ro / ISO international position
            return 89, true
        case 0x79: // JIS Henkan / Convert
            return 92, true
        case 0x7B: // JIS Muhenkan / Non-convert
            return 94, true
        case 0x7D: // JIS Yen
            return 124, true
        case 0x7E: // ABNT/JIS keypad comma position
            return 121, true
        }
        return 0, false
    }
    switch k.Scan {
    case 0x1C:
        return 96, true // keypad enter
    case 0x1D:
        return 97, true // right ctrl
    case 0x35:
        return 98, true // keypad slash
    case 0x37:
        return 99, true // print/sysrq
    case 0x38:
        return 100, true // right alt / AltGr
    case 0x47:
        return 102, true
    case 0x48:
        return 103, true
    case 0x49:
        return 104, true
    case 0x4B:
        return 105, true
    case 0x4D:
        return 106, true
    case 0x4F:
        return 107, true
    case 0x50:
        return 108, true
    case 0x51:
        return 109, true
    case 0x52:
        return 110, true
    case 0x53:
        return 111, true
    case 0x5B:
        return 125, true // left meta
    case 0x5C:
        return 126, true // right meta
    case 0x5D:
        return 127, true // menu/compose
    }
    return 0, false
}
'''
linux_text = read("internal/platform/input_linux.go")
if "func resolveLinuxKey(" not in linux_text:
    write("internal/platform/input_linux.go", linux_text.rstrip() + linux_append + "\n")
else:
    raise SystemExit("input_linux.go already contains resolveLinuxKey; refusing duplicate patch")

# ---------------------------------------------------------------------------
# Layout-aware text entry. Windows uses the Elite window's active keyboard
# layout and falls back to Unicode SendInput for characters the layout cannot
# directly produce. Linux uses xdotool when present, otherwise the old ASCII
# mapper remains as a safe fallback.
# ---------------------------------------------------------------------------
write("internal/platform/text.go", r'''package platform

import (
    "fmt"
    "time"
)

func TypeText(d InputDriver, text string, interval time.Duration) error {
    if handled, err := typeTextNative(d, text, interval); handled {
        return err
    }
    for _, r := range text {
        key, mods, ok := textRune(r)
        if !ok {
            return fmt.Errorf("text character %q is not supported by the JACoB keyboard mapper on this host", r)
        }
        if err := d.TapChord(key, mods); err != nil {
            return err
        }
        if interval > 0 {
            time.Sleep(interval)
        }
    }
    return nil
}

// textRune remains for compatibility and for hosts without a native
// layout-aware text path. Physical-control APIs do not use this mapping.
func textRune(r rune) (string, []string, bool) {
    if r >= 'a' && r <= 'z' {
        return string(r - 'a' + 'A'), nil, true
    }
    if r >= 'A' && r <= 'Z' {
        return string(r), []string{"SHIFT"}, true
    }
    if r >= '0' && r <= '9' {
        return string(r), nil, true
    }
    switch r {
    case ' ':
        return "SPACE", nil, true
    case '-':
        return "MINUS", nil, true
    case '_':
        return "MINUS", []string{"SHIFT"}, true
    case '=':
        return "EQUALS", nil, true
    case '+':
        return "EQUALS", []string{"SHIFT"}, true
    case '.':
        return "PERIOD", nil, true
    case ',':
        return "COMMA", nil, true
    case '/':
        return "SLASH", nil, true
    case '?':
        return "SLASH", []string{"SHIFT"}, true
    case '\\':
        return "BACKSLASH", nil, true
    case ';':
        return "SEMICOLON", nil, true
    case ':':
        return "SEMICOLON", []string{"SHIFT"}, true
    case '\'':
        return "APOSTROPHE", nil, true
    case '"':
        return "APOSTROPHE", []string{"SHIFT"}, true
    case '[':
        return "LEFTBRACKET", nil, true
    case '{':
        return "LEFTBRACKET", []string{"SHIFT"}, true
    case ']':
        return "RIGHTBRACKET", nil, true
    case '}':
        return "RIGHTBRACKET", []string{"SHIFT"}, true
    case '!':
        return "1", []string{"SHIFT"}, true
    case '@':
        return "2", []string{"SHIFT"}, true
    case '#':
        return "3", []string{"SHIFT"}, true
    case '$':
        return "4", []string{"SHIFT"}, true
    case '%':
        return "5", []string{"SHIFT"}, true
    case '^':
        return "6", []string{"SHIFT"}, true
    case '&':
        return "7", []string{"SHIFT"}, true
    case '*':
        return "8", []string{"SHIFT"}, true
    case '(':
        return "9", []string{"SHIFT"}, true
    case ')':
        return "0", []string{"SHIFT"}, true
    case '\n':
        return "ENTER", nil, true
    case '\t':
        return "TAB", nil, true
    }
    return "", nil, false
}
''')

write("internal/platform/text_windows.go", r'''//go:build windows

package platform

import (
    "fmt"
    "syscall"
    "time"
    "unicode/utf16"
)

const mapvkVkToVscEx = 4

var (
    procGetKeyboardLayout = user32.NewProc("GetKeyboardLayout")
    procVkKeyScanExW      = user32.NewProc("VkKeyScanExW")
    procMapVirtualKeyExW  = user32.NewProc("MapVirtualKeyExW")
)

func typeTextNative(d InputDriver, text string, interval time.Duration) (bool, error) {
    if _, ok := d.(*windowsInput); !ok {
        return false, nil
    }
    hwnd, _, err := findEliteWindow()
    if err != nil || hwnd == 0 {
        if err != nil {
            return true, err
        }
        return true, fmt.Errorf("Elite Dangerous window was not found")
    }
    threadID := windowThreadID(hwnd)
    layout, _, _ := procGetKeyboardLayout.Call(uintptr(threadID))

    for _, r := range text {
        switch r {
        case '\r':
            continue
        case '\n':
            if err := d.TapKey("ENTER"); err != nil {
                return true, err
            }
        case '\t':
            if err := d.TapKey("TAB"); err != nil {
                return true, err
            }
        default:
            token, mods, ok := windowsLayoutRune(r, layout)
            if ok {
                if err := d.TapChord(token, mods); err != nil {
                    return true, err
                }
            } else if err := sendWindowsUnicodeRune(r); err != nil {
                return true, fmt.Errorf("type Unicode character %q: %w", r, err)
            }
        }
        if interval > 0 {
            time.Sleep(interval)
        }
    }
    return true, nil
}

func windowsLayoutRune(r rune, layout uintptr) (string, []string, bool) {
    if r < 0 || r > 0xffff || (r >= 0xd800 && r <= 0xdfff) {
        return "", nil, false
    }
    raw, _, _ := procVkKeyScanExW.Call(uintptr(uint16(r)), layout)
    mapped := uint16(raw & 0xffff)
    if mapped == 0xffff {
        return "", nil, false
    }
    vk := uint16(mapped & 0xff)
    shiftState := byte(mapped >> 8)
    if shiftState&0x08 != 0 { // layout-specific Hankaku state; Unicode fallback is safer
        return "", nil, false
    }
    scanRaw, _, _ := procMapVirtualKeyExW.Call(uintptr(vk), mapvkVkToVscEx, layout)
    if scanRaw == 0 {
        return "", nil, false
    }
    scan := uint32(scanRaw & 0xff)
    prefix := byte((scanRaw >> 8) & 0xff)
    extended := prefix == 0xe0
    token := FormatPhysicalKeyToken(scan, extended)
    if token == "" {
        return "", nil, false
    }
    mods := []string{}
    if shiftState&0x01 != 0 {
        mods = append(mods, "SHIFT")
    }
    // Windows reports AltGr layouts as Ctrl+Alt. Right Alt is the physical
    // AltGr key and lets the active layout perform its normal synthesis.
    if shiftState&0x06 == 0x06 {
        mods = append(mods, "RIGHTALT")
    } else {
        if shiftState&0x02 != 0 {
            mods = append(mods, "CTRL")
        }
        if shiftState&0x04 != 0 {
            mods = append(mods, "ALT")
        }
    }
    return token, mods, true
}

func sendWindowsUnicodeRune(r rune) error {
    units := utf16.Encode([]rune{r})
    if len(units) == 0 {
        return nil
    }
    events := make([]winInput, 0, len(units)*2)
    for _, unit := range units {
        events = append(events, makeWindowsUnicodeEvent(unit, false), makeWindowsUnicodeEvent(unit, true))
    }
    if err := sendInputBatch(events); err != nil {
        return err
    }
    return nil
}

var _ = syscall.Errno(0)
''')

# syscall is not actually needed above; remove the deliberately-unused guard by
# keeping the import list minimal after gofmt validation.
text_windows = read("internal/platform/text_windows.go")
text_windows = text_windows.replace('    "syscall"\n', '').replace('\nvar _ = syscall.Errno(0)\n', '\n')
write("internal/platform/text_windows.go", text_windows)

write("internal/platform/text_linux.go", r'''//go:build linux

package platform

import (
    "fmt"
    "os/exec"
    "strconv"
    "strings"
    "time"
)

func typeTextNative(d InputDriver, text string, interval time.Duration) (bool, error) {
    if _, ok := d.(*linuxInput); !ok {
        return false, nil
    }
    path, err := exec.LookPath("xdotool")
    if err != nil {
        return false, nil
    }
    delay := interval.Milliseconds()
    if delay < 0 {
        delay = 0
    }
    if delay > 1000 {
        delay = 1000
    }
    args := []string{"type", "--clearmodifiers", "--delay", strconv.FormatInt(delay, 10), "--", text}
    out, err := exec.Command(path, args...).CombinedOutput()
    if err != nil {
        detail := strings.TrimSpace(string(out))
        if detail != "" {
            return true, fmt.Errorf("xdotool text input failed: %w: %s", err, detail)
        }
        return true, fmt.Errorf("xdotool text input failed: %w", err)
    }
    return true, nil
}
''')

write("internal/platform/text_other.go", r'''//go:build !windows && !linux

package platform

import "time"

func typeTextNative(d InputDriver, text string, interval time.Duration) (bool, error) {
    return false, nil
}
''')

# ---------------------------------------------------------------------------
# Recorder events retain physical identity and localized display text while
# preserving the existing key/modifier fields for old tabs.
# ---------------------------------------------------------------------------
write("internal/platform/recorder.go", r'''package platform

import "sync"

type RecordedInputEvent struct {
    PressID       uint64   `json:"pressId"`
    Type          string   `json:"type"`
    Key           string   `json:"key"`
    Physical      string   `json:"physical,omitempty"`
    LocalizedName string   `json:"localizedName,omitempty"`
    ScanCode      uint32   `json:"scanCode,omitempty"`
    VirtualKey    uint32   `json:"virtualKey,omitempty"`
    Extended      bool     `json:"extended,omitempty"`
    Modifiers     []string `json:"modifiers,omitempty"`
    AtMs          int64    `json:"atMs"`
    DeltaMs       int64    `json:"deltaMs"`
    DurationMs    int64    `json:"durationMs,omitempty"`
    IsModifier    bool     `json:"isModifier,omitempty"`
}

type RecorderStatus struct {
    Available  bool   `json:"available"`
    Driver     string `json:"driver"`
    Recording  bool   `json:"recording"`
    EventCount int    `json:"eventCount"`
    Scope      string `json:"scope"`
}

type InputRecorder interface {
    Name() string
    Available() bool
    Start(func(RecordedInputEvent)) error
    Stop() ([]RecordedInputEvent, error)
    Status() RecorderStatus
}

func NewInputRecorder() InputRecorder { return newInputRecorder() }

type recorderBase struct {
    mu sync.Mutex
}
''')

replace_once(
    "internal/platform/recorder_windows.go",
    '''\tllkhfLowerILInjected = 0x00000002\n\tllkhfInjected        = 0x00000010\n''',
    '''\tllkhfExtended        = 0x00000001\n\tllkhfLowerILInjected = 0x00000002\n\tllkhfInjected        = 0x00000010\n''',
)
replace_once(
    "internal/platform/recorder_windows.go",
    '''type activePress struct {\n\tid        uint64\n\tkey       string\n\tmodifiers []string\n\tatMs      int64\n\tisMod     bool\n}\n''',
    '''type activePress struct {\n\tid            uint64\n\tkey           string\n\tphysical      string\n\tlocalizedName string\n\tscanCode      uint32\n\tvirtualKey    uint32\n\textended      bool\n\tmodifiers     []string\n\tatMs          int64\n\tisMod         bool\n}\n''',
)
replace_once(
    "internal/platform/recorder_windows.go",
    '''\tprocPostThreadMessageW  = user32.NewProc("PostThreadMessageW")\n''',
    '''\tprocPostThreadMessageW  = user32.NewProc("PostThreadMessageW")\n\tprocGetKeyNameTextW      = user32.NewProc("GetKeyNameTextW")\n''',
)
replace_once(
    "internal/platform/recorder_windows.go",
    '''\t\t\t\tswitch uint32(wParam) {\n\t\t\t\tcase wmKeyDown, wmSysKeyDown:\n\t\t\t\t\tr.recordKey(k.VkCode, true)\n\t\t\t\tcase wmKeyUp, wmSysKeyUp:\n\t\t\t\t\tr.recordKey(k.VkCode, false)\n\t\t\t\t}\n''',
    '''\t\t\t\textended := k.Flags&llkhfExtended != 0\n\t\t\t\tswitch uint32(wParam) {\n\t\t\t\tcase wmKeyDown, wmSysKeyDown:\n\t\t\t\t\tr.recordKey(k.VkCode, k.ScanCode, extended, true)\n\t\t\t\tcase wmKeyUp, wmSysKeyUp:\n\t\t\t\t\tr.recordKey(k.VkCode, k.ScanCode, extended, false)\n\t\t\t\t}\n''',
)

regex_replace_once(
    "internal/platform/recorder_windows.go",
    r'''func \(r \*windowsRecorder\) recordKey\(vk uint32, down bool\) \{.*?\n\}\n\nfunc \(r \*windowsRecorder\) currentModifiersLocked''',
    r'''func windowsPressIdentity(scan uint32, extended bool, vk uint32) uint32 {
    if scan == 0 {
        return 0x80000000 | vk
    }
    if extended {
        return 0x10000 | scan
    }
    return scan
}

func localizedWindowsKeyName(scan uint32, extended bool) string {
    if scan == 0 {
        return ""
    }
    lparam := uintptr((scan & 0xff) << 16)
    if extended {
        lparam |= 1 << 24
    }
    buf := make([]uint16, 128)
    n, _, _ := procGetKeyNameTextW.Call(lparam, uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)))
    if n == 0 {
        return ""
    }
    return syscall.UTF16ToString(buf[:n])
}

func (r *windowsRecorder) recordKey(vk uint32, scan uint32, extended bool, down bool) {
    if scan == 0 {
        if mapped, _, _ := procMapVirtualKeyW.Call(uintptr(vk), mapvkVkToVsc); mapped != 0 {
            scan = uint32(mapped & 0xff)
        }
    }
    physical := FormatPhysicalKeyToken(scan, extended)
    key, known := normalizedVK(vk)
    if !known {
        key = physical
    }
    if key == "" {
        return
    }
    localizedName := localizedWindowsKeyName(scan, extended)
    isMod := isModifierKey(key)
    identity := windowsPressIdentity(scan, extended, vk)

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
        if _, exists := r.pressed[identity]; exists {
            r.mu.Unlock()
            return
        }
        mods := r.currentModifiersLocked()
        id := atomic.AddUint64(&r.seq, 1)
        p := activePress{id: id, key: key, physical: physical, localizedName: localizedName, scanCode: scan, virtualKey: vk, extended: extended, modifiers: mods, atMs: now, isMod: isMod}
        r.pressed[identity] = p
        ev := RecordedInputEvent{PressID: id, Type: "down", Key: key, Physical: physical, LocalizedName: localizedName, ScanCode: scan, VirtualKey: vk, Extended: extended, Modifiers: append([]string(nil), mods...), AtMs: now, DeltaMs: delta, IsModifier: isMod}
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

    p, exists := r.pressed[identity]
    if !exists {
        if isMod {
            delete(r.mods, key)
        }
        r.mu.Unlock()
        return
    }
    delete(r.pressed, identity)
    if p.isMod {
        delete(r.mods, p.key)
    }
    dur := now - p.atMs
    if dur < 0 {
        dur = 0
    }
    ev := RecordedInputEvent{PressID: p.id, Type: "up", Key: p.key, Physical: p.physical, LocalizedName: p.localizedName, ScanCode: p.scanCode, VirtualKey: p.virtualKey, Extended: p.extended, Modifiers: append([]string(nil), p.modifiers...), AtMs: now, DeltaMs: delta, DurationMs: dur, IsModifier: p.isMod}
    r.lastAtMs = now
    r.events = append(r.events, ev)
    cb := r.callback
    r.mu.Unlock()
    if cb != nil {
        cb(ev)
    }
}

func (r *windowsRecorder) currentModifiersLocked''',
    flags=re.S,
)

# Add a few useful international/OEM names without changing legacy names.
replace_once(
    "internal/platform/recorder_windows.go",
    '''\t\t0xBA: "SEMICOLON", 0xBB: "EQUALS", 0xBC: "COMMA", 0xBD: "MINUS", 0xBE: "PERIOD", 0xBF: "SLASH", 0xC0: "GRAVE", 0xDB: "LEFTBRACKET", 0xDC: "BACKSLASH", 0xDD: "RIGHTBRACKET", 0xDE: "APOSTROPHE",\n''',
    '''\t\t0xBA: "SEMICOLON", 0xBB: "EQUALS", 0xBC: "COMMA", 0xBD: "MINUS", 0xBE: "PERIOD", 0xBF: "SLASH", 0xC0: "GRAVE", 0xDB: "LEFTBRACKET", 0xDC: "BACKSLASH", 0xDD: "RIGHTBRACKET", 0xDE: "APOSTROPHE",\n\t\t0xE2: "OEM102", 0x15: "KANA", 0x19: "KANJI", 0x1C: "CONVERT", 0x1D: "NONCONVERT",\n''',
)

# ---------------------------------------------------------------------------
# Generic tests for physical tokens and safety. Windows runner also gets a
# direct resolver test.
# ---------------------------------------------------------------------------
write("internal/platform/input_physical_test.go", r'''package platform

import "testing"

func TestPhysicalKeyTokenRoundTrip(t *testing.T) {
    cases := []struct {
        token    string
        scan     uint16
        extended bool
    }{
        {"SC:29", 0x29, false},
        {"sc:56", 0x56, false},
        {"SC:E0:38", 0x38, true},
    }
    for _, c := range cases {
        got, ok := ParsePhysicalKeyToken(c.token)
        if !ok || got.Scan != c.scan || got.Extended != c.extended {
            t.Fatalf("ParsePhysicalKeyToken(%q)=%+v,%v", c.token, got, ok)
        }
        if ParsePhysicalKeyTokenMust := FormatPhysicalKeyToken(uint32(got.Scan), got.Extended); ParsePhysicalKeyTokenMust == "" {
            t.Fatalf("FormatPhysicalKeyToken(%q) returned empty", c.token)
        }
    }
}

func TestPhysicalShortcutSafety(t *testing.T) {
    blocked := []struct {
        key  string
        mods []string
    }{
        {"SC:0F", []string{"SC:38"}},       // Alt+Tab
        {"SC:01", []string{"SC:1D"}},       // Ctrl+Esc
        {"SC:3E", []string{"SC:E0:38"}},    // AltGr/Alt+F4 physical expression
    }
    for _, c := range blocked {
        if err := ValidateSafeChord(c.key, c.mods); err == nil {
            t.Fatalf("expected physical shortcut to be blocked: %v + %s", c.mods, c.key)
        }
    }
    if err := ValidateSafeChord("SC:10", []string{"SC:E0:38"}); err != nil {
        t.Fatalf("ordinary AltGr chord was blocked: %v", err)
    }
}
''')

write("internal/platform/input_windows_test.go", r'''//go:build windows

package platform

import "testing"

func TestResolveWindowsPhysicalKey(t *testing.T) {
    got, err := resolveWindowsKey("SC:E0:38")
    if err != nil {
        t.Fatal(err)
    }
    if got.scan != 0x38 || !got.extended {
        t.Fatalf("resolved=%+v", got)
    }
}
''')

# ---------------------------------------------------------------------------
# Persist LAN pairing credentials and advertise SDK 8.
# ---------------------------------------------------------------------------
replace_once(
    "internal/core/server.go",
    '''\tif cfg.LANEnabled && cfg.PairToken == "" {\n\t\ttoken, err := secureRandomToken()\n''',
    '''\tif cfg.LANEnabled && cfg.PairToken == "" {\n\t\ttoken, err := persistentPairToken(cfg.DataDir)\n''',
)

server_text = read("internal/core/server.go")
if '"apiVersion": 7' not in server_text:
    raise SystemExit('server.go: apiVersion 7 marker not found')
server_text = server_text.replace('"apiVersion": 7', '"apiVersion": 8', 1)
if "func persistentPairToken(" in server_text:
    raise SystemExit("server.go already has persistentPairToken")
server_text = server_text.rstrip() + r'''

func persistentPairToken(dataDir string) (string, error) {
    path := filepath.Join(dataDir, "pairing-token.txt")
    if b, err := os.ReadFile(path); err == nil {
        token := strings.TrimSpace(string(b))
        if len(token) >= 16 {
            return token, nil
        }
    }
    token, err := secureRandomToken()
    if err != nil {
        return "", err
    }
    if strings.TrimSpace(dataDir) == "" {
        return token, nil
    }
    if err := os.MkdirAll(dataDir, 0o700); err != nil {
        return "", err
    }
    tmp := path + ".tmp"
    if err := os.WriteFile(tmp, []byte(token+"\n"), 0o600); err != nil {
        return "", err
    }
    _ = os.Remove(path)
    if err := os.Rename(tmp, path); err != nil {
        _ = os.Remove(tmp)
        return "", err
    }
    return token, nil
}
''' + "\n"
write("internal/core/server.go", server_text)

# ---------------------------------------------------------------------------
# Windows installer: keep previous executable and verify the restarted core is
# actually the version embedded in this installer. Roll back on mismatch.
# ---------------------------------------------------------------------------
replace_once(
    "cmd/installer/main.go",
    '''import (\n\t"embed"\n\t"fmt"\n''',
    '''import (\n\t"embed"\n\t"encoding/json"\n\t"fmt"\n\t"net/http"\n''',
)

old_auto = '''\tif opts.autoUpdate {\n\t\tif opts.waitPID > 0 {\n\t\t\twaitForPID(opts.waitPID, 30*time.Second)\n\t\t}\n\t\tincludeRecorder := installedRecorderEnabled()\n\t\tif opts.recorderSet {\n\t\t\tincludeRecorder = opts.recorder\n\t\t}\n\t\tif err := install(includeRecorder); err != nil {\n\t\t\tmsg("JACoB Update", "Update failed:\\n\\n"+err.Error(), mbOK|mbIconWarning)\n\t\t\treturn\n\t\t}\n\t\t_ = exec.Command(filepath.Join(installDir(), "JACoB.exe")).Start()\n\t\treturn\n\t}\n'''
new_auto = '''\tif opts.autoUpdate {\n\t\tpreviousVersion := installedVersion()\n\t\tif opts.waitPID > 0 {\n\t\t\twaitForPID(opts.waitPID, 30*time.Second)\n\t\t}\n\t\tincludeRecorder := installedRecorderEnabled()\n\t\tif opts.recorderSet {\n\t\t\tincludeRecorder = opts.recorder\n\t\t}\n\t\tif err := install(includeRecorder); err != nil {\n\t\t\tmsg("JACoB Update", "Update failed:\\n\\n"+err.Error(), mbOK|mbIconWarning)\n\t\t\treturn\n\t\t}\n\t\tdir := installDir()\n\t\ttarget := filepath.Join(dir, "JACoB.exe")\n\t\tif err := exec.Command(target).Start(); err != nil {\n\t\t\trollbackErr := rollbackExecutable(dir, previousVersion)\n\t\t\tmsg("JACoB Update", rollbackMessage("Updated JACoB could not be started: "+err.Error(), rollbackErr), mbOK|mbIconWarning)\n\t\t\treturn\n\t\t}\n\t\tif err := verifyRunningVersion(buildinfo.Version, 15*time.Second); err != nil {\n\t\t\t_ = hidden("taskkill", "/IM", "JACoB.exe", "/F").Run()\n\t\t\ttime.Sleep(500 * time.Millisecond)\n\t\t\trollbackErr := rollbackExecutable(dir, previousVersion)\n\t\t\tmsg("JACoB Update", rollbackMessage("Updated JACoB failed its startup/version check: "+err.Error(), rollbackErr), mbOK|mbIconWarning)\n\t\t}\n\t\treturn\n\t}\n'''
replace_once("cmd/installer/main.go", old_auto, new_auto)

old_write = '''\tapp, err := payload.ReadFile(appName)\n\tif err != nil {\n\t\treturn err\n\t}\n\tif err := os.WriteFile(filepath.Join(dir, "JACoB.exe"), app, 0o755); err != nil {\n\t\treturn err\n\t}\n'''
new_write = '''\tapp, err := payload.ReadFile(appName)\n\tif err != nil {\n\t\treturn err\n\t}\n\ttarget := filepath.Join(dir, "JACoB.exe")\n\tbackup := target + ".previous"\n\tif old, readErr := os.ReadFile(target); readErr == nil {\n\t\tif err := os.WriteFile(backup, old, 0o755); err != nil {\n\t\t\treturn fmt.Errorf("backup current JACoB executable: %w", err)\n\t\t}\n\t} else if !os.IsNotExist(readErr) {\n\t\treturn fmt.Errorf("read current JACoB executable: %w", readErr)\n\t}\n\tif err := os.WriteFile(target, app, 0o755); err != nil {\n\t\treturn err\n\t}\n'''
replace_once("cmd/installer/main.go", old_write, new_write)

installer_text = read("cmd/installer/main.go")
if "func verifyRunningVersion(" in installer_text:
    raise SystemExit("installer already has verification helpers")
installer_text = installer_text.rstrip() + r'''

func verifyRunningVersion(expected string, timeout time.Duration) error {
    client := http.Client{Timeout: 900 * time.Millisecond}
    deadline := time.Now().Add(timeout)
    var last string
    for time.Now().Before(deadline) {
        resp, err := client.Get("http://127.0.0.1:4510/api/health")
        if err == nil {
            var health struct {
                Product string `json:"product"`
                Version string `json:"version"`
            }
            decodeErr := json.NewDecoder(resp.Body).Decode(&health)
            _ = resp.Body.Close()
            if resp.StatusCode == http.StatusOK && decodeErr == nil && strings.EqualFold(health.Product, "JACoB") {
                if health.Version == expected {
                    return nil
                }
                last = fmt.Sprintf("core reported version %q, expected %q", health.Version, expected)
            }
        }
        time.Sleep(250 * time.Millisecond)
    }
    if last == "" {
        last = "local health endpoint did not become ready"
    }
    return fmt.Errorf("%s", last)
}

func rollbackExecutable(dir, previousVersion string) error {
    target := filepath.Join(dir, "JACoB.exe")
    backup := target + ".previous"
    if _, err := os.Stat(backup); err != nil {
        return fmt.Errorf("previous executable is unavailable: %w", err)
    }
    _ = os.Remove(target + ".failed")
    if _, err := os.Stat(target); err == nil {
        _ = os.Rename(target, target+".failed")
    }
    if err := os.Rename(backup, target); err != nil {
        return fmt.Errorf("restore previous executable: %w", err)
    }
    if strings.TrimSpace(previousVersion) != "" {
        _ = hidden("reg", "add", uninstallKey, "/v", "DisplayVersion", "/t", "REG_SZ", "/d", previousVersion, "/f").Run()
    }
    if err := exec.Command(target).Start(); err != nil {
        return fmt.Errorf("restart previous JACoB: %w", err)
    }
    return nil
}

func rollbackMessage(reason string, rollbackErr error) string {
    if rollbackErr == nil {
        return reason + "\n\nThe previous JACoB executable was restored and restarted."
    }
    return reason + "\n\nAutomatic rollback also failed: " + rollbackErr.Error()
}
''' + "\n"
write("cmd/installer/main.go", installer_text)

# ---------------------------------------------------------------------------
# Update badge and periodic checks. Same available version stays dismissed on
# this browser after Settings is opened; a later newer version re-arms it.
# ---------------------------------------------------------------------------
replace_once(
    "internal/webui/assets/app.js",
    "api:Object.freeze({version:7})",
    "api:Object.freeze({version:8})",
)

replace_once(
    "internal/webui/assets/app.js",
    '''  const isLocal = ['127.0.0.1','localhost','::1'].includes(location.hostname);\n''',
    '''  const isLocal = ['127.0.0.1','localhost','::1'].includes(location.hostname);\n  const UPDATE_CHECK_INTERVAL_MS=30*60*1000,UPDATE_BADGE_KEY='jacob-update-badge-dismissed';\n  let updateCheckTimer=null,updateCheckKickoff=null;\n  function dismissedUpdateVersion(){try{return localStorage.getItem(UPDATE_BADGE_KEY)||''}catch{return''}}\n  function updateBadgeVisible(){const i=state.updateInfo;return !!(i?.available&&i.latestVersion&&dismissedUpdateVersion()!==String(i.latestVersion))}\n  function decorateSettingsUpdateBadge(btn){if(!btn||!updateBadgeVisible())return;const badge=document.createElement('span');badge.className='settings-update-badge';badge.textContent='NEW';badge.setAttribute('aria-hidden','true');btn.appendChild(badge);btn.setAttribute('aria-label',`${navLabel('settings')} — ${tr('update available')}`)}\n  function dismissSettingsUpdateBadge(){const i=state.updateInfo;if(i?.available&&i.latestVersion){try{localStorage.setItem(UPDATE_BADGE_KEY,String(i.latestVersion))}catch{}}renderNavigation()}\n  function queueUpdateCheckSoon(){clearTimeout(updateCheckKickoff);updateCheckKickoff=setTimeout(()=>checkForUpdates({quiet:true}),5000)}\n  function startPeriodicUpdateChecks(){clearInterval(updateCheckTimer);updateCheckTimer=setInterval(()=>checkForUpdates({quiet:true}),UPDATE_CHECK_INTERVAL_MS)}\n''',
)

replace_once(
    "internal/webui/assets/app.js",
    '''  function switchTab(tabName){\n    state.activeTab=tabName;\n''',
    '''  function switchTab(tabName){\n    state.activeTab=tabName;\n    if(tabName==='settings')dismissSettingsUpdateBadge();\n''',
)

replace_once(
    "internal/webui/assets/app.js",
    '''      const btn=document.createElement('button');btn.className='nav';btn.dataset.tab=id;btn.textContent=navLabel(id);btn.title=navLabel(id);wireNavButton(btn);nav.appendChild(btn);\n''',
    '''      const btn=document.createElement('button');btn.className='nav';btn.dataset.tab=id;btn.textContent=navLabel(id);btn.title=navLabel(id);if(id==='settings')decorateSettingsUpdateBadge(btn);wireNavButton(btn);nav.appendChild(btn);\n''',
)

replace_once(
    "internal/webui/assets/app.js",
    '''      state.core=data;state.updating=false;if(data.locale)applyLocaleInfo(data.locale);if($('#quit-jacob'))$('#quit-jacob').hidden=!data.localClient;if($('#install-update'))$('#install-update').hidden=!data.localClient;$('#core-os').textContent=`${data.os}/${data.arch}`;$('#core-api').textContent=`v${data.apiVersion} (${data.product||'JACoB'} ${data.prototype})`;if($('#update-current'))$('#update-current').textContent=data.version||data.prototype||'—';\n''',
    '''      state.core=data;state.updating=false;if(data.locale)applyLocaleInfo(data.locale);if($('#quit-jacob'))$('#quit-jacob').hidden=!data.localClient;if($('#install-update'))$('#install-update').hidden=!data.localClient;$('#core-os').textContent=`${data.os}/${data.arch}`;$('#core-api').textContent=`v${data.apiVersion} (${data.product||'JACoB'} ${data.prototype})`;if($('#update-current'))$('#update-current').textContent=data.version||data.prototype||'—';queueUpdateCheckSoon();\n''',
)

old_check = '''  async function checkForUpdates(){\n    const out=$('#update-result'),btn=$('#check-update'),install=$('#install-update');btn.disabled=true;install.disabled=true;out.textContent=tr('Querying the release channel…');\n    try{const info=await request('update.check');state.updateInfo=info;$('#update-latest').textContent=info.latestVersion||'—';if(info.available){out.textContent=`${info.releaseName||('JACoB '+info.latestVersion)} is available.${info.installable?' Ready for installation.':' '+(info.installNote||'Manual installation required.')}`;install.disabled=!(info.installable&&state.core?.localClient)}else{out.textContent=tr('Current release confirmed. No newer published build was found.')}}\n    catch(e){out.textContent=e?.message||pretty(e)}finally{btn.disabled=false}\n  }\n'''
new_check = '''  async function checkForUpdates({quiet=false}={}){\n    const out=$('#update-result'),btn=$('#check-update'),install=$('#install-update');if(!state.socket||state.socket.readyState!==WebSocket.OPEN)return;if(quiet&&!state.core?.localClient)return;if(!quiet){btn.disabled=true;install.disabled=true;out.textContent=tr('Querying the release channel…')}\n    try{const info=await request('update.check');state.updateInfo=info;$('#update-latest').textContent=info.latestVersion||'—';install.disabled=!(info.available&&info.installable&&state.core?.localClient);if(info.available&&state.activeTab==='settings'&&info.latestVersion){try{localStorage.setItem(UPDATE_BADGE_KEY,String(info.latestVersion))}catch{}}renderNavigation();if(info.available){out.textContent=`${info.releaseName||('JACoB '+info.latestVersion)} is available.${info.installable?' Ready for installation.':' '+(info.installNote||'Manual installation required.')}`}else{out.textContent=tr('Current release confirmed. No newer published build was found.')}}\n    catch(e){if(!quiet)out.textContent=e?.message||pretty(e)}finally{if(!quiet)btn.disabled=false}\n  }\n'''
replace_once("internal/webui/assets/app.js", old_check, new_check)
replace_once(
    "internal/webui/assets/app.js",
    '''  $('#check-update').onclick=checkForUpdates;$('#install-update').onclick=installUpdate;\n''',
    '''  $('#check-update').onclick=()=>checkForUpdates({quiet:false});$('#install-update').onclick=installUpdate;\n''',
)
replace_once(
    "internal/webui/assets/app.js",
    '''  ensurePairUI();\n  connect();\n})();\n''',
    '''  ensurePairUI();\n  startPeriodicUpdateChecks();\n  connect();\n})();\n''',
)

styles = read("internal/webui/assets/styles.css")
if ".settings-update-badge" not in styles:
    styles = styles.rstrip() + r'''

.settings-update-badge{display:inline-flex;align-items:center;justify-content:center;margin-left:7px;min-width:25px;height:16px;padding:0 4px;border:1px solid var(--line-hot);background:#1a0f07;color:#ffd0a2;font:9px/1 ui-monospace,SFMono-Regular,Consolas,monospace;letter-spacing:.05em}
''' + "\n"
    write("internal/webui/assets/styles.css", styles)
else:
    raise SystemExit("styles.css already contains settings-update-badge")

replace_once(
    "internal/webui/assets/index.html",
    '<footer id="footer-slot"><span>JACoB Alpha 0.2.8</span>',
    '<footer id="footer-slot"><span>JACoB Alpha 0.2.10 · SDK 8</span>',
)

# ---------------------------------------------------------------------------
# SDK reference copies shipped in the web UI and installer.
# ---------------------------------------------------------------------------
for doc_path in [
    "docs/JACOB_CUSTOM_TAB_REFERENCE.md",
    "internal/webui/assets/docs/JACOB_CUSTOM_TAB_REFERENCE.md",
    "cmd/installer/payload/Documentation/JACoB-Custom-Tab-Developer-Reference.md",
]:
    doc = read(doc_path)
    doc = doc.replace("Version: Alpha 0.2.8", "Version: Alpha 0.2.10")
    doc = doc.replace("SDK version: 7", "SDK version: 8")
    doc = doc.replace("SDK 7 also injects", "SDK 8 also injects")
    doc = doc.replace("Elite.api.version === 7", "Elite.api.version === 8")
    if "### 3.1 Physical keyboard identifiers (SDK 8)" not in doc:
        anchor = "The current SDK version is:\n\n```js\nElite.api.version === 8\n```\n"
        addition = r'''

### 3.1 Physical keyboard identifiers (SDK 8)

Keyboard-control calls accept the existing logical names and canonical physical scan-code tokens. Physical tokens are layout-independent and use PC/AT set-1 scan codes:

```text
SC:29
SC:56
SC:E0:38
```

`Elite.input.tap()` and `Elite.input.hold()` may use these tokens for the main key or modifiers. Existing names such as `A`, `ENTER`, `RIGHTALT`, and `LEFTSHIFT` remain supported.

Windows recorder events retain the legacy `key` and `modifiers` fields and additionally report `physical`, `scanCode`, `virtualKey`, `extended`, and `localizedName`. New recorder-driven tools should store `physical` when present and use `localizedName` only for display. This preserves AZERTY/QWERTZ, ISO-102, JIS/IME, ABNT, Nordic, Cyrillic-layout, AltGr/right-Alt, and left/right modifier positions without assuming a US keyboard.

`Elite.input.text()` is character-oriented rather than physical-key-oriented. JACoB uses the active host layout for text entry and preserves Unicode where the platform input path supports it.
'''
        if anchor not in doc:
            raise SystemExit(f"{doc_path}: SDK version anchor not found")
        doc = doc.replace(anchor, anchor + addition, 1)
    write(doc_path, doc)

for html_path in [
    "internal/webui/assets/docs/developer-reference.html",
    "cmd/installer/payload/Documentation/JACoB-Custom-Tab-Developer-Reference.html",
]:
    html = read(html_path)
    html = html.replace("Alpha 0.2.8", "Alpha 0.2.10")
    html = html.replace("SDK version: 7", "SDK version: 8")
    html = html.replace("SDK 7", "SDK 8")
    html = html.replace("Elite.api.version === 7", "Elite.api.version === 8")
    write(html_path, html)

# ---------------------------------------------------------------------------
# README release metadata was accidentally rolled back during the 0.29 work.
# ---------------------------------------------------------------------------
readme = read("README.md")
readme = readme.replace('> Current field build: **0.2.4 Alpha**', '> Current field build: **0.2.10 Alpha · SDK 8**')
readme = readme.replace('SDK / protocol version **2** adds:', 'SDK / protocol version **8** includes:')
if 'physical scan-code tokens' not in readme:
    marker = 'SDK / protocol version **8** includes:\n\n'
    if marker in readme:
        readme = readme.replace(marker, marker + '- physical scan-code tokens (`SC:xx` / `SC:E0:xx`) for layout-independent keyboard control and recorder replay\n- layout-aware Unicode text entry, including AltGr/international layouts where the host supports it\n- richer recorder events with physical scan code, virtual key, extended-key flag, and localized key label\n', 1)
write("README.md", readme)

# ---------------------------------------------------------------------------
# Permanent web-friendly release builder. Future releases can be built and
# placed into a draft Release without any local Git tooling.
# ---------------------------------------------------------------------------
write(".github/workflows/build-windows-release.yml", r'''name: Build JACoB Release

on:
  workflow_dispatch:
    inputs:
      version:
        description: Version from internal/buildinfo/version.go (example 0.2.11-alpha)
        required: true
        default: 0.2.10-alpha
      create_draft_release:
        description: Create/update a draft GitHub release with the built assets
        required: true
        type: boolean
        default: true

permissions:
  contents: write

jobs:
  build:
    runs-on: windows-latest
    steps:
      - uses: actions/checkout@v4

      - uses: actions/setup-go@v5
        with:
          go-version-file: go.mod

      - name: Verify version
        shell: powershell
        run: |
          $want = "${{ inputs.version }}"
          $src = Get-Content internal/buildinfo/version.go -Raw
          if ($src -notmatch ('Version\s*=\s*"' + [regex]::Escape($want) + '"')) {
            throw "buildinfo.Version does not match requested version $want"
          }

      - name: Test Windows
        run: go test ./...

      - name: Compile Linux core
        shell: powershell
        run: |
          New-Item -ItemType Directory -Force dist | Out-Null
          $env:CGO_ENABLED="0"
          $env:GOOS="linux"
          $env:GOARCH="amd64"
          go build -trimpath -o dist/JACoB-linux-amd64 ./cmd/jacob
          $env:GOOS="windows"
          $env:GOARCH="amd64"

      - name: Build Windows applications and installer
        shell: powershell
        run: |
          $env:CGO_ENABLED="0"
          $env:GOOS="windows"
          $env:GOARCH="amd64"
          $v = "${{ inputs.version }}"
          go build -trimpath -ldflags="-H=windowsgui" -o dist/JACoB.exe ./cmd/jacob
          go build -trimpath -tags norecorder -ldflags="-H=windowsgui" -o dist/JACoB-no-recorder.exe ./cmd/jacob
          Copy-Item dist/JACoB.exe cmd/installer/payload/JACoB.exe
          Copy-Item dist/JACoB-no-recorder.exe cmd/installer/payload/JACoB-no-recorder.exe
          try {
            go build -trimpath -ldflags="-H=windowsgui" -o "dist/JACoB-Setup-$v.exe" ./cmd/installer
          } finally {
            Remove-Item cmd/installer/payload/JACoB.exe,cmd/installer/payload/JACoB-no-recorder.exe -Force -ErrorAction SilentlyContinue
          }
          $hash=(Get-FileHash "dist/JACoB-Setup-$v.exe" -Algorithm SHA256).Hash.ToLower()
          "$hash  JACoB-Setup-$v.exe" | Out-File "dist/JACoB-Setup-$v.exe.sha256" -Encoding ascii

      - uses: actions/upload-artifact@v4
        with:
          name: JACoB-${{ inputs.version }}
          path: |
            dist/JACoB.exe
            dist/JACoB-no-recorder.exe
            dist/JACoB-linux-amd64
            dist/JACoB-Setup-${{ inputs.version }}.exe
            dist/JACoB-Setup-${{ inputs.version }}.exe.sha256

      - name: Create or update draft release
        if: ${{ inputs.create_draft_release }}
        shell: powershell
        env:
          GH_TOKEN: ${{ github.token }}
        run: |
          $v="${{ inputs.version }}"
          $tag="v$v"
          $assets=@("dist/JACoB.exe","dist/JACoB-no-recorder.exe","dist/JACoB-linux-amd64","dist/JACoB-Setup-$v.exe","dist/JACoB-Setup-$v.exe.sha256")
          gh release view $tag *> $null
          if ($LASTEXITCODE -eq 0) {
            gh release upload $tag @assets --clobber
            gh release edit $tag --draft --prerelease --title "JACoB $v"
          } else {
            gh release create $tag @assets --target "${{ github.sha }}" --draft --prerelease --title "JACoB $v" --notes "Automated JACoB build. Test the installer before publishing this draft release."
          }
''')

write("RELEASE-NOTES-0.2.10-alpha.md", r'''# JACoB 0.2.10 Alpha — Recovery + SDK 8

This release repairs the broken 0.29 update path and advances the custom-tab SDK to version 8.

## Recovery

- Corrects the embedded/runtime version so the updater no longer installs a build that still identifies itself as 0.2.8.
- Windows installer keeps `JACoB.exe.previous`, verifies the restarted core reports the expected version, and restores the previous executable if verification fails.
- LAN pairing tokens persist in the JACoB data directory instead of rotating every restart.
- Release builds are produced by GitHub Actions from the committed source and exact installer payload.
- Version comparison now handles additional numeric components safely.

## International keyboard support

- Physical keyboard controls can use canonical scan-code tokens such as `SC:29`, `SC:56`, and `SC:E0:38`.
- Existing key names remain supported for backward compatibility.
- Windows recorder events retain physical scan code, extended-key state, virtual key, localized key label, and the legacy `key`/`modifiers` fields.
- Unknown ISO/JIS/OEM keys are no longer discarded by the Windows recorder.
- Right Alt / AltGr, right Control, right Shift, ISO-102, JIS conversion keys, and equivalent physical positions can be learned and replayed without assuming QWERTY.
- Linux/Steam Deck accepts the same physical-key tokens and translates common extended/international set-1 scan codes to evdev.
- Text entry is separated from physical control replay. Windows uses the Elite window's active keyboard layout and Unicode fallback; Linux uses layout-aware `xdotool type` when available and retains the legacy mapper as fallback.
- Physical scan-code expressions pass through the same global-shortcut safety guard as legacy key names.

## Updates UI

- JACoB checks the published release channel shortly after connecting and every 30 minutes while the UI remains open.
- A `NEW` badge appears on Settings when a newer release is found.
- Opening Settings dismisses that badge for that specific published version even if the update is not installed.
- A later, newer release re-arms the badge.
- Background checks never install an update automatically.
''')

# ---------------------------------------------------------------------------
# SDK 9: host-brokered cross-tab actions + visibility for custom tabs.
# This runs after the SDK 8 recovery so every target below is the finished
# 0.2.10 recovery state, not the broken 0.2.9 tree.
# ---------------------------------------------------------------------------
replace_once(
    "internal/buildinfo/version.go",
    'Display    = "Alpha 0.2.10 SDK8 Recovery"',
    'Display    = "Alpha 0.2.10 SDK9 Recovery"',
)
replace_once("internal/core/server.go", '"apiVersion": 8', '"apiVersion": 9')

# Navigation storage already persists a hidden-default list. Keep the JSON key
# for backward compatibility, but permit every valid navigation id except the
# Tab Manager recovery page. Old schema-2 stores load unchanged.
replace_once(
    "internal/customtabs/store.go",
    'var defaultNavIDs = []string{"dashboard", "tabmanager", "tutorial", "settings"}\nvar hideableDefaultNavIDs = map[string]bool{"dashboard": true, "tutorial": true, "settings": true}\n',
    'var defaultNavIDs = []string{"dashboard", "tabmanager", "tutorial", "settings"}\n',
)
replace_once(
    "internal/customtabs/store.go",
    '''\t\tif hideableDefaultNavIDs[id] && !hiddenSeen[id] {\n\t\t\thidden = append(hidden, id)\n\t\t\thiddenSeen[id] = true\n\t\t}\n''',
    '''\t\tif valid[id] && id != "tabmanager" && !hiddenSeen[id] {\n\t\t\thidden = append(hidden, id)\n\t\t\thiddenSeen[id] = true\n\t\t}\n''',
)
customtabs_test = read("internal/customtabs/store_test.go")
if "func TestLayoutCanHideCustomTabs" not in customtabs_test:
    customtabs_test = customtabs_test.rstrip() + r'''

func TestLayoutCanHideCustomTabs(t *testing.T) {
    dir := t.TempDir()
    s, err := New(dir)
    if err != nil {
        t.Fatal(err)
    }
    tab, err := s.Save("", "Hidden Tool", "<p>x</p>")
    if err != nil {
        t.Fatal(err)
    }
    navID := "custom-" + tab.ID
    layout, err := s.SaveLayout([]string{"dashboard", navID, "tabmanager", "tutorial", "settings"}, []string{navID, "settings", "tabmanager"})
    if err != nil {
        t.Fatal(err)
    }
    if len(layout.HiddenDefaults) != 2 || layout.HiddenDefaults[0] != navID || layout.HiddenDefaults[1] != "settings" {
        t.Fatalf("unexpected hidden navigation: %#v", layout.HiddenDefaults)
    }
    s2, err := New(dir)
    if err != nil {
        t.Fatal(err)
    }
    got := s2.Layout()
    found := false
    for _, id := range got.HiddenDefaults {
        if id == navID {
            found = true
        }
        if id == "tabmanager" {
            t.Fatalf("Tab Manager must never be hideable: %#v", got.HiddenDefaults)
        }
    }
    if !found {
        t.Fatalf("hidden custom tab did not persist: %#v", got.HiddenDefaults)
    }
    if err := s2.Delete(tab.ID); err != nil {
        t.Fatal(err)
    }
    for _, id := range s2.Layout().HiddenDefaults {
        if id == navID {
            t.Fatalf("deleted custom tab remained hidden in layout: %#v", s2.Layout())
        }
    }
}
''' + "\n"
    write("internal/customtabs/store_test.go", customtabs_test)
else:
    raise SystemExit("customtabs store tests already contain SDK 9 visibility test")

# Host/browser SDK action broker. Cross-tab calls stay inside the privileged host
# message bridge; custom iframes never receive direct references to one another.
replace_once(
    "internal/webui/assets/app.js",
    "bindings:[],savedTabs:[],tabHTMLCache:new Map(),tabLoadPromises:new Map(),navLayout:{order:[],hiddenDefaults:[]},editingTabId:'',activeTab:'dashboard',themeHTML:'',locale:{language:'en',supported:[]},quitting:false,updating:false,updateInfo:null",
    "bindings:[],savedTabs:[],tabHTMLCache:new Map(),tabLoadPromises:new Map(),navLayout:{order:[],hiddenDefaults:[]},editingTabId:'',activeTab:'dashboard',themeHTML:'',locale:{language:'en',supported:[]},quitting:false,updating:false,updateInfo:null,actionRegistry:new Map(),actionInvocations:new Map(),actionSeq:0,actionDiscoveryDone:false,actionDiscoveryPromise:null",
)
replace_once(
    "internal/webui/assets/app.js",
    "let seq=0;const pending=new Map(),eventSubs=new Map(),journalSubs=[];",
    "let seq=0;const pending=new Map(),eventSubs=new Map(),journalSubs=[],actionHandlers=new Map();",
)
replace_once("internal/webui/assets/app.js", "api:Object.freeze({version:8})", "api:Object.freeze({version:9})")
replace_once(
    "internal/webui/assets/app.js",
    """ store:Object.freeze({get:async(key,fallback=null)=>{const r=await request('tabstate.get',{key});return r?.found?r.value:fallback},set:(key,value)=>request('tabstate.set',{key,value}),delete:key=>request('tabstate.delete',{key}),clear:()=>request('tabstate.clear')}),
 locale:Object.freeze({get language(){return localeState.language},get supported(){return localeState.supported.map(x=>({...x}))},get:()=>request('locale.get'),t:(dictionary,key,vars={})=>localeTranslate(dictionary,key,vars),subscribe:cb=>{const off=onEvent('locale.changed',cb);queueMicrotask(()=>cb({...localeState,supported:localeState.supported.map(x=>({...x}))}));return off}}),
""",
    """ store:Object.freeze({get:async(key,fallback=null)=>{const r=await request('tabstate.get',{key});return r?.found?r.value:fallback},set:(key,value)=>request('tabstate.set',{key,value}),delete:key=>request('tabstate.delete',{key}),clear:()=>request('tabstate.clear')}),
 actions:Object.freeze({
  register:async(name,handler,options={})=>{name=String(name||'').trim();if(!name)throw Object.assign(new Error('action name is required'),{code:'BAD_PARAMS'});if(typeof handler!=='function')throw Object.assign(new Error('action handler must be a function'),{code:'BAD_PARAMS'});actionHandlers.set(name,handler);try{await request('actions.register',{name,label:String(options.label||''),description:String(options.description||'')})}catch(error){if(actionHandlers.get(name)===handler)actionHandlers.delete(name);throw error}return async()=>{if(actionHandlers.get(name)===handler)actionHandlers.delete(name);try{return await request('actions.unregister',{name})}catch(error){if(error?.code!=='ACTION_NOT_FOUND')throw error;return{unregistered:true,name}}}},
  unregister:async name=>{name=String(name||'').trim();actionHandlers.delete(name);return request('actions.unregister',{name})},
  list:(options={})=>request('actions.list',{discover:options.discover!==false}),
  invoke:(name,payload=null,options={})=>request('actions.invoke',{name:String(name||'').trim(),payload,timeoutMs:Number(options.timeoutMs)||15000,discover:options.discover!==false})
 }),
 tabs:Object.freeze({list:()=>request('tabs.sdk.list'),activate:target=>request('tabs.activate',{target:String(target||'')})}),
 locale:Object.freeze({get language(){return localeState.language},get supported(){return localeState.supported.map(x=>({...x}))},get:()=>request('locale.get'),t:(dictionary,key,vars={})=>localeTranslate(dictionary,key,vars),subscribe:cb=>{const off=onEvent('locale.changed',cb);queueMicrotask(()=>cb({...localeState,supported:localeState.supported.map(x=>({...x}))}));return off}}),
""",
)
replace_once(
    "internal/webui/assets/app.js",
    """addEventListener('message',ev=>{const m=ev.data;if(!m||m.channel!=='jacob-host')return;if(m.kind==='response'){const p=pending.get(m.id);if(!p)return;pending.delete(m.id);m.ok?p.resolve(m.result):p.reject(Object.assign(new Error(m.error?.message||'JACoB error'),{code:m.error?.code}));return}if(m.kind==='event'){if(m.event==='locale.changed'&&m.data)localeState={language:m.data.language||localeState.language,supported:Array.isArray(m.data.supported)&&m.data.supported.length?m.data.supported:localeState.supported};if(m.event==='core.hello'&&m.data?.locale)localeState={language:m.data.locale.language||localeState.language,supported:Array.isArray(m.data.locale.supported)&&m.data.locale.supported.length?m.data.locale.supported:localeState.supported};const set=eventSubs.get(m.event);if(set)for(const cb of set)try{cb(m.data)}catch(e){console.error(e)};const all=eventSubs.get('*');if(all)for(const cb of all)try{cb(m.event,m.data)}catch(e){console.error(e)};if(m.event==='journal')for(const sub of [...journalSubs])if(sub.eventName==='*'||sub.eventName===m.data?.event)try{sub.cb(m.data)}catch(e){console.error(e)}}});
""",
    """addEventListener('message',ev=>{const m=ev.data;if(!m||m.channel!=='jacob-host')return;if(m.kind==='response'){const p=pending.get(m.id);if(!p)return;pending.delete(m.id);m.ok?p.resolve(m.result):p.reject(Object.assign(new Error(m.error?.message||'JACoB error'),{code:m.error?.code}));return}if(m.kind==='action-invoke'){const handler=actionHandlers.get(String(m.action||''));if(!handler){parent.postMessage({channel:'jacob-tab',kind:'action-result',id:m.id,ok:false,error:{code:'ACTION_NOT_FOUND',message:'registered action is unavailable'}},'*');return}Promise.resolve().then(()=>handler(m.payload,{action:m.action,caller:m.caller||null})).then(result=>parent.postMessage({channel:'jacob-tab',kind:'action-result',id:m.id,ok:true,result},'*')).catch(error=>parent.postMessage({channel:'jacob-tab',kind:'action-result',id:m.id,ok:false,error:{code:String(error?.code||'ACTION_FAILED'),message:String(error?.message||error||'action failed')}},'*'));return}if(m.kind==='event'){if(m.event==='locale.changed'&&m.data)localeState={language:m.data.language||localeState.language,supported:Array.isArray(m.data.supported)&&m.data.supported.length?m.data.supported:localeState.supported};if(m.event==='core.hello'&&m.data?.locale)localeState={language:m.data.locale.language||localeState.language,supported:Array.isArray(m.data.locale.supported)&&m.data.locale.supported.length?m.data.locale.supported:localeState.supported};const set=eventSubs.get(m.event);if(set)for(const cb of set)try{cb(m.data)}catch(e){console.error(e)};const all=eventSubs.get('*');if(all)for(const cb of all)try{cb(m.event,m.data)}catch(e){console.error(e)};if(m.event==='journal')for(const sub of [...journalSubs])if(sub.eventName==='*'||sub.eventName===m.data?.event)try{sub.cb(m.data)}catch(e){console.error(e)}}});
""",
)
replace_once(
    "internal/webui/assets/app.js",
    "const defaultFooter='<span>JACoB Alpha 0.2.10 · SDK 8</span><a href=\"/docs/index.html\" target=\"_blank\" rel=\"noopener\">Documentation</a>';",
    "const defaultFooter='<span>JACoB Alpha 0.2.10 · SDK 9</span><a href=\"/docs/index.html\" target=\"_blank\" rel=\"noopener\">Documentation</a>';",
)
replace_once(
    "internal/webui/assets/app.js",
    "    vision:{label:'Use derived game vision',detail:'receive feature points and motion measurements computed from Elite-only frames; raw pixels are never exposed'}\n",
    "    vision:{label:'Use derived game vision',detail:'receive feature points and motion measurements computed from Elite-only frames; raw pixels are never exposed'},\n    intertab:{label:'Control other JACoB tabs',detail:'invoke actions that other saved tabs explicitly publish through the JACoB action broker'}\n",
)
replace_once(
    "internal/webui/assets/app.js",
    "    if(method.startsWith('input.')||method.startsWith('binding.'))return'control';\n",
    "    if(method==='actions.invoke')return'intertab';\n    if(method.startsWith('input.')||method.startsWith('binding.'))return'control';\n",
)

broker_anchor = "  function seedFrame(frame){if(state.core)sendToFrame(frame,{channel:'jacob-host',kind:'event',event:'core.hello',data:sanitizedCoreForTab(state.core)});if(state.locale)sendToFrame(frame,{channel:'jacob-host',kind:'event',event:'locale.changed',data:state.locale});if(state.snapshot)sendToFrame(frame,{channel:'jacob-host',kind:'event',event:'state',data:state.snapshot})}\n\n"
broker_addition = r'''  const frameReadyWaiters=new WeakMap();
  function publicActionRecord(record){return{name:record.name,label:record.label||record.name,description:record.description||'',tabId:record.tabId,tabName:record.tabName||record.tabId}}
  function actionError(code,message){return{code,message}}
  function clearActionsForFrame(frame,reason='target tab reloaded'){
    for(const [name,record] of [...state.actionRegistry])if(record.frame===frame)state.actionRegistry.delete(name);
    for(const [id,pending] of [...state.actionInvocations])if(pending.targetFrame===frame){clearTimeout(pending.timer);state.actionInvocations.delete(id);pending.reject(actionError('ACTION_TARGET_RELOADED',reason))}
  }
  function markFrameReady(frame){frame.dataset.sdkReady='1';const waiters=frameReadyWaiters.get(frame)||[];frameReadyWaiters.delete(frame);for(const done of waiters)done(true)}
  function waitForFrameReady(frame,timeoutMs=5000){if(frame?.dataset?.sdkReady==='1')return Promise.resolve(true);return new Promise(resolve=>{const list=frameReadyWaiters.get(frame)||[];let settled=false;const done=value=>{if(settled)return;settled=true;clearTimeout(timer);resolve(value)};list.push(done);frameReadyWaiters.set(frame,list);const timer=setTimeout(()=>done(false),timeoutMs)})}
  function registerTabAction(frame,params={}){
    const tabId=frame?.dataset?.savedTabId||'';if(!tabId)throw actionError('ACTION_PREVIEW','preview tabs cannot publish cross-tab actions');
    const name=String(params.name||'').trim();if(!/^[A-Za-z0-9][A-Za-z0-9._:-]{0,95}$/.test(name))throw actionError('BAD_PARAMS','action name must be 1-96 letters, numbers, dots, colons, underscores, or hyphens');
    const existing=state.actionRegistry.get(name);if(existing&&existing.frame!==frame)throw actionError('ACTION_CONFLICT',`action ${name} is already published by ${existing.tabName||existing.tabId}`);
    const meta=savedTabMeta(tabId);const record={name,label:String(params.label||name).slice(0,120),description:String(params.description||'').slice(0,300),tabId,tabName:meta?.name||tabId,frame};state.actionRegistry.set(name,record);return publicActionRecord(record)
  }
  function unregisterTabAction(frame,name){name=String(name||'').trim();const existing=state.actionRegistry.get(name);if(!existing)throw actionError('ACTION_NOT_FOUND','registered action was not found');if(existing.frame!==frame)throw actionError('ACTION_NOT_OWNER','a tab may unregister only its own action');state.actionRegistry.delete(name);return{unregistered:true,name}}
  async function discoverTabActions(){
    if(state.actionDiscoveryDone)return;
    if(state.actionDiscoveryPromise)return state.actionDiscoveryPromise;
    state.actionDiscoveryPromise=(async()=>{const loads=state.savedTabs.map(async tab=>{try{await ensureSavedTabLoaded(tab.id);const frame=savedTabFrame(tab.id);if(frame)await waitForFrameReady(frame,5000)}catch{}});await Promise.all(loads);await new Promise(resolve=>setTimeout(resolve,100));state.actionDiscoveryDone=true})().finally(()=>{state.actionDiscoveryPromise=null});
    return state.actionDiscoveryPromise
  }
  function listRegisteredActions(){return[...state.actionRegistry.values()].map(publicActionRecord).sort((a,b)=>a.name.localeCompare(b.name))}
  function callerInfo(frame){const id=frame?.dataset?.savedTabId||'';const meta=id?savedTabMeta(id):null;return{id,tabId:id,tabName:meta?.name||(id?'Saved tab':'Tab preview')}}
  async function invokeRegisteredAction(callerFrame,params={}){
    const name=String(params.name||'').trim();if(!name)throw actionError('BAD_PARAMS','action name is required');
    let record=state.actionRegistry.get(name);if(!record&&params.discover!==false){await discoverTabActions();record=state.actionRegistry.get(name)}
    if(!record)throw actionError('ACTION_NOT_FOUND',`no loaded saved tab publishes ${name}`);
    const timeoutMs=Math.max(500,Math.min(60000,Number(params.timeoutMs)||15000));const id=`action-${++state.actionSeq}-${Date.now()}`;
    return new Promise((resolve,reject)=>{const timer=setTimeout(()=>{state.actionInvocations.delete(id);reject(actionError('ACTION_TIMEOUT',`${name} did not finish within ${timeoutMs} ms`))},timeoutMs);state.actionInvocations.set(id,{resolve,reject,timer,targetFrame:record.frame});sendToFrame(record.frame,{channel:'jacob-host',kind:'action-invoke',id,action:name,payload:params.payload,caller:callerInfo(callerFrame)})})
  }
  function resolveSDKTabTarget(target){const raw=String(target||'').trim();if(!raw)return'';if(navigationOrder().includes(raw))return raw;const byId=state.savedTabs.find(tab=>tab.id===raw);if(byId)return navIDForTab(byId);const lower=raw.toLowerCase(),byName=state.savedTabs.find(tab=>String(tab.name||'').toLowerCase()===lower);if(byName)return navIDForTab(byName);for(const id of Object.keys(defaultNav))if(id.toLowerCase()===lower||navLabel(id).toLowerCase()===lower)return id;return''}
  function sdkTabList(){const hidden=hiddenNavigation();return navigationOrder().map(id=>{const custom=id.startsWith('custom-'),tab=custom?tabForNavID(id):null;return{id:custom?tab?.id||id.slice(7):id,navId:id,name:navLabel(id),type:custom?'custom':'default',hidden:hidden.has(id),active:state.activeTab===id,loaded:custom?savedTabFrame(tab?.id||'')?.dataset?.sdkReady==='1':true}})}
  async function activateSDKTab(target){const id=resolveSDKTabTarget(target);if(!id)throw actionError('TAB_NOT_FOUND','requested JACoB tab was not found');if(id.startsWith('custom-'))await ensureSavedTabLoaded(id.slice(7));switchTab(id);return{id,name:navLabel(id),hidden:hiddenNavigation().has(id)}}

'''
replace_once("internal/webui/assets/app.js", broker_anchor, broker_anchor + broker_addition)

app_text = read("internal/webui/assets/app.js")
if "  const hideableDefaults=new Set(['dashboard','tutorial','settings']);\n" not in app_text:
    raise SystemExit("app.js: hideableDefaults anchor not found")
app_text = app_text.replace("  const hideableDefaults=new Set(['dashboard','tutorial','settings']);\n", "", 1)
if "  function hiddenDefaults(){return new Set(state.navLayout?.hiddenDefaults||[])}\n" not in app_text:
    raise SystemExit("app.js: hiddenDefaults helper not found")
app_text = app_text.replace("  function hiddenDefaults(){return new Set(state.navLayout?.hiddenDefaults||[])}\n", "  function hiddenNavigation(){return new Set(state.navLayout?.hiddenDefaults||[])}\n", 1)
app_text = app_text.replace("hiddenDefaults()", "hiddenNavigation()")
write("internal/webui/assets/app.js", app_text)
replace_once(
    "internal/webui/assets/app.js",
    "  function toggleDefaultNavigation(id){if(!hideableDefaults.has(id))return;const hidden=hiddenNavigation();hidden.has(id)?hidden.delete(id):hidden.add(id);persistNavigation(navigationOrder(),[...hidden])}\n",
    "  function toggleNavigationVisibility(id){if(id==='tabmanager'||!navigationOrder().includes(id))return;const hidden=hiddenNavigation();hidden.has(id)?hidden.delete(id):hidden.add(id);persistNavigation(navigationOrder(),[...hidden])}\n",
)
replace_once(
    "internal/webui/assets/app.js",
    """    order.forEach((id,index)=>{const row=document.createElement('div');row.className='saved-tab-row';const meta=document.createElement('div');meta.className='saved-tab-meta';const custom=id.startsWith('custom-');meta.innerHTML=`<strong${custom?' data-jacob-no-i18n="true"':''}>${escapeHTML(navLabel(id))}</strong><span class="muted">${custom?tr('Custom tab'):id==='tabmanager'?tr('Default · required'):(hidden.has(id)?tr('Default · hidden'):tr('Default'))}</span>`;const actions=document.createElement('div');actions.className='row';const up=document.createElement('button');up.className='secondary compact-button';up.textContent='↑';up.title=tr('Move up');up.disabled=index===0;up.onclick=()=>moveNavigation(id,-1);const down=document.createElement('button');down.className='secondary compact-button';down.textContent='↓';down.title=tr('Move down');down.disabled=index===order.length-1;down.onclick=()=>moveNavigation(id,1);actions.append(up,down);if(!custom){const visibility=document.createElement('button');visibility.className='secondary';if(id==='tabmanager'){visibility.textContent=tr('Required');visibility.disabled=true}else{visibility.textContent=hidden.has(id)?tr('Show'):tr('Hide');visibility.onclick=()=>toggleDefaultNavigation(id)}actions.appendChild(visibility)}row.append(meta,actions);box.appendChild(row)});localizeDOM(box);
""",
    """    order.forEach((id,index)=>{const row=document.createElement('div');row.className='saved-tab-row';const meta=document.createElement('div');meta.className='saved-tab-meta';const custom=id.startsWith('custom-'),isHidden=hidden.has(id);meta.innerHTML=`<strong${custom?' data-jacob-no-i18n="true"':''}>${escapeHTML(navLabel(id))}</strong><span class="muted">${id==='tabmanager'?tr('Default · required'):(custom?(isHidden?tr('Custom tab · hidden'):tr('Custom tab')):(isHidden?tr('Default · hidden'):tr('Default')))}</span>`;const actions=document.createElement('div');actions.className='row';const up=document.createElement('button');up.className='secondary compact-button';up.textContent='↑';up.title=tr('Move up');up.disabled=index===0;up.onclick=()=>moveNavigation(id,-1);const down=document.createElement('button');down.className='secondary compact-button';down.textContent='↓';down.title=tr('Move down');down.disabled=index===order.length-1;down.onclick=()=>moveNavigation(id,1);actions.append(up,down);const visibility=document.createElement('button');visibility.className='secondary';if(id==='tabmanager'){visibility.textContent=tr('Required');visibility.disabled=true}else{visibility.textContent=isHidden?tr('Show'):tr('Hide');visibility.onclick=()=>toggleNavigationVisibility(id)}actions.appendChild(visibility);row.append(meta,actions);box.appendChild(row)});localizeDOM(box);
""",
)
replace_once(
    "internal/webui/assets/app.js",
    """  async function loadSavedTabs(){
    const result=await request('tabs.list');state.savedTabs=result.tabs||[];state.navLayout=result.layout||{order:[],hiddenDefaults:[]};
    const live=new Set(state.savedTabs.map(t=>t.id));
""",
    """  async function loadSavedTabs(){
    const before=state.savedTabs.map(t=>`${t.id}:${t.updatedAt||''}`).join('|');const result=await request('tabs.list');state.savedTabs=result.tabs||[];state.navLayout=result.layout||{order:[],hiddenDefaults:[]};const after=state.savedTabs.map(t=>`${t.id}:${t.updatedAt||''}`).join('|');if(before!==after)state.actionDiscoveryDone=false;
    const live=new Set(state.savedTabs.map(t=>t.id));
""",
)
replace_once(
    "internal/webui/assets/app.js",
    "  function applySavedTabHTML(tab){const frame=savedTabFrame(tab.id);if(!frame)return;if(frame.dataset.updatedAt===String(tab.updatedAt||''))return;frame.dataset.updatedAt=tab.updatedAt||'';frame.srcdoc=composeTabHTML(tab.html||'')}\n",
    "  function applySavedTabHTML(tab){const frame=savedTabFrame(tab.id);if(!frame)return;if(frame.dataset.updatedAt===String(tab.updatedAt||''))return;clearActionsForFrame(frame);frame.dataset.sdkReady='';frame.dataset.updatedAt=tab.updatedAt||'';frame.srcdoc=composeTabHTML(tab.html||'')}\n",
)
replace_once(
    "internal/webui/assets/app.js",
    "    for(const existing of [...pages.querySelectorAll('.custom-user-page')])if(!wanted.has(existing.id))existing.remove();\n",
    "    for(const existing of [...pages.querySelectorAll('.custom-user-page')])if(!wanted.has(existing.id)){const frame=existing.querySelector('iframe.jacob-sdk-frame');if(frame)clearActionsForFrame(frame,'target tab was removed');existing.remove()}\n",
)
replace_once(
    "internal/webui/assets/app.js",
    "      if(cached&&cached.updatedAt===tab.updatedAt)applySavedTabHTML({...tab,html:cached.html});else if(iframe.dataset.updatedAt&&iframe.dataset.updatedAt!==String(tab.updatedAt||'')){iframe.removeAttribute('srcdoc');iframe.dataset.updatedAt=''}\n",
    "      if(cached&&cached.updatedAt===tab.updatedAt)applySavedTabHTML({...tab,html:cached.html});else if(iframe.dataset.updatedAt&&iframe.dataset.updatedAt!==String(tab.updatedAt||'')){clearActionsForFrame(iframe);iframe.removeAttribute('srcdoc');iframe.dataset.sdkReady='';iframe.dataset.updatedAt=''}\n",
)
replace_once(
    "internal/webui/assets/app.js",
    """    if(m.kind==='ready'){seedFrame(frame);return}
    if(m.kind==='request'){
      if(!allowTabRequest(frame,m.method)){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error:{code:'RATE_LIMITED',message:'custom-tab request rate exceeded'}});return}
      if(m.method==='video.url'){
""",
    """    if(m.kind==='ready'){markFrameReady(frame);seedFrame(frame);return}
    if(m.kind==='action-result'){const pending=state.actionInvocations.get(m.id);if(!pending||pending.targetFrame!==frame)return;clearTimeout(pending.timer);state.actionInvocations.delete(m.id);m.ok?pending.resolve(m.result):pending.reject(m.error||actionError('ACTION_FAILED','action failed'));return}
    if(m.kind==='request'){
      if(!allowTabRequest(frame,m.method)){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error:{code:'RATE_LIMITED',message:'custom-tab request rate exceeded'}});return}
      if(m.method==='actions.register'){try{const result=registerTabAction(frame,m.params);sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return}
      if(m.method==='actions.unregister'){try{const result=unregisterTabAction(frame,m.params?.name);sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return}
      if(m.method==='actions.list'){try{if(m.params?.discover!==false)await discoverTabActions();sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result:{actions:listRegisteredActions()}})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return}
      if(m.method==='actions.invoke'){if(!requireTabCapability(frame,'intertab')){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error:{code:'PERMISSION_DENIED',message:'inter-tab control permission was not granted'}});return}try{const result=await invokeRegisteredAction(frame,m.params);sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return}
      if(m.method==='tabs.sdk.list'){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result:{tabs:sdkTabList()}});return}
      if(m.method==='tabs.activate'){try{const result=await activateSDKTab(m.params?.target);sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:true,result})}catch(error){sendToFrame(frame,{channel:'jacob-host',kind:'response',id:m.id,ok:false,error})}return}
      if(m.method==='video.url'){
""",
)

replace_once(
    "internal/webui/assets/index.html",
    '<article class="card"><div class="card-head"><div><h2>Navigation manifest</h2><p class="muted">Set the order of every tab. Default pages may be taken off the navigation bar; Tab Manager remains available for recovery.</p></div></div><div id="navigation-list" class="saved-tab-list"></div><pre id="navigation-result" class="output small">Navigation changes are stored with the tab manifest.</pre></article>',
    '<article class="card"><div class="card-head"><div><h2>Navigation manifest</h2><p class="muted">Set the order and visibility of every default and custom tab. Hidden custom tabs stay installed and may continue running; Tab Manager remains available for recovery.</p></div></div><div id="navigation-list" class="saved-tab-list"></div><pre id="navigation-result" class="output small">Navigation changes are stored with the tab manifest.</pre></article>',
)
replace_once("internal/webui/assets/index.html", "JACoB Alpha 0.2.10 · SDK 8", "JACoB Alpha 0.2.10 · SDK 9")

sdk9_section = r'''## 12.5 Cross-tab actions and tab navigation (SDK 9)

Saved tabs can publish named actions to the JACoB host. Other saved tabs can discover and invoke those actions without gaining direct access to another iframe. The host remains the broker between sandboxed tabs.

A provider registers an action:

```js
const unregister = await Elite.actions.register(
  'carrier.runLoad',
  async ({ load = 0 } = {}) => {
    await prepareRun(load);
    return { prepared: true, load };
  },
  {
    label: 'Run carrier load',
    description: 'Prepare and run a Fleet Carrier market load.'
  }
);
```

An action name is global within the current JACoB browser session. Use a stable namespace such as `carrier.*`, `mining.*`, `neutron.*`, or `race.*`. A second tab cannot silently replace an action already registered by another tab.

A controller such as Touch Deck can discover published actions:

```js
const result = await Elite.actions.list();
console.log(result.actions);
```

`Elite.actions.list()` loads saved tabs as needed so they have an opportunity to register their actions. Hidden tabs remain eligible action providers.

Invoke an action:

```js
await Elite.actions.invoke('carrier.runLoad', { load: 0 });
```

The first action invocation from a saved tab asks the user for the **Control other JACoB tabs** capability on that browser. The provider's own permissions remain in force. For example, a carrier action that sends Elite controls still requires the carrier tab to have its normal game-control permission.

Invocation waits for the provider's handler result. The default timeout is 15 seconds and can be changed per call:

```js
await Elite.actions.invoke('neutron.plotNext', null, { timeoutMs: 30000 });
```

Providers can unregister actions:

```js
await Elite.actions.unregister('carrier.runLoad');
await unregister();
```

Tab navigation is also exposed to saved tabs:

```js
const { tabs } = await Elite.tabs.list();
await Elite.tabs.activate('tab-0123456789abcdef');
await Elite.tabs.activate('Carrier Market Orders');
```

`Elite.tabs.activate()` accepts a saved-tab ID, navigation ID, saved-tab name, or default page name. Activating a hidden tab does not unhide it; it only displays that tab in the current browser.

### Action errors

Cross-tab calls may reject with:

```text
ACTION_PREVIEW
ACTION_CONFLICT
ACTION_NOT_FOUND
ACTION_NOT_OWNER
ACTION_TARGET_RELOADED
ACTION_TIMEOUT
ACTION_FAILED
TAB_NOT_FOUND
```

A target reload or deletion cancels outstanding invocations instead of leaving the caller waiting indefinitely.

### Navigation visibility

The navigation manifest can hide both default pages and saved custom tabs. **Tab Manager** remains visible as the recovery page. Hiding a custom tab does not delete it, unload an already-running iframe, clear its state, or prevent it from publishing actions.

---

'''
for doc_path in [
    "docs/JACOB_CUSTOM_TAB_REFERENCE.md",
    "internal/webui/assets/docs/JACOB_CUSTOM_TAB_REFERENCE.md",
    "cmd/installer/payload/Documentation/JACoB-Custom-Tab-Developer-Reference.md",
]:
    doc = read(doc_path)
    doc = doc.replace("SDK version: 8", "SDK version: 9")
    doc = doc.replace("Elite.api.version === 8", "Elite.api.version === 9")
    doc = doc.replace('"apiVersion": 8', '"apiVersion": 9')
    doc = doc.replace("SDK 8 also injects a restrictive Content Security Policy into custom tabs.", "SDK 9 retains the restrictive Content Security Policy introduced in SDK 8 for custom tabs.")
    doc = doc.replace("game control, public-network access, recorder, overlay, video, and derived vision.", "game control, public-network access, recorder, overlay, video, derived vision, and cross-tab action invocation.")
    doc = doc.replace("Elite.store\nElite.files\nElite.events", "Elite.store\nElite.actions\nElite.tabs\nElite.files\nElite.events")
    if "## 12.5 Cross-tab actions and tab navigation (SDK 9)" not in doc:
        anchor = "## 13. Direct WebSocket protocol\n"
        if anchor not in doc:
            raise SystemExit(f"{doc_path}: direct WebSocket anchor missing")
        doc = doc.replace(anchor, sdk9_section + anchor, 1)
    write(doc_path, doc)

sdk9_html = r'''<h2 id="125-cross-tab-actions-and-tab-navigation-sdk-9">12.5 Cross-tab actions and tab navigation (SDK 9)</h2>
<p>Saved tabs can publish named actions to the JACoB host. Other saved tabs invoke those actions through the host broker; sandboxed iframes do not receive direct access to each other.</p>
<pre><code class="language-js">const unregister = await Elite.actions.register(
  'carrier.runLoad',
  async ({ load = 0 } = {}) =&gt; {
    await prepareRun(load);
    return { prepared: true, load };
  },
  { label: 'Run carrier load', description: 'Prepare and run a Fleet Carrier market load.' }
);

const { actions } = await Elite.actions.list();
await Elite.actions.invoke('carrier.runLoad', { load: 0 });
</code></pre>
<p><code>Elite.actions.list()</code> loads saved tabs as needed so hidden tabs can publish actions. Action names are global in the current JACoB browser session; use stable namespaces such as <code>carrier.*</code>, <code>mining.*</code>, <code>neutron.*</code>, or <code>race.*</code>.</p>
<p>The first invocation from a saved tab asks for the <strong>Control other JACoB tabs</strong> capability. The provider tab still needs its normal permissions for any game control, network, overlay, recorder, video, or vision work it performs.</p>
<pre><code class="language-js">await Elite.actions.invoke('neutron.plotNext', null, { timeoutMs: 30000 });
await Elite.actions.unregister('carrier.runLoad');
await unregister();

const { tabs } = await Elite.tabs.list();
await Elite.tabs.activate('Carrier Market Orders');
</code></pre>
<p><code>Elite.tabs.activate()</code> accepts a saved-tab ID, navigation ID, saved-tab name, or default page name. Activating a hidden tab does not change its saved visibility.</p>
<p>Action errors include <code>ACTION_PREVIEW</code>, <code>ACTION_CONFLICT</code>, <code>ACTION_NOT_FOUND</code>, <code>ACTION_NOT_OWNER</code>, <code>ACTION_TARGET_RELOADED</code>, <code>ACTION_TIMEOUT</code>, <code>ACTION_FAILED</code>, and <code>TAB_NOT_FOUND</code>. Reloading or deleting a provider cancels outstanding invocations.</p>
<p>The navigation manifest may hide default pages and saved custom tabs. <strong>Tab Manager</strong> remains visible as the recovery page. Hiding a custom tab does not delete it or stop an already-running tab.</p>
'''
for html_path in [
    "internal/webui/assets/docs/developer-reference.html",
    "cmd/installer/payload/Documentation/JACoB-Custom-Tab-Developer-Reference.html",
]:
    html = read(html_path)
    html = html.replace("SDK version: 8", "SDK version: 9")
    html = html.replace("Elite.api.version === 8", "Elite.api.version === 9")
    html = html.replace('<span class="dt">&quot;apiVersion&quot;</span><span class="fu">:</span> <span class="dv">8</span>', '<span class="dt">&quot;apiVersion&quot;</span><span class="fu">:</span> <span class="dv">9</span>')
    html = html.replace("Elite.store\nElite.files\nElite.events", "Elite.store\nElite.actions\nElite.tabs\nElite.files\nElite.events")
    if "125-cross-tab-actions-and-tab-navigation-sdk-9" not in html:
        anchor = '<h2 id="13-direct-websocket-protocol">13. Direct WebSocket protocol</h2>'
        if anchor not in html:
            raise SystemExit(f"{html_path}: direct WebSocket HTML anchor missing")
        html = html.replace(anchor, sdk9_html + anchor, 1)
    write(html_path, html)

readme = read("README.md")
readme = readme.replace("**0.2.10 Alpha · SDK 8**", "**0.2.10 Alpha · SDK 9**")
readme = readme.replace("Home, Tutorial, and Settings may be removed from the navigation bar through Tab Manager. Tab Manager remains present as the recovery point.", "Any default or custom tab may be hidden from the navigation bar through Tab Manager. Hidden custom tabs remain installed and can keep running in the background. Tab Manager remains present as the recovery point.")
readme = readme.replace("SDK / protocol version **8** includes:", "SDK / protocol version **9** includes:")
marker = "SDK / protocol version **9** includes:\n\n"
if marker in readme and "host-brokered cross-tab actions" not in readme:
    readme = readme.replace(marker, marker + "- host-brokered cross-tab actions through `Elite.actions`, allowing tools such as Touch Deck to invoke actions explicitly published by another saved tab\n- `Elite.tabs.list()` / `Elite.tabs.activate()` for safe host-mediated tab discovery and navigation\n- navigation visibility for saved custom tabs as well as default pages; hidden tabs remain installed\n", 1)
write("README.md", readme)

notes = read("RELEASE-NOTES-0.2.10-alpha.md")
notes = notes.replace("# JACoB 0.2.10 Alpha — Recovery + SDK 8", "# JACoB 0.2.10 Alpha — Recovery + SDK 9")
notes = notes.replace("advances the custom-tab SDK to version 8.", "advances the custom-tab SDK to version 9.")
if "## Cross-tab actions" not in notes:
    notes = notes.rstrip() + r'''

## Cross-tab actions

- Saved tabs can publish named host-brokered actions with `Elite.actions.register()`.
- Other saved tabs can discover and invoke published actions with `Elite.actions.list()` and `Elite.actions.invoke()`.
- The action broker preserves iframe isolation; tabs never receive direct access to another tab's DOM or JavaScript context.
- Cross-tab invocation has its own per-tab/browser permission, conflict detection, timeouts, and reload/deletion cleanup.
- `Elite.tabs.list()` and `Elite.tabs.activate()` allow tools such as Touch Deck to open another JACoB tab without host DOM access.
- Action discovery can load hidden saved tabs so they can publish controls while remaining off the navigation bar.

## Navigation visibility

- Tab Manager can hide or restore saved custom tabs as well as default pages.
- Hidden custom tabs stay installed and may continue running in the background.
- Tab Manager remains visible as the recovery page.
''' + "\n"
write("RELEASE-NOTES-0.2.10-alpha.md", notes)

print("JACoB 0.2.10 recovery + SDK 9 cross-tab patch prepared successfully.")
