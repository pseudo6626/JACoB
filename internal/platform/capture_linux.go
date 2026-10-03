//go:build linux

package platform

import "fmt"

type linuxCapture struct{}

func newCaptureDriver() CaptureDriver   { return &linuxCapture{} }
func (l *linuxCapture) Name() string    { return "linux-pipewire-pending" }
func (l *linuxCapture) Available() bool { return false }
func (l *linuxCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	return nil, CaptureInfo{}, fmt.Errorf("game capture is not enabled in the JACoB Linux build yet; Steam Deck PipeWire/Gamescope capture is the next capture adapter")
}
