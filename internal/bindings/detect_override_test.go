package bindings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectBindingsWithUnicodeManualOverride(t *testing.T) {
	t.Setenv("JACOB_BINDINGS_DIR", "")
	t.Setenv("EDBRIDGE_BINDINGS_DIR", "")
	dir := filepath.Join(t.TempDir(), "Привязки")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	got, _ := DetectDirectoryWithOverride(dir)
	if got != dir {
		t.Fatalf("got %q want %q", got, dir)
	}
}

func TestManualControlSchemeDirectoryIsIncluded(t *testing.T) {
	t.Setenv("JACOB_CONTROL_SCHEMES_DIR", "")
	t.Setenv("EDBRIDGE_CONTROL_SCHEMES_DIR", "")
	dir := filepath.Join(t.TempDir(), "Схемы управления")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	got := DetectControlSchemeDirectoriesWithOverrides([]string{dir})
	found := false
	for _, item := range got {
		if item == dir {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("manual ControlSchemes directory missing from %v", got)
	}
}
