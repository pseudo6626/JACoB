//go:build linux && !cgo

package platform

func linuxNativeFocusAvailable() bool         { return false }
func linuxNativeEliteFocus() (bool, bool)     { return false, false }
func linuxNativeEliteWindow() (uintptr, bool) { return 0, false }
