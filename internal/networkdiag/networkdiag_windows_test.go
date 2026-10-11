//go:build windows

package networkdiag

import "testing"

func TestHiddenCommandDoesNotAllocateConsoleWindow(t *testing.T) {
	cmd := hiddenCommand("powershell.exe", "-NoProfile")
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow {
		t.Fatal("hidden command must set HideWindow")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("hidden command creation flags=%#x", cmd.SysProcAttr.CreationFlags)
	}
}
