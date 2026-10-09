package platform

import (
	"os"
	"strings"
	"testing"
)

func TestWindowsCaptureSourceNeverUsesDesktopPixels(t *testing.T) {
	paths := []string{"capture_windows.go", "wgc_capture_windows.go"}
	var all strings.Builder
	for _, path := range paths {
		b, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		all.Write(b)
		all.WriteByte('\n')
	}
	src := all.String()

	if !strings.Contains(src, "CreateForWindow") {
		t.Fatal("Windows capture must create a WGC source for the Elite HWND")
	}
	for _, forbidden := range []string{
		"CreateForMonitor",
		"MonitorFromWindow",
		"GetDesktopWindow",
		`GetDC("`,
		"GetDC.Call(0",
		"BitBlt",
		"StretchBlt",
		"DuplicateOutput",
	} {
		if strings.Contains(src, forbidden) {
			t.Fatalf("Windows canonical capture contains forbidden desktop/monitor capture path %q", forbidden)
		}
	}
}

func TestWindowsWGCClientCoordinatesAreWindowRelative(t *testing.T) {
	b, err := os.ReadFile("wgc_capture_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	src := string(b)
	for _, required := range []string{
		"GetClientRect",
		"ClientToScreen",
		"GetWindowRect",
		"wgcClientCrop",
	} {
		if !strings.Contains(src, required) {
			t.Fatalf("WGC client crop is missing %q", required)
		}
	}
}
