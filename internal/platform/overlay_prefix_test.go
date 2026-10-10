package platform

import "testing"

func TestOverlayStateClearPrefixRemovesOwnedNamespaceOnly(t *testing.T) {
	s := newOverlayState()
	s.set("tab:alpha", OverlayScene{})
	s.set("tab:alpha:vision-debug", OverlayScene{})
	s.set("tab:alpha:hud:secondary", OverlayScene{})
	s.set("tab:alphabet", OverlayScene{})
	s.set("tab:beta", OverlayScene{})
	s.set("preview", OverlayScene{})
	s.clearPrefix("tab:alpha")
	layers, _ := s.snapshot()
	for _, layer := range []string{"tab:alpha", "tab:alpha:vision-debug", "tab:alpha:hud:secondary"} {
		if _, ok := layers[layer]; ok {
			t.Fatalf("owned layer %q survived prefix cleanup", layer)
		}
	}
	for _, layer := range []string{"tab:alphabet", "tab:beta", "preview"} {
		if _, ok := layers[layer]; !ok {
			t.Fatalf("unrelated layer %q was removed", layer)
		}
	}
}
