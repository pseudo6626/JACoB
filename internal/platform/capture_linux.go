//go:build linux && !nocapture && !cgo

package platform

import "fmt"

type linuxCapture struct{}

func newCaptureDriver() CaptureDriver   { return &linuxCapture{} }
func (l *linuxCapture) Name() string    { return "linux-window-capture-requires-cgo-x11" }
func (l *linuxCapture) Available() bool { return false }
func (l *linuxCapture) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	_ = maxWidth
	_ = intent
	return GameFrame{}, fmt.Errorf("Steam Deck Vision requires the CGO/X11 build")
}
func (l *linuxCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	_ = maxWidth
	_ = quality
	return nil, CaptureInfo{Driver: l.Name(), Blanked: true, Reason: "Steam Deck Vision requires CGO/X11"}, fmt.Errorf("Steam Deck Vision requires the CGO/X11 build")
}
