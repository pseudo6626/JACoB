//go:build nocapture

package platform

import "fmt"

func CaptureCalibrationJPEG(c CaptureDriver, maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	_ = c
	_ = maxWidth
	_ = quality
	return nil, CaptureInfo{Driver: "not-installed-no-capture-build", Blanked: true, Reason: "screen capture was not installed"}, fmt.Errorf("screen capture was not installed in this JACoB build")
}
