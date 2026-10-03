//go:build !windows && !linux

package platform

func probeInputDevices() InputDeviceReport { return InputDeviceReport{Source: "unsupported"} }
func gameRunning() bool                    { return false }
