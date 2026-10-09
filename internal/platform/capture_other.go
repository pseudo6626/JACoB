//go:build !windows && !linux && !nocapture

package platform

import "fmt"

type unsupportedCapture struct{}

func newCaptureDriver() CaptureDriver         { return &unsupportedCapture{} }
func (u *unsupportedCapture) Name() string    { return "window-capture-unsupported" }
func (u *unsupportedCapture) Available() bool { return false }
func (u *unsupportedCapture) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	_ = maxWidth
	_ = intent
	return GameFrame{}, fmt.Errorf("safe Elite-window capture is unsupported on this platform")
}
func (u *unsupportedCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	_ = maxWidth
	_ = quality
	return nil, CaptureInfo{Driver: u.Name(), Blanked: true, Reason: "safe Elite-window capture unsupported"}, fmt.Errorf("safe Elite-window capture is unsupported")
}
