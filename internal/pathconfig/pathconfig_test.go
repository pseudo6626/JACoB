package pathconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSaveLoadUnicodePaths(t *testing.T) {
	root := t.TempDir()
	journal := filepath.Join(root, "Журналы Elite")
	bindings := filepath.Join(root, "Привязки")
	schemes := filepath.Join(root, "Схемы управления")
	for _, dir := range []string{journal, bindings, schemes} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	want := Config{JournalDir: journal, BindingsDir: bindings, ControlSchemeDirs: []string{schemes}}
	if err := Save(root, want); err != nil {
		t.Fatal(err)
	}
	got, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !Equal(got, want) {
		t.Fatalf("loaded=%+v want=%+v", got, want)
	}
}

func TestValidateRejectsMissingManualDirectory(t *testing.T) {
	cfg := Config{JournalDir: filepath.Join(t.TempDir(), "нет такой папки")}
	if err := Validate(cfg); err == nil {
		t.Fatal("expected validation error")
	}
}

func TestNormalizePathCaseMatchesHostSemantics(t *testing.T) {
	cfg := Normalize(Config{ControlSchemeDirs: []string{filepath.Join("root", "Case"), filepath.Join("root", "case")}})
	if runtime.GOOS == "windows" {
		if len(cfg.ControlSchemeDirs) != 1 {
			t.Fatalf("windows should de-duplicate case-only path variants: %v", cfg.ControlSchemeDirs)
		}
		return
	}
	if len(cfg.ControlSchemeDirs) != 2 {
		t.Fatalf("case-sensitive hosts must preserve distinct paths: %v", cfg.ControlSchemeDirs)
	}
}

func TestValidateRejectsRelativeManualDirectory(t *testing.T) {
	cfg := Config{JournalDir: "."}
	if err := Validate(cfg); err == nil {
		t.Fatal("expected relative manual path to be rejected")
	}
}
