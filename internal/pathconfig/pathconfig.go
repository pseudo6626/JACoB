package pathconfig

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const schemaVersion = 1

type Config struct {
	JournalDir        string   `json:"journalDir,omitempty"`
	BindingsDir       string   `json:"bindingsDir,omitempty"`
	ControlSchemeDirs []string `json:"controlSchemeDirs,omitempty"`
}

type diskConfig struct {
	SchemaVersion int    `json:"schemaVersion"`
	Paths         Config `json:"paths"`
}

func FilePath(dataDir string) string {
	return filepath.Join(dataDir, "paths.json")
}

func Normalize(cfg Config) Config {
	cfg.JournalDir = clean(cfg.JournalDir)
	cfg.BindingsDir = clean(cfg.BindingsDir)
	out := make([]string, 0, len(cfg.ControlSchemeDirs))
	seen := map[string]bool{}
	for _, raw := range cfg.ControlSchemeDirs {
		v := clean(raw)
		if v == "" {
			continue
		}
		key := v
		if runtime.GOOS == "windows" {
			key = strings.ToLower(v)
		}
		if seen[key] {
			continue
		}
		seen[key] = true
		out = append(out, v)
	}
	cfg.ControlSchemeDirs = out
	return cfg
}

func clean(v string) string {
	v = strings.TrimSpace(v)
	if v == "" {
		return ""
	}
	return filepath.Clean(v)
}

func Load(dataDir string) (Config, error) {
	path := FilePath(dataDir)
	b, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return Config{}, nil
	}
	if err != nil {
		return Config{}, err
	}
	var f diskConfig
	if err := json.Unmarshal(b, &f); err != nil {
		return Config{}, fmt.Errorf("read %s: %w", path, err)
	}
	if f.SchemaVersion != 0 && f.SchemaVersion != schemaVersion {
		return Config{}, fmt.Errorf("unsupported path settings schema %d", f.SchemaVersion)
	}
	return Normalize(f.Paths), nil
}

func Save(dataDir string, cfg Config) error {
	cfg = Normalize(cfg)
	if err := Validate(cfg); err != nil {
		return err
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	b, err := json.MarshalIndent(diskConfig{SchemaVersion: schemaVersion, Paths: cfg}, "", "  ")
	if err != nil {
		return err
	}
	b = append(b, '\n')
	path := FilePath(dataDir)
	tmp := path + ".tmp"
	backup := path + ".previous"
	_ = os.Remove(tmp)
	_ = os.Remove(backup)
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	hadPrevious := false
	if _, err := os.Stat(path); err == nil {
		if err := os.Rename(path, backup); err != nil {
			_ = os.Remove(tmp)
			return err
		}
		hadPrevious = true
	} else if !os.IsNotExist(err) {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		if hadPrevious {
			_ = os.Rename(backup, path)
		}
		_ = os.Remove(tmp)
		return err
	}
	if hadPrevious {
		_ = os.Remove(backup)
	}
	return nil
}

func Validate(cfg Config) error {
	cfg = Normalize(cfg)
	for _, item := range []struct {
		label string
		path  string
	}{
		{"journal directory", cfg.JournalDir},
		{"bindings directory", cfg.BindingsDir},
	} {
		if item.path == "" {
			continue
		}
		if err := validateDir(item.label, item.path); err != nil {
			return err
		}
	}
	for _, path := range cfg.ControlSchemeDirs {
		if err := validateDir("control schemes directory", path); err != nil {
			return err
		}
	}
	return nil
}

func validateDir(label, path string) error {
	if !filepath.IsAbs(path) {
		return fmt.Errorf("%s %q must be an absolute path", label, path)
	}
	st, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("%s %q is unavailable: %w", label, path, err)
	}
	if !st.IsDir() {
		return fmt.Errorf("%s %q is not a directory", label, path)
	}
	return nil
}

func pathEqual(a, b string) bool {
	if runtime.GOOS == "windows" {
		return strings.EqualFold(a, b)
	}
	return a == b
}

func Equal(a, b Config) bool {
	a, b = Normalize(a), Normalize(b)
	if !pathEqual(a.JournalDir, b.JournalDir) || !pathEqual(a.BindingsDir, b.BindingsDir) {
		return false
	}
	if len(a.ControlSchemeDirs) != len(b.ControlSchemeDirs) {
		return false
	}
	for i := range a.ControlSchemeDirs {
		if !pathEqual(a.ControlSchemeDirs[i], b.ControlSchemeDirs[i]) {
			return false
		}
	}
	return true
}
