//go:build windows

package platform

import "testing"

func TestWindowsUnicodeFallbackCyrillic(t *testing.T) {
	cases := map[rune]uint16{
		'П': 0x041F,
		'р': 0x0440,
		'и': 0x0438,
		'в': 0x0432,
		'е': 0x0435,
		'т': 0x0442,
	}
	for r, want := range cases {
		units := windowsUTF16Units(r)
		if len(units) != 1 || units[0] != want {
			t.Fatalf("%q units=%v want=%04X", r, units, want)
		}
	}
}

func TestWindowsUnicodeFallbackSurrogatePair(t *testing.T) {
	units := windowsUTF16Units('🚀')
	if len(units) != 2 {
		t.Fatalf("units=%v", units)
	}
}
