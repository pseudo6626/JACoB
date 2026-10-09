//go:build windows && nocapture

package platform

import "testing"

func TestNoCaptureBuildHasNoScreenCaptureDriver(t *testing.T) {
	c := NewCaptureDriver()
	if c.Available() {
		t.Fatal("nocapture build must not expose a screen capture driver")
	}
	if c.Name() != "not-installed-no-capture-build" {
		t.Fatalf("unexpected nocapture driver name: %s", c.Name())
	}
	if _, _, err := c.CaptureJPEG(320, 72); err == nil {
		t.Fatal("nocapture driver must refuse CaptureJPEG")
	}
}
