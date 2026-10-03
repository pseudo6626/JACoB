//go:build windows

package platform

import (
	"syscall"
	"unsafe"
)

const (
	rimTypeMouse    = 0
	rimTypeKeyboard = 1
	rimTypeHID      = 2
)

type rawInputDeviceList struct {
	HDevice uintptr
	Type    uint32
}

var procGetRawInputDeviceList = user32.NewProc("GetRawInputDeviceList")

func probeInputDevices() InputDeviceReport {
	report := InputDeviceReport{Source: "windows-raw-input"}
	var count uint32
	sz := uint32(unsafe.Sizeof(rawInputDeviceList{}))
	r, _, callErr := procGetRawInputDeviceList.Call(0, uintptr(unsafe.Pointer(&count)), uintptr(sz))
	if r == ^uintptr(0) {
		report.Error = "GetRawInputDeviceList(count) failed: " + callErr.Error()
		return report
	}
	if count == 0 {
		return report
	}
	items := make([]rawInputDeviceList, count)
	r, _, callErr = procGetRawInputDeviceList.Call(uintptr(unsafe.Pointer(&items[0])), uintptr(unsafe.Pointer(&count)), uintptr(sz))
	if r == ^uintptr(0) {
		report.Error = "GetRawInputDeviceList(list) failed: " + callErr.Error()
		return report
	}
	for i := uint32(0); i < count && int(i) < len(items); i++ {
		switch items[i].Type {
		case rimTypeMouse:
			report.MouseCount++
		case rimTypeKeyboard:
			report.KeyboardCount++
		case rimTypeHID:
			report.HIDCount++
		}
	}
	return report
}

func gameRunning() bool {
	_, _, err := findEliteWindow()
	return err == nil
}

var _ = syscall.Errno(0)
