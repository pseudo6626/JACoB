package platform

type CaptureInfo struct {
	Driver       string `json:"driver"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	SourceWidth  int    `json:"sourceWidth"`
	SourceHeight int    `json:"sourceHeight"`
}

type CaptureDriver interface {
	Name() string
	Available() bool
	CaptureJPEG(maxWidth int, quality int) ([]byte, CaptureInfo, error)
}

func NewCaptureDriver() CaptureDriver { return newCaptureDriver() }
