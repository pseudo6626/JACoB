package journal

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

func DetectDirectory() (string, []string) {
	return DetectDirectoryWithOverride("")
}

func DetectDirectoryWithOverride(manual string) (string, []string) {
	var candidates []string
	if v := strings.TrimSpace(os.Getenv("JACOB_JOURNAL_DIR")); v != "" {
		candidates = append(candidates, v)
	} else if v := strings.TrimSpace(os.Getenv("EDBRIDGE_JOURNAL_DIR")); v != "" {
		candidates = append(candidates, v)
	}
	if v := strings.TrimSpace(manual); v != "" {
		candidates = append(candidates, v)
	}
	home, _ := os.UserHomeDir()
	if home != "" {
		if runtime.GOOS == "windows" {
			candidates = append(candidates,
				filepath.Join(home, "Saved Games", "Frontier Developments", "Elite Dangerous"),
			)
		} else if runtime.GOOS == "linux" {
			candidates = append(candidates,
				filepath.Join(home, ".steam", "steam", "steamapps", "compatdata", "359320", "pfx", "drive_c", "users", "steamuser", "Saved Games", "Frontier Developments", "Elite Dangerous"),
				filepath.Join(home, ".local", "share", "Steam", "steamapps", "compatdata", "359320", "pfx", "drive_c", "users", "steamuser", "Saved Games", "Frontier Developments", "Elite Dangerous"),
				filepath.Join(home, ".steam", "debian-installation", "steamapps", "compatdata", "359320", "pfx", "drive_c", "users", "steamuser", "Saved Games", "Frontier Developments", "Elite Dangerous"),
			)
		}
	}
	for _, c := range candidates {
		if st, err := os.Stat(c); err == nil && st.IsDir() {
			return c, candidates
		}
	}
	return "", candidates
}
