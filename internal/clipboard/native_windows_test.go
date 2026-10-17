//go:build windows

package clipboard

import (
	"os"
	"runtime"
	"syscall"
	"testing"
	"unsafe"
)

// Explicit opt-in: this replaces the real clipboard. Ordinary go test runs
// never access the user's clipboard. Reading exists only in this test file.
func TestNativeWindowsClipboard(t *testing.T) {
	if os.Getenv("JACOB_TEST_NATIVE_CLIPBOARD") != "1" {
		t.Skip("opt-in native clipboard test replaces the current clipboard")
	}
	foreground := user32.NewProc("GetForegroundWindow")
	readData := user32.NewProc("GetClipboardData")
	before, _, _ := foreground.Call()
	for _, text := range []string{"Зоря 🌟\r\nline two", "", "Phroea Phio UK-W c18-0"} {
		result, err := New().WriteText(text)
		if err != nil || !result.Written {
			t.Fatalf("native write failed: %+v, %v", result, err)
		}
		// Read after WriteText has destroyed its temporary owner window; the OS
		// must still retain the non-delayed CF_UNICODETEXT allocation.
		runtime.LockOSThread()
		ok, _, err := openClipboard.Call(0)
		if ok == 0 {
			runtime.UnlockOSThread()
			t.Fatalf("open for read: %v", err)
		}
		h, _, _ := readData.Call(13)
		ptr, _, _ := globalLock.Call(h)
		if h == 0 || ptr == 0 {
			closeClipboard.Call()
			runtime.UnlockOSThread()
			t.Fatal("Unicode data missing after owner closed")
		}
		units, _ := syscall.UTF16FromString(text)
		got := syscall.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(units)))
		globalUnlock.Call(h)
		closeClipboard.Call()
		runtime.UnlockOSThread()
		if got != text {
			t.Fatalf("Unicode round-trip mismatch")
		}
	}
	after, _, _ := foreground.Call()
	if before != after {
		t.Fatal("foreground window changed during clipboard test")
	}
}
