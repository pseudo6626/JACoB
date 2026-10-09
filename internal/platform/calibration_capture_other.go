//go:build !windows && !nocapture

package platform

func CaptureCalibrationJPEG(c CaptureDriver, maxWidth int, quality int) ([]byte, CaptureInfo, error) {
	frame, err := c.CaptureFrame(maxWidth, CaptureIntentCalibration)
	if err != nil {
		return nil, CaptureInfo{}, err
	}
	return EncodeGameFrameJPEG(frame, quality)
}
