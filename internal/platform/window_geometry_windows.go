//go:build windows

package platform

type point struct{ X, Y int32 }
type rect struct{ Left, Top, Right, Bottom int32 }

var (
	procGetClientRect  = user32.NewProc("GetClientRect")
	procClientToScreen = user32.NewProc("ClientToScreen")
)
