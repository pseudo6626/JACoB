package clipboard

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type commandSpec struct {
	name, path string
	args       []string
}

// Prefer the active Wayland clipboard, then X11 where a display is available.
// No shell is used, and caller text is sent only over stdin.
func linuxCommand(getenv func(string) string, lookPath func(string) (string, error)) (commandSpec, bool) {
	candidates := []commandSpec{}
	if getenv("WAYLAND_DISPLAY") != "" {
		candidates = append(candidates, commandSpec{name: "wl-copy", args: []string{"--type", "text/plain;charset=utf-8"}})
	}
	if getenv("DISPLAY") != "" {
		candidates = append(candidates,
			commandSpec{name: "xclip", args: []string{"-selection", "clipboard", "-in"}},
			commandSpec{name: "xsel", args: []string{"--clipboard", "--input"}})
	}
	for _, candidate := range candidates {
		if path, err := lookPath(candidate.name); err == nil {
			candidate.path = path
			return candidate, true
		}
	}
	return commandSpec{}, false
}

func runCommand(ctx context.Context, spec commandSpec, text string) error {
	cmd := exec.CommandContext(ctx, spec.path, spec.args...)
	cmd.Stdin = strings.NewReader(text)
	// Clipboard utilities fork an owner to keep the selection alive. Do not
	// create stdout/stderr pipes that this child could retain and block Wait.
	null, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		return err
	}
	defer null.Close()
	cmd.Stdout, cmd.Stderr = null, null
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("%s could not set the session clipboard (check display access): %w", spec.name, err)
	}
	return nil
}
