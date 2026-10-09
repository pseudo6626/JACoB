//go:build linux && !nocapture

package platform

import "fmt"

type linuxCapture struct{}

func newCaptureDriver() CaptureDriver   { return &linuxCapture{} }
func (l *linuxCapture) Name() string    { return "linux-window-backend-pending" }
func (l *linuxCapture) Available() bool { return false }
func (l *linuxCapture) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	_ = maxWidth
	_ = intent
	return GameFrame{}, fmt.Errorf("safe Elite-window capture is not enabled in the JACoB Linux build yet; the backend slot is reserved for XDG Desktop Portal/PipeWire window capture")
}
func (l *linuxCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	_ = maxWidth
	_ = quality
	return nil, CaptureInfo{Driver: l.Name(), Blanked: true, Reason: "safe Elite-window capture unavailable"}, fmt.Errorf("safe Elite-window capture is unavailable")
}
