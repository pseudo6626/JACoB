package vision

import (
	"errors"
	"sync"
	"time"

	"jacob/internal/platform"
)

// Calibration owns a single frozen, verified Elite frame for the host-owned
// calibration UI. On Windows the platform layer briefly focuses Elite, takes
// the snapshot, then restores the previously focused window. The captured
// image is available only to JACoB's host UI; custom tabs receive normalized
// region/color results and never receive frame bytes.
type Calibration struct {
	mu      sync.RWMutex
	capture platform.CaptureDriver
	armed   bool
	ready   bool
	started time.Time
	at      time.Time
	expires time.Time
	reason  string
	frame   []byte
	info    platform.CaptureInfo
	cancel  chan struct{}
}

func NewCalibration(c platform.CaptureDriver) *Calibration {
	return &Calibration{capture: c}
}

func (c *Calibration) Arm(timeout time.Duration) error {
	if c == nil || c.capture == nil || !c.capture.Available() {
		return errors.New("screen capture is unavailable in this build")
	}
	if timeout <= 0 {
		timeout = 8 * time.Second
	}
	if timeout > 30*time.Second {
		timeout = 30 * time.Second
	}
	c.mu.Lock()
	if c.cancel != nil {
		close(c.cancel)
	}
	cancel := make(chan struct{})
	c.cancel = cancel
	c.armed = true
	c.ready = false
	c.started = time.Now().UTC()
	c.at = time.Time{}
	c.expires = time.Now().Add(timeout)
	c.reason = "capturing-elite"
	c.frame = nil
	c.info = platform.CaptureInfo{}
	c.mu.Unlock()

	go c.captureOnce(cancel, timeout)
	return nil
}

type calibrationCaptureResult struct {
	frame []byte
	info  platform.CaptureInfo
	err   error
}

func (c *Calibration) captureOnce(cancel <-chan struct{}, timeout time.Duration) {
	result := make(chan calibrationCaptureResult, 1)
	go func() {
		frame, info, err := platform.CaptureCalibrationJPEG(c.capture, 1600, 92)
		result <- calibrationCaptureResult{frame: frame, info: info, err: err}
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()
	select {
	case <-cancel:
		return
	case <-timer.C:
		c.mu.Lock()
		if c.cancel == cancel {
			c.armed = false
			c.reason = "timed-out"
		}
		c.mu.Unlock()
		return
	case got := <-result:
		c.mu.Lock()
		defer c.mu.Unlock()
		if c.cancel != cancel {
			return
		}
		c.armed = false
		if got.err != nil {
			c.reason = got.err.Error()
			return
		}
		if got.info.Blanked {
			c.reason = got.info.Reason
			return
		}
		c.ready = true
		c.at = time.Now().UTC()
		c.expires = time.Now().Add(5 * time.Minute)
		c.reason = ""
		c.frame = append(c.frame[:0], got.frame...)
		c.info = got.info
	}
}

func (c *Calibration) Status() map[string]any {
	if c == nil {
		return map[string]any{"available": false, "armed": false, "ready": false}
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.ready && !c.expires.IsZero() && time.Now().After(c.expires) {
		c.ready = false
		c.frame = nil
		c.reason = "expired"
	}
	out := map[string]any{
		"available": c.capture != nil && c.capture.Available(),
		"armed":     c.armed,
		"ready":     c.ready,
		"reason":    c.reason,
	}
	if !c.started.IsZero() {
		out["startedAt"] = c.started.Format(time.RFC3339Nano)
	}
	if !c.at.IsZero() {
		out["capturedAt"] = c.at.Format(time.RFC3339Nano)
		out["width"] = c.info.Width
		out["height"] = c.info.Height
		out["sourceWidth"] = c.info.SourceWidth
		out["sourceHeight"] = c.info.SourceHeight
	}
	return out
}

func (c *Calibration) Frame() ([]byte, platform.CaptureInfo, bool) {
	if c == nil {
		return nil, platform.CaptureInfo{}, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if !c.ready || len(c.frame) == 0 {
		return nil, platform.CaptureInfo{}, false
	}
	if !c.expires.IsZero() && time.Now().After(c.expires) {
		c.ready = false
		c.frame = nil
		c.reason = "expired"
		return nil, platform.CaptureInfo{}, false
	}
	return append([]byte(nil), c.frame...), c.info, true
}

func (c *Calibration) Clear() {
	if c == nil {
		return
	}
	c.mu.Lock()
	if c.cancel != nil {
		close(c.cancel)
		c.cancel = nil
	}
	c.armed = false
	c.ready = false
	c.reason = ""
	c.frame = nil
	c.info = platform.CaptureInfo{}
	c.mu.Unlock()
}
