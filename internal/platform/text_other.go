//go:build !windows && !linux

package platform

import "time"

func typeTextNative(d InputDriver, text string, interval time.Duration) (bool, error) {
	return false, nil
}
