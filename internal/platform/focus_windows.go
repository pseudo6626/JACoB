//go:build windows

package platform

import (
	"fmt"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"
)

const (
	processQueryLimitedInformation = 0x1000
	swRestore                      = 9
)

type eliteWindowSearch struct {
	found    uintptr
	foundExe string
}

var (
	kernel32                       = syscall.NewLazyDLL("kernel32.dll")
	procEnumWindows                = user32.NewProc("EnumWindows")
	procIsWindowVisible            = user32.NewProc("IsWindowVisible")
	procIsWindow                   = user32.NewProc("IsWindow")
	procGetForegroundWindow        = user32.NewProc("GetForegroundWindow")
	procSetForegroundWindow        = user32.NewProc("SetForegroundWindow")
	procBringWindowToTop           = user32.NewProc("BringWindowToTop")
	procShowWindow                 = user32.NewProc("ShowWindow")
	procGetWindowThreadProcessId   = user32.NewProc("GetWindowThreadProcessId")
	procAttachThreadInput          = user32.NewProc("AttachThreadInput")
	procGetCurrentThreadID         = kernel32.NewProc("GetCurrentThreadId")
	procOpenProcess                = kernel32.NewProc("OpenProcess")
	procQueryFullProcessImageNameW = kernel32.NewProc("QueryFullProcessImageNameW")
	procCloseHandle                = kernel32.NewProc("CloseHandle")

	enumEliteWindowCallback = syscall.NewCallback(enumEliteWindowProc)
	findEliteWindowMu       sync.Mutex
	findEliteWindowState    eliteWindowSearch
)

func enumEliteWindowProc(hwnd uintptr, lparam uintptr) uintptr {
	_ = lparam
	search := &findEliteWindowState
	visible, _, _ := procIsWindowVisible.Call(hwnd)
	if visible == 0 {
		return 1
	}
	var pid uint32
	procGetWindowThreadProcessId.Call(hwnd, uintptr(unsafe.Pointer(&pid)))
	if pid == 0 {
		return 1
	}
	exe, err := processBaseName(pid)
	if err != nil {
		return 1
	}
	if isEliteExecutable(exe) {
		search.found = hwnd
		search.foundExe = exe
		return 0
	}
	return 1
}

func (w *windowsInput) FocusGame() error {
	hwnd, exe, err := findEliteWindow()
	if err != nil {
		return err
	}
	if hwnd == 0 {
		return fmt.Errorf("Elite Dangerous window was not found; make sure the game is running")
	}

	fg, _, _ := procGetForegroundWindow.Call()
	if fg == hwnd {
		return nil
	}

	runtime.LockOSThread()
	defer runtime.UnlockOSThread()

	currentThread, _, _ := procGetCurrentThreadID.Call()
	fgThread := windowThreadID(fg)
	targetThread := windowThreadID(hwnd)

	attachedFG := false
	attachedTarget := false
	if fgThread != 0 && fgThread != currentThread {
		if r, _, _ := procAttachThreadInput.Call(currentThread, fgThread, 1); r != 0 {
			attachedFG = true
		}
	}
	if targetThread != 0 && targetThread != currentThread && targetThread != fgThread {
		if r, _, _ := procAttachThreadInput.Call(currentThread, targetThread, 1); r != 0 {
			attachedTarget = true
		}
	}

	defer func() {
		if attachedTarget {
			procAttachThreadInput.Call(currentThread, targetThread, 0)
		}
		if attachedFG {
			procAttachThreadInput.Call(currentThread, fgThread, 0)
		}
	}()

	procShowWindow.Call(hwnd, swRestore)
	procBringWindowToTop.Call(hwnd)
	procSetForegroundWindow.Call(hwnd)

	deadline := time.Now().Add(750 * time.Millisecond)
	for time.Now().Before(deadline) {
		now, _, _ := procGetForegroundWindow.Call()
		if now == hwnd {
			time.Sleep(120 * time.Millisecond)
			return nil
		}
		time.Sleep(15 * time.Millisecond)
	}

	now, _, _ := procGetForegroundWindow.Call()
	return fmt.Errorf("found %s but Windows would not make its window foreground (target=0x%X foreground=0x%X)", exe, hwnd, now)
}

func findEliteWindow() (uintptr, string, error) {
	findEliteWindowMu.Lock()
	defer findEliteWindowMu.Unlock()
	findEliteWindowState = eliteWindowSearch{}
	search := &findEliteWindowState
	r, _, callErr := procEnumWindows.Call(enumEliteWindowCallback, 0)
	if search.found == 0 && r == 0 && callErr != syscall.Errno(0) {
		return 0, "", fmt.Errorf("EnumWindows failed: %w", callErr)
	}
	if search.found == 0 {
		return 0, "", fmt.Errorf("Elite Dangerous window was not found; expected EliteDangerous64.exe or EliteDangerous.exe")
	}
	return search.found, search.foundExe, nil
}

func processBaseName(pid uint32) (string, error) {
	h, _, callErr := procOpenProcess.Call(processQueryLimitedInformation, 0, uintptr(pid))
	if h == 0 {
		if callErr != syscall.Errno(0) {
			return "", callErr
		}
		return "", fmt.Errorf("OpenProcess failed for pid %d", pid)
	}
	defer procCloseHandle.Call(h)

	buf := make([]uint16, 2048)
	size := uint32(len(buf))
	r, _, callErr := procQueryFullProcessImageNameW.Call(h, 0, uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)))
	if r == 0 {
		if callErr != syscall.Errno(0) {
			return "", callErr
		}
		return "", fmt.Errorf("QueryFullProcessImageNameW failed for pid %d", pid)
	}
	full := syscall.UTF16ToString(buf[:size])
	if i := strings.LastIndexAny(full, `\\/`); i >= 0 {
		full = full[i+1:]
	}
	return full, nil
}

func isEliteExecutable(name string) bool {
	return strings.EqualFold(name, "EliteDangerous64.exe") || strings.EqualFold(name, "EliteDangerous.exe")
}

func windowThreadID(hwnd uintptr) uintptr {
	if hwnd == 0 {
		return 0
	}
	tid, _, _ := procGetWindowThreadProcessId.Call(hwnd, 0)
	return tid
}
