//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"strings"
)

func probeInputDevices() InputDeviceReport {
	report := InputDeviceReport{Source: "linux-proc-input"}
	b, err := os.ReadFile("/proc/bus/input/devices")
	if err != nil {
		report.Error = err.Error()
		return report
	}
	for _, block := range strings.Split(string(b), "\n\n") {
		low := strings.ToLower(block)
		if strings.Contains(low, "handlers=") && strings.Contains(low, "kbd") {
			report.KeyboardCount++
		}
		if strings.Contains(low, "handlers=") && strings.Contains(low, "mouse") {
			report.MouseCount++
		}
		if strings.Contains(low, "handlers=") && (strings.Contains(low, "event") || strings.Contains(low, "js")) {
			report.HIDCount++
		}
	}
	return report
}

func gameRunning() bool {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if name == "" || name[0] < '0' || name[0] > '9' {
			continue
		}
		for _, f := range []string{"comm", "cmdline"} {
			b, err := os.ReadFile(filepath.Join("/proc", name, f))
			if err != nil {
				continue
			}
			s := strings.ToLower(string(b))
			if strings.Contains(s, "elitedangerous64.exe") || strings.Contains(s, "elitedangerous.exe") {
				return true
			}
		}
	}
	return false
}
