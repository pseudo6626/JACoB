//go:build windows

package vision

import (
	"strings"
	"testing"
)

func TestOCRWorkerCommandIsHidden(t *testing.T) {
	cmd := newOCRWorkerCommand()
	if cmd.SysProcAttr == nil {
		t.Fatal("OCR worker has no Windows process attributes")
	}
	if !cmd.SysProcAttr.HideWindow {
		t.Fatal("OCR worker must set HideWindow")
	}
	if cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatal("OCR worker must set CREATE_NO_WINDOW")
	}
	args := strings.Join(cmd.Args, " ")
	if !strings.Contains(args, "-WindowStyle Hidden") {
		t.Fatalf("OCR worker must request hidden PowerShell window, args=%q", args)
	}
}
