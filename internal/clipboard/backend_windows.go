//go:build windows

package clipboard

import (
	"context"
	"fmt"
	"runtime"
	"syscall"
	"time"
	"unsafe"
)

var (
	user32           = syscall.NewLazyDLL("user32.dll")
	kernel32         = syscall.NewLazyDLL("kernel32.dll")
	createWindow     = user32.NewProc("CreateWindowExW")
	destroyWindow    = user32.NewProc("DestroyWindow")
	openClipboard    = user32.NewProc("OpenClipboard")
	closeClipboard   = user32.NewProc("CloseClipboard")
	emptyClipboard   = user32.NewProc("EmptyClipboard")
	setClipboardData = user32.NewProc("SetClipboardData")
	globalAlloc      = kernel32.NewProc("GlobalAlloc")
	globalLock       = kernel32.NewProc("GlobalLock")
	globalUnlock     = kernel32.NewProc("GlobalUnlock")
	globalFree       = kernel32.NewProc("GlobalFree")
)

func nativeBackend() backend { return backend{name: "win32", write: writeWindows} }

func writeWindows(ctx context.Context, text string) error {
	// The temporary message-only window is created and destroyed on one OS thread.
	// OpenClipboard(NULL) followed by EmptyClipboard leaves no valid data owner.
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	class, _ := syscall.UTF16PtrFromString("STATIC")
	const hwndMessage = ^uintptr(2) // HWND_MESSAGE (-3), never visible or activated.
	hwnd, _, err := createWindow.Call(0, uintptr(unsafe.Pointer(class)), 0, 0,
		0, 0, 0, 0, hwndMessage, 0, 0, 0)
	if hwnd == 0 {
		return fmt.Errorf("create clipboard owner: %w", err)
	}
	defer destroyWindow.Call(hwnd)

	units, err := syscall.UTF16FromString(text)
	if err != nil {
		return err
	}
	// Prepare memory before emptying the clipboard. Windows takes ownership only
	// after SetClipboardData succeeds; every failure path frees our allocation.
	handle, _, err := globalAlloc.Call(0x0042, uintptr(len(units)*2)) // MOVEABLE | ZEROINIT
	if handle == 0 {
		return fmt.Errorf("allocate clipboard text: %w", err)
	}
	transferred := false
	defer func() {
		if !transferred {
			globalFree.Call(handle)
		}
	}()
	ptr, _, err := globalLock.Call(handle)
	if ptr == 0 {
		return fmt.Errorf("lock clipboard text: %w", err)
	}
	copy(unsafe.Slice((*uint16)(unsafe.Pointer(ptr)), len(units)), units)
	globalUnlock.Call(handle)

	deadline := time.Now().Add(250 * time.Millisecond)
	for {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if ok, _, _ := openClipboard.Call(hwnd); ok != 0 {
			break
		}
		if time.Now().After(deadline) {
			return &Error{"CLIPBOARD_BUSY", "the Windows clipboard is in use by another application"}
		}
		time.Sleep(10 * time.Millisecond)
	}
	defer closeClipboard.Call()
	if ok, _, err := emptyClipboard.Call(); ok == 0 {
		return fmt.Errorf("empty clipboard: %w", err)
	}
	if value, _, err := setClipboardData.Call(13, handle); value == 0 { // CF_UNICODETEXT
		return fmt.Errorf("set clipboard text: %w", err)
	}
	transferred = true
	return nil
}
