package platform

import "testing"

func TestValidateSafeChordBlocksOSShortcuts(t *testing.T) {
	bad := []struct {
		key  string
		mods []string
	}{
		{"TAB", []string{"ALT"}},
		{"ESC", []string{"ALT"}},
		{"F4", []string{"LEFTALT"}},
		{"ESCAPE", []string{"CTRL"}},
		{"ESC", []string{"CONTROL", "SHIFT"}},
	}
	for _, tc := range bad {
		if err := ValidateSafeChord(tc.key, tc.mods); err == nil {
			t.Fatalf("expected %v+%s to be blocked", tc.mods, tc.key)
		}
	}
}

func TestValidateSafeChordAllowsGameChords(t *testing.T) {
	good := []struct {
		key  string
		mods []string
	}{
		{"A", nil}, {"ENTER", nil}, {"F10", nil}, {"A", []string{"CTRL"}}, {"ENTER", []string{"ALT"}},
	}
	for _, tc := range good {
		if err := ValidateSafeChord(tc.key, tc.mods); err != nil {
			t.Fatalf("unexpected rejection of %v+%s: %v", tc.mods, tc.key, err)
		}
	}
}

func TestParseXWindowID(t *testing.T) {
	if got := parseXWindowID("GAMESCOPE_FOCUSED_WINDOW(WINDOW): window id # 0x1400001"); got != "0x1400001" {
		t.Fatalf("got %q", got)
	}
}

func TestParseXPropPID(t *testing.T) {
	if got := parseXPropPID("_NET_WM_PID(CARDINAL) = 12345\nWM_CLASS = foo"); got != 12345 {
		t.Fatalf("got %d", got)
	}
}
