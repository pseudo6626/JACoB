package journal

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectDirectoryWithUnicodeManualOverride(t *testing.T) {
	t.Setenv("JACOB_JOURNAL_DIR", "")
	t.Setenv("EDBRIDGE_JOURNAL_DIR", "")
	dir := filepath.Join(t.TempDir(), "Журналы Elite")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	got, _ := DetectDirectoryWithOverride(dir)
	if got != dir {
		t.Fatalf("got %q want %q", got, dir)
	}
}

func TestJournalEnvironmentOverridePrecedesManual(t *testing.T) {
	envDir := filepath.Join(t.TempDir(), "env")
	manualDir := filepath.Join(t.TempDir(), "manual")
	for _, dir := range []string{envDir, manualDir} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("JACOB_JOURNAL_DIR", envDir)
	t.Setenv("EDBRIDGE_JOURNAL_DIR", "")
	got, _ := DetectDirectoryWithOverride(manualDir)
	if got != envDir {
		t.Fatalf("got %q want environment override %q", got, envDir)
	}
}
