//go:build nocapture

package platform

import "fmt"

type disabledCapture struct{}

func newCaptureDriver() CaptureDriver      { return &disabledCapture{} }
func (d *disabledCapture) Name() string    { return "not-installed-no-capture-build" }
func (d *disabledCapture) Available() bool { return false }
func (d *disabledCapture) CaptureFrame(maxWidth int, intent CaptureIntent) (GameFrame, error) {
	_ = maxWidth
	_ = intent
	return GameFrame{}, fmt.Errorf("screen capture was not installed in this JACoB build")
}
func (d *disabledCapture) CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	_ = maxWidth
	_ = quality
	return nil, CaptureInfo{Driver: d.Name(), Blanked: true, Reason: "screen capture was not installed"}, fmt.Errorf("screen capture was not installed in this JACoB build")
}
