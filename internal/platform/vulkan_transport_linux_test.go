//go:build linux && cgo && !nocapture

package platform

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
)

func TestVulkanHUDTransportWritesTriangleStream(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("JACOB_VK_RUNTIME_DIR", dir)
	o := newVulkanOverlayDriver()
	if err := o.SetLayer("test", OverlayScene{Items: []OverlayItem{{
		Type: "rect", X: 0.1, Y: 0.2, W: 0.25, H: 0.1, Fill: "#FF6600CC",
	}}}); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "overlay.vertices"))
	if err != nil {
		t.Fatal(err)
	}
	if len(b) < 40 {
		t.Fatalf("transport too short: %d", len(b))
	}
	if got := binary.LittleEndian.Uint32(b[0:4]); got != vulkanHUDMagic {
		t.Fatalf("magic=%#x", got)
	}
	if got := binary.LittleEndian.Uint32(b[4:8]); got != vulkanHUDVersion {
		t.Fatalf("version=%d", got)
	}
	count := binary.LittleEndian.Uint32(b[20:24])
	if count < 6 || count%3 != 0 {
		t.Fatalf("unexpected vertex count %d", count)
	}
	if want := 40 + int(count)*24; len(b) != want {
		t.Fatalf("transport bytes=%d want=%d", len(b), want)
	}
}

func TestReadVulkanCaptureFrameConvertsBGRA(t *testing.T) {
	path := filepath.Join(t.TempDir(), "capture.frame")
	b := make([]byte, 32+8)
	binary.LittleEndian.PutUint32(b[0:4], vulkanCaptureMagic)
	binary.LittleEndian.PutUint32(b[4:8], vulkanCaptureVersion)
	binary.LittleEndian.PutUint64(b[8:16], 99)
	binary.LittleEndian.PutUint32(b[16:20], 2)
	binary.LittleEndian.PutUint32(b[20:24], 1)
	binary.LittleEndian.PutUint32(b[24:28], vulkanCaptureBGRA)
	binary.LittleEndian.PutUint32(b[28:32], 8)
	copy(b[32:], []byte{3, 2, 1, 255, 30, 20, 10, 128})
	if err := os.WriteFile(path, b, 0o600); err != nil {
		t.Fatal(err)
	}
	frame, seq, err := readVulkanCaptureFrame(path)
	if err != nil {
		t.Fatal(err)
	}
	if seq != 99 || frame.Width != 2 || frame.Height != 1 {
		t.Fatalf("seq/dimensions = %d %dx%d", seq, frame.Width, frame.Height)
	}
	want := []byte{1, 2, 3, 255, 10, 20, 30, 128}
	for i := range want {
		if frame.Pix[i] != want[i] {
			t.Fatalf("pixel[%d]=%d want=%d", i, frame.Pix[i], want[i])
		}
	}
}
