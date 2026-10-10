//go:build !windows

package networkdiag

func Inspect(exe string, port int) Report {
	return Report{Supported: false}
}

func EnsureInteractive(exe string, port int) error { return nil }
func RemoveInteractive() error                     { return nil }
