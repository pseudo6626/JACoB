package platform

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
		{"SC:0F", []string{"SC:38"}},    // Alt+Tab
		{"SC:01", []string{"SC:1D"}},    // Ctrl+Esc
		{"SC:3E", []string{"SC:E0:38"}}, // AltGr/Alt+F4 physical expression
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
