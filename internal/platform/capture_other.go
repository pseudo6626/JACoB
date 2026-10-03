//go:build !windows && !linux

package platform

import "fmt"

type unsupportedCapture struct{}

func newCaptureDriver() CaptureDriver         { return &unsupportedCapture{} }
func (u *unsupportedCapture) Name() string    { return "unsupported" }
func (u *unsupportedCapture) Available() bool { return false }
func (u *unsupportedCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	return nil, CaptureInfo{}, fmt.Errorf("capture unsupported")
}
