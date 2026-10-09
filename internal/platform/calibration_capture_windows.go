//go:build windows && !nocapture

package platform

// CaptureCalibrationJPEG is now only an encoding adapter. The platform capture
// backend itself owns temporary focus and returns the same canonical Elite
// client frame used by live Vision.
func CaptureCalibrationJPEG(c CaptureDriver, maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	frame, err := c.CaptureFrame(maxWidth, CaptureIntentCalibration)
	if err != nil {
		return nil, CaptureInfo{}, err
	}
	return EncodeGameFrameJPEG(frame, quality)
}
