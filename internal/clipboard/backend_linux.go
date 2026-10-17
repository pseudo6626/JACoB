//go:build linux

package clipboard

import (
	"context"
	"os"
	"os/exec"
)

func nativeBackend() backend {
	spec, ok := linuxCommand(os.Getenv, exec.LookPath)
	if !ok {
		return backend{name: "unavailable", reason: "no session clipboard backend: install wl-clipboard for Wayland or xclip/xsel for X11, and run JACoB in the game desktop session"}
	}
	return backend{name: spec.name, write: func(ctx context.Context, text string) error {
		return runCommand(ctx, spec, text)
	}}
}
