package platform

import "testing"

func TestTextRuneSystemNameCharacters(t *testing.T) {
	for _, r := range "Sol HIP 22460 COL 285 Sector AB-C d1-2 Shinrarta Dezhra" {
		if _, _, ok := textRune(r); !ok {
			t.Fatalf("character %q should be supported", r)
		}
	}
	k, mods, ok := textRune('S')
	if !ok || k != "S" || len(mods) != 1 || mods[0] != "SHIFT" {
		t.Fatalf("capital S mapping = %q %#v %v", k, mods, ok)
	}
}
