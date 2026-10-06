package platform

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
