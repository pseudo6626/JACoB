package bindings

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestParseKeyboardChord(t *testing.T) {
	dir := t.TempDir()
	xmlData := `<?xml version="1.0"?><Root PresetName="Test" MajorVersion="4" MinorVersion="2">
<GalaxyMapOpen><Primary Device="Keyboard" Key="Key_G"><Modifier Device="Keyboard" Key="Key_LeftControl" /></Primary><Secondary Device="{NoDevice}" Key="" /></GalaxyMapOpen>
<UI_Select><Primary Device="Keyboard" Key="Key_Space" /><Secondary Device="{NoDevice}" Key="" /></UI_Select>
</Root>`
	path := filepath.Join(dir, "Custom.4.2.binds")
	if err := os.WriteFile(path, []byte(xmlData), 0644); err != nil {
		t.Fatal(err)
	}
	actions, preset, err := parseFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if preset != "Test" {
		t.Fatalf("preset=%q", preset)
	}
	a, ok := actions["GalaxyMapOpen"]
	if !ok {
		t.Fatal("GalaxyMapOpen missing")
	}
	key, mods, ok := KeyboardChord(a.Primary)
	if !ok {
		t.Fatal("keyboard chord not resolved")
	}
	if key != "G" {
		t.Fatalf("key=%q", key)
	}
	if len(mods) != 1 || mods[0] != "CTRL" {
		t.Fatalf("mods=%v", mods)
	}
	selectAction := actions["UI_Select"]
	key, mods, ok = KeyboardChord(selectAction.Primary)
	if !ok || key != "SPACE" || len(mods) != 0 {
		t.Fatalf("select chord key=%q mods=%v ok=%v", key, mods, ok)
	}
}

func TestEnsureMissingPrimaryBindingsOnlyFillsEmptyActions(t *testing.T) {
	dir := t.TempDir()
	xmlData := `<?xml version="1.0"?><Root PresetName="Test" MajorVersion="4" MinorVersion="2">
<AlreadyBound><Primary Device="Keyboard" Key="Key_G" /><Secondary Device="{NoDevice}" Key="" /></AlreadyBound>
<ExistingSecondary><Primary Device="{NoDevice}" Key="" /><Secondary Device="Keyboard" Key="Key_H" /></ExistingSecondary>
<CompletelyUnbound><Primary Device="{NoDevice}" Key="" /><Secondary Device="{NoDevice}" Key="" /></CompletelyUnbound>
<AxisOnly><Binding Device="Mouse" Key="Mouse_XAxis" /></AxisOnly>
</Root>`
	path := filepath.Join(dir, "Custom.4.2.binds")
	if err := os.WriteFile(path, []byte(xmlData), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "StartPreset.start"), []byte("Test\n"), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(dir)
	report, err := s.EnsureMissingPrimaryBindings()
	if err != nil {
		t.Fatal(err)
	}
	if report.Assigned != 1 {
		t.Fatalf("assigned=%d report=%+v", report.Assigned, report)
	}
	if report.BackupPath == "" {
		t.Fatal("expected selector backup path")
	}
	sourceAfter, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(sourceAfter) != xmlData {
		t.Fatal("source preset was modified")
	}
	if s.ActiveFile() == path || len(report.CreatedFiles) != 1 {
		t.Fatalf("expected one active clone, active=%q report=%+v", s.ActiveFile(), report)
	}
	if _, err := os.Stat(report.BackupPath); err != nil {
		t.Fatalf("backup missing: %v", err)
	}

	bound, _ := s.Get("AlreadyBound")
	if bound.Primary.Key != "Key_G" || bound.Secondary.Key != "" {
		t.Fatalf("bound action changed: %+v", bound)
	}
	existing, _ := s.Get("ExistingSecondary")
	if existing.Secondary.Key != "Key_H" {
		t.Fatalf("existing secondary changed: %+v", existing)
	}
	filled, ok := s.Get("CompletelyUnbound")
	if !ok {
		t.Fatal("unbound action disappeared")
	}
	if filled.Primary.Key != "" {
		t.Fatalf("primary should remain empty: %+v", filled)
	}
	if filled.Secondary.Key == "" || filled.Secondary.Device != "Keyboard" {
		t.Fatalf("secondary not filled: %+v", filled)
	}
	if _, ok := s.Get("AxisOnly"); ok {
		t.Fatal("axis-only node should not be treated as discrete action")
	}

	report2, err := s.EnsureMissingPrimaryBindings()
	if err != nil {
		t.Fatal(err)
	}
	if report2.Assigned != 0 {
		t.Fatalf("second scan should be idempotent, assigned=%d", report2.Assigned)
	}
	if report2.BackupPath != "" {
		t.Fatalf("second scan should not create backup: %s", report2.BackupPath)
	}
}

func TestInjectSecondaryBindingsPreservesEverythingElseByteForByte(t *testing.T) {
	original := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\" ?>\r\n<!-- keep-this-comment -->\r\n<Root PresetName=\"Custom\" MajorVersion=\"4\" MinorVersion=\"2\">\r\n\t<Alpha>\r\n\t\t<Primary Device=\"{NoDevice}\" Key=\"\" />\r\n\t\t<Secondary Device=\"{NoDevice}\" Key=\"\" />\r\n\t</Alpha>\r\n\t<Beta>\r\n\t\t<Primary Device=\"Keyboard\" Key=\"Key_B\" />\r\n\t\t<Secondary Device=\"{NoDevice}\" Key=\"\" />\r\n\t</Beta>\r\n</Root>\r\n")
	assignments := map[string]Assignment{
		"Alpha": {Action: "Alpha", Key: "Key_A", Modifiers: []string{"Key_LeftControl", "Key_LeftAlt"}},
	}
	updated, err := injectSecondaryBindings(original, assignments)
	if err != nil {
		t.Fatal(err)
	}
	expected := []byte("<?xml version=\"1.0\" encoding=\"UTF-8\" ?>\r\n<!-- keep-this-comment -->\r\n<Root PresetName=\"Custom\" MajorVersion=\"4\" MinorVersion=\"2\">\r\n\t<Alpha>\r\n\t\t<Primary Device=\"{NoDevice}\" Key=\"\" />\r\n\t\t<Secondary Device=\"Keyboard\" Key=\"Key_A\"><Modifier Device=\"Keyboard\" Key=\"Key_LeftControl\" /><Modifier Device=\"Keyboard\" Key=\"Key_LeftAlt\" /></Secondary>\r\n\t</Alpha>\r\n\t<Beta>\r\n\t\t<Primary Device=\"Keyboard\" Key=\"Key_B\" />\r\n\t\t<Secondary Device=\"{NoDevice}\" Key=\"\" />\r\n\t</Beta>\r\n</Root>\r\n")
	if string(updated) != string(expected) {
		t.Fatalf("surgical patch changed unexpected bytes\nGOT:\n%s\nWANT:\n%s", updated, expected)
	}
	if err := validatePatchedBindings(original, updated, assignments); err != nil {
		t.Fatalf("validation failed: %v", err)
	}
}

func TestOdysseySelectorAndBuiltinPresetResolution(t *testing.T) {
	local := t.TempDir()
	builtin := t.TempDir()
	t.Setenv("JACOB_CONTROL_SCHEMES_DIR", builtin)
	if err := os.WriteFile(filepath.Join(local, "StartPreset.4.start"), []byte("KeyboardMouseOnly\nKeyboardMouseOnly\nKeyboardMouseOnly\nKeyboardMouseOnly\n"), 0644); err != nil {
		t.Fatal(err)
	}
	xmlData := `<?xml version="1.0"?><Root PresetName="KeyboardMouseOnly">
<UI_Up><Primary Device="Keyboard" Key="Key_W" /><Secondary Device="{NoDevice}" Key="" /></UI_Up>
<MouseGUI><Primary Device="Mouse" Key="Mouse_1" /><Secondary Device="{NoDevice}" Key="" /></MouseGUI>
<CompletelyUnbound><Primary Device="{NoDevice}" Key="" /><Secondary Device="{NoDevice}" Key="" /></CompletelyUnbound>
</Root>`
	path := filepath.Join(builtin, "KeyboardMouseOnly.binds")
	if err := os.WriteFile(path, []byte(xmlData), 0644); err != nil {
		t.Fatal(err)
	}
	s := New(local)
	if s.ActiveSource() != "builtin" {
		t.Fatalf("source=%q want builtin", s.ActiveSource())
	}
	if s.ActiveFile() != path {
		t.Fatalf("active=%q want %q", s.ActiveFile(), path)
	}
	if _, ok := s.Get("UI_Up"); !ok {
		t.Fatal("built-in action not resolved")
	}
	d := s.Diagnostics()
	if !d.ReadOnly {
		t.Fatal("built-in preset should be read-only")
	}
	if len(d.SelectorOdyssey) != 4 {
		t.Fatalf("selector=%v", d.SelectorOdyssey)
	}
	report, err := s.EnsureMissingPrimaryBindings()
	if err != nil {
		t.Fatalf("autofill should clone shipped preset: %v", err)
	}
	if report.Assigned != 1 || len(report.CreatedFiles) != 1 {
		t.Fatalf("report=%+v", report)
	}
	if s.ActiveSource() != "user" {
		t.Fatalf("source=%q want user clone", s.ActiveSource())
	}
	if s.ActiveFile() == path {
		t.Fatal("active preset should be the new user clone, not the shipped source")
	}
	cloneBytes, err := os.ReadFile(s.ActiveFile())
	if err != nil {
		t.Fatal(err)
	}
	clonePreset, major, minor, err := readRootMeta(cloneBytes)
	if err != nil {
		t.Fatal(err)
	}
	if clonePreset != "JACoB - KeyboardMouseOnly" || major != "4" || minor != "2" {
		t.Fatalf("clone meta preset=%q version=%s.%s", clonePreset, major, minor)
	}
	original, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(original) != xmlData {
		t.Fatal("shipped source preset was modified")
	}
}

func TestAutofillClonesMultipleOdysseyPresetSelections(t *testing.T) {
	dir := t.TempDir()
	aData := `<?xml version="1.0"?><Root PresetName="GeneralA" MajorVersion="4" MinorVersion="2">
<AOnly><Primary Device="{NoDevice}" Key="" /><Secondary Device="{NoDevice}" Key="" /></AOnly>
<Shared><Primary Device="Keyboard" Key="Key_S" /><Secondary Device="{NoDevice}" Key="" /></Shared>
</Root>`
	bData := `<?xml version="1.0"?><Root PresetName="ShipB" MajorVersion="4" MinorVersion="2">
<BOnly><Primary Device="{NoDevice}" Key="" /><Secondary Device="{NoDevice}" Key="" /></BOnly>
<Shared><Primary Device="Keyboard" Key="Key_S" /><Secondary Device="{NoDevice}" Key="" /></Shared>
</Root>`
	aPath := filepath.Join(dir, "GeneralA.4.2.binds")
	bPath := filepath.Join(dir, "ShipB.4.2.binds")
	if err := os.WriteFile(aPath, []byte(aData), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(bPath, []byte(bData), 0644); err != nil {
		t.Fatal(err)
	}
	selector := "GeneralA\nShipB\nGeneralA\nShipB\n"
	if err := os.WriteFile(filepath.Join(dir, "StartPreset.4.start"), []byte(selector), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(dir)
	if len(s.ActiveFiles()) != 2 {
		t.Fatalf("active=%v", s.ActiveFiles())
	}
	report, err := s.EnsureMissingPrimaryBindings()
	if err != nil {
		t.Fatal(err)
	}
	if report.Assigned != 2 || len(report.CreatedFiles) != 2 || len(report.Clones) != 2 {
		t.Fatalf("report=%+v", report)
	}
	gotSelector, err := os.ReadFile(filepath.Join(dir, "StartPreset.4.start"))
	if err != nil {
		t.Fatal(err)
	}
	wantSelector := "JACoB - GeneralA\nJACoB - ShipB\nJACoB - GeneralA\nJACoB - ShipB\n"
	if string(gotSelector) != wantSelector {
		t.Fatalf("selector=%q want %q", gotSelector, wantSelector)
	}
	gotA, _ := os.ReadFile(aPath)
	gotB, _ := os.ReadFile(bPath)
	if string(gotA) != aData || string(gotB) != bData {
		t.Fatal("source preset was modified")
	}
	if len(s.ActiveFiles()) != 2 {
		t.Fatalf("cloned active files=%v", s.ActiveFiles())
	}
	for _, p := range s.ActiveFiles() {
		if p == aPath || p == bPath {
			t.Fatalf("source still active: %s", p)
		}
	}
}

func TestDiagnosticsDetectsRelevantLoaderErrorsAndDuplicateElements(t *testing.T) {
	dir := t.TempDir()
	xmlData := `<?xml version="1.0"?><Root PresetName="Test" MajorVersion="4" MinorVersion="2">
<MouseGUI><Primary Device="Mouse" Key="Mouse_1" /><Secondary Device="{NoDevice}" Key="" /></MouseGUI>
<MouseGUI><Primary Device="Mouse" Key="Mouse_2" /><Secondary Device="{NoDevice}" Key="" /></MouseGUI>
</Root>`
	path := filepath.Join(dir, "Custom.4.2.binds")
	if err := os.WriteFile(path, []byte(xmlData), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "StartPreset.4.start"), []byte("Test\nTest\nTest\nTest\n"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	logData := "There where errors when loading preset file: Custom.4.2.binds\nThere are multiple entries of binding \"MouseGUI\" Only the first will be used\nMissing devices: Mouse\n"
	if err := os.WriteFile(filepath.Join(dir, "BindingLoadingErrors.log"), []byte(logData), 0644); err != nil {
		t.Fatal(err)
	}
	s := New(dir)
	d := s.Diagnostics()
	if len(d.DuplicateElements) != 1 || d.DuplicateElements[0] != "MouseGUI" {
		t.Fatalf("duplicates=%v", d.DuplicateElements)
	}
	if !d.Loader.Relevant || !d.Loader.HasErrors {
		t.Fatalf("loader=%+v", d.Loader)
	}
	if len(d.Loader.MissingDevices) != 1 || d.Loader.MissingDevices[0] != "Mouse" {
		t.Fatalf("missing=%v", d.Loader.MissingDevices)
	}
	if _, err := s.EnsureMissingPrimaryBindings(); err == nil {
		t.Fatal("autofill should refuse malformed/erroring preset")
	}
}

func TestMouseGUIDuplicateIsKnownNonFatal(t *testing.T) {
	blocking := FilterBlockingDuplicateBindings([]string{"MouseGUI", "UI_Select"})
	if len(blocking) != 1 || blocking[0] != "UI_Select" {
		t.Fatalf("blocking=%v", blocking)
	}
	nonFatal := FilterNonFatalDuplicateBindings([]string{"MouseGUI", "UI_Select"})
	if len(nonFatal) != 1 || nonFatal[0] != "MouseGUI" {
		t.Fatalf("nonFatal=%v", nonFatal)
	}
}

func TestAutofillAllowsMouseGUIDuplicateWithoutOtherLoaderBlockers(t *testing.T) {
	dir := t.TempDir()
	xmlData := `<?xml version="1.0"?><Root PresetName="Test" MajorVersion="4" MinorVersion="2">
<MouseGUI><Primary Device="Mouse" Key="Mouse_1" /><Secondary Device="{NoDevice}" Key="" /></MouseGUI>
<MouseGUI><Primary Device="Mouse" Key="Mouse_2" /><Secondary Device="{NoDevice}" Key="" /></MouseGUI>
<UI_Select><Primary Device="{NoDevice}" Key="" /><Secondary Device="{NoDevice}" Key="" /></UI_Select>
</Root>`
	path := filepath.Join(dir, "Custom.4.2.binds")
	if err := os.WriteFile(path, []byte(xmlData), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "StartPreset.4.start"), []byte("Test\nTest\nTest\nTest\n"), 0644); err != nil {
		t.Fatal(err)
	}
	time.Sleep(5 * time.Millisecond)
	logData := "There where errors when loading preset file: Custom.4.2.binds\nThere are multiple entries of binding \"MouseGUI\" Only the first will be used\n"
	if err := os.WriteFile(filepath.Join(dir, "BindingLoadingErrors.log"), []byte(logData), 0644); err != nil {
		t.Fatal(err)
	}

	s := New(dir)
	d := s.Diagnostics()
	if !d.Loader.Relevant {
		t.Fatalf("loader should be relevant: %+v", d.Loader)
	}
	if got := FilterBlockingDuplicateBindings(d.DuplicateElements); len(got) != 0 {
		t.Fatalf("MouseGUI should not be blocking: %v", got)
	}
	report, err := s.EnsureMissingPrimaryBindings()
	if err != nil {
		t.Fatalf("MouseGUI-only duplicate warning should not block clone/autofill: %v", err)
	}
	if report.Assigned != 1 {
		t.Fatalf("assigned=%d want 1", report.Assigned)
	}
}
