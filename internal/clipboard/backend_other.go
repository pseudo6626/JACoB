//go:build !windows && !linux

package clipboard

func nativeBackend() backend {
	return backend{name: "unsupported", reason: "native clipboard writing is supported on Windows and Linux"}
}
