package macros

import (
	"os"
	"strings"
	"testing"
)

func accept(t *testing.T, s *Store) {
	t.Helper()
	state, err := s.AcceptPolicy()
	if err != nil {
		t.Fatal(err)
	}
	if !state.Accepted || state.Version != policyVersion || state.MaxCustomSteps != maxCustomSteps {
		t.Fatalf("policy=%+v", state)
	}
}

func TestDefaultsAreLogisticsAndNavigation(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	for _, forbidden := range []string{"toggle_analysis_mode", "enter_fss", "leave_fss"} {
		if _, ok := s.Resolve(forbidden); ok {
			t.Fatalf("non-logistics built-in %s should not ship", forbidden)
		}
	}
	for _, id := range []string{"exit_menus", "open_galaxy_map", "open_system_map", "open_navigation_panel", "open_comms_panel", "open_role_panel", "open_internal_panel", "open_quick_comms", "open_codex_discovery", "plot_route"} {
		def, ok := s.Resolve(id)
		if !ok || !def.BuiltIn {
			t.Fatalf("built-in %s missing", id)
		}
		for i, step := range def.Steps {
			if err := validateBuiltInStep(step, i+1); err != nil {
				t.Fatalf("%s: %v", id, err)
			}
		}
	}
	route, _ := s.Resolve("plot_route")
	if len(route.Steps) <= maxCustomSteps {
		t.Fatalf("route should exercise separate built-in limit: %d", len(route.Steps))
	}
	if got, err := ExpandText("{{system}}", map[string]string{"system": "Sol"}); err != nil || got != "Sol" {
		t.Fatalf("expand=%q err=%v", got, err)
	}
}

func TestPolicyRequiredAndPersists(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if s.Policy().Accepted {
		t.Fatal("policy unexpectedly accepted")
	}
	if _, err := s.Save(Definition{ID: "hello", Name: "Hello", Steps: []Step{{Type: "binding", Action: "UI_Select"}}}); err == nil {
		t.Fatal("custom macro save should require agreement")
	}
	accept(t, s)
	reloaded, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if !reloaded.Policy().Accepted {
		t.Fatal("policy agreement did not persist")
	}
}

func TestCustomStepLimit(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	accept(t, s)
	steps := make([]Step, maxCustomSteps+1)
	for i := range steps {
		steps[i] = Step{Type: "binding", Action: "UI_Select"}
	}
	if _, err := s.Save(Definition{ID: "too-long", Name: "Too long", Steps: steps}); err == nil {
		t.Fatalf("custom macro over %d steps accepted", maxCustomSteps)
	}
}

func TestCustomPersists(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	accept(t, s)
	saved, err := s.Save(Definition{Name: "Привет macro", Steps: []Step{{Type: "text", Text: "Привет"}}})
	if err != nil {
		t.Fatal(err)
	}
	reloaded, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := reloaded.Resolve(saved.ID)
	if !ok || got.Name != "Привет macro" {
		t.Fatalf("got=%+v ok=%v", got, ok)
	}
}

func TestBuiltInEditingRestrictedToNavigation(t *testing.T) {
	s, err := New(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	accept(t, s)
	d, _ := s.Resolve("open_galaxy_map")
	d.Steps = []Step{{Type: "binding", Action: "UI_Select"}}
	if _, err := s.Save(d); err != nil {
		t.Fatal(err)
	}
	d.Steps = []Step{{Type: "binding", Action: "FirePrimary"}}
	if _, err := s.Save(d); err == nil {
		t.Fatal("combat action accepted in built-in macro")
	}
	d.Steps = []Step{{Type: "raw", Key: "SC:39", HoldMs: 60}}
	if _, err := s.Save(d); err == nil {
		t.Fatal("non-Escape raw key accepted in built-in macro")
	}
	reset, err := s.Reset("open_galaxy_map")
	if err != nil || reset.Steps[0].Action != "GalaxyMapOpen" {
		t.Fatalf("reset=%+v err=%v", reset, err)
	}
}

func TestValidationAndAtomicFile(t *testing.T) {
	root := t.TempDir()
	s, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	accept(t, s)
	if _, err := s.Save(Definition{ID: "bad id", Name: "Bad", Steps: []Step{{Type: "binding", Action: "UI_Select"}}}); err == nil {
		t.Fatal("bad id accepted")
	}
	if _, err := s.Save(Definition{ID: "bad-param", Name: "Bad", Steps: []Step{{Type: "text", Text: "{{system}}"}}}); err == nil {
		t.Fatal("undeclared parameter accepted")
	}
	if _, err := s.Save(Definition{ID: "hello", Name: "Hello", Steps: []Step{{Type: "binding", Action: "UI_Select"}}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(s.Path())
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), `"hello"`) || !strings.Contains(string(b), `"acceptedAt"`) {
		t.Fatal("macro or policy agreement not persisted")
	}
	if _, err := os.Stat(s.Path() + ".tmp"); !os.IsNotExist(err) {
		t.Fatalf("temp file remains: %v", err)
	}
}
