package platform

import (
	"strconv"
	"strings"
)

func parseXWindowID(s string) string {
	for _, field := range strings.Fields(s) {
		f := strings.Trim(field, " ,\t\r\n")
		if strings.HasPrefix(strings.ToLower(f), "0x") {
			if _, err := strconv.ParseUint(strings.TrimPrefix(strings.ToLower(f), "0x"), 16, 64); err == nil {
				return f
			}
		}
	}
	return ""
}

func parseXPropPID(s string) int {
	for _, line := range strings.Split(s, "\n") {
		if !strings.Contains(line, "_NET_WM_PID") {
			continue
		}
		i := strings.LastIndex(line, "=")
		if i < 0 {
			continue
		}
		if pid, err := strconv.Atoi(strings.TrimSpace(line[i+1:])); err == nil {
			return pid
		}
	}
	return 0
}
