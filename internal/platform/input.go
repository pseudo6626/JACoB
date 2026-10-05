package platform

import (
	"fmt"
	"sort"
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

// ValidateSafeChord rejects operating-system/global shortcuts that can escape
// the Elite Dangerous window even when Elite is foreground. JACoB is a game
// control bridge, not a general desktop automation API.
func ValidateSafeChord(key string, modifiers []string) error {
	k := strings.ToUpper(strings.TrimSpace(key))
	mods := map[string]bool{}
	for _, m := range modifiers {
		s := strings.ToUpper(strings.TrimSpace(m))
		switch s {
		case "CONTROL", "LEFTCONTROL", "RIGHTCONTROL":
			s = "CTRL"
		case "LEFTALT", "RIGHTALT":
			s = "ALT"
		case "LEFTSHIFT", "RIGHTSHIFT":
			s = "SHIFT"
		}
		if s != "" {
			mods[s] = true
		}
	}
	unsafe := false
	switch {
	case mods["ALT"] && (k == "TAB" || k == "ESC" || k == "ESCAPE" || k == "F4"):
		unsafe = true
	case mods["CTRL"] && (k == "ESC" || k == "ESCAPE"):
		unsafe = true
	case mods["CTRL"] && mods["SHIFT"] && (k == "ESC" || k == "ESCAPE"):
		unsafe = true
	}
	if unsafe {
		names := make([]string, 0, len(mods))
		for m := range mods {
			names = append(names, m)
		}
		sort.Strings(names)
		return fmt.Errorf("OS-global shortcut %s%s is blocked by JACoB", strings.Join(names, "+"), func() string {
			if len(names) > 0 {
				return "+" + k
			}
			return k
		}())
	}
	return nil
}
