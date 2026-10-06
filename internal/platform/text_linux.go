//go:build linux

package platform

import (
	"fmt"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

func typeTextNative(d InputDriver, text string, interval time.Duration) (bool, error) {
	if _, ok := d.(*linuxInput); !ok {
		return false, nil
	}
	path, err := exec.LookPath("xdotool")
	if err != nil {
		return false, nil
	}
	delay := interval.Milliseconds()
	if delay < 0 {
		delay = 0
	}
	if delay > 1000 {
		delay = 1000
	}
	args := []string{"type", "--clearmodifiers", "--delay", strconv.FormatInt(delay, 10), "--", text}
	out, err := exec.Command(path, args...).CombinedOutput()
	if err != nil {
		detail := strings.TrimSpace(string(out))
		if detail != "" {
			return true, fmt.Errorf("xdotool text input failed: %w: %s", err, detail)
		}
		return true, fmt.Errorf("xdotool text input failed: %w", err)
	}
	return true, nil
}
