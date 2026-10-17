//go:build linux && cgo

package platform

import (
	"fmt"
	"os"
	"path/filepath"
)

func deckGamescopePresent() bool {
	runtimeDir := os.Getenv("XDG_RUNTIME_DIR")
	if runtimeDir == "" {
		runtimeDir = fmt.Sprintf("/run/user/%d", os.Getuid())
	}
	st, err := os.Stat(filepath.Join(runtimeDir, "gamescope-0"))
	return err == nil && st.Mode()&os.ModeSocket != 0
}

func newOverlayDriver() OverlayDriver {
	if deckGamescopePresent() {
		return newVulkanOverlayDriver()
	}
	return newLinuxNativeOverlayDriver()
}
