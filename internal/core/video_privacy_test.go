package core

import (
	"net/http/httptest"
	"testing"

	"jacob/internal/platform"
)

type privacyMatteCaptureStub struct {
	tightCalls int
	matteCalls int
}

func (s *privacyMatteCaptureStub) Name() string    { return "privacy-matte-test" }
func (s *privacyMatteCaptureStub) Available() bool { return true }
func (s *privacyMatteCaptureStub) CaptureFrame(maxWidth int, intent platform.CaptureIntent) (platform.GameFrame, error) {
	return platform.GameFrame{Width: maxWidth, Height: 90, Stride: maxWidth * 4, Format: platform.PixelFormatRGBA8, Verified: true, Foreground: true, Driver: s.Name(), Pix: make([]byte, maxWidth*90*4)}, nil
}
func (s *privacyMatteCaptureStub) CaptureJPEG(maxWidth int, quality int) ([]byte, platform.CaptureInfo, error) {
	s.tightCalls++
	return []byte("tight"), platform.CaptureInfo{Driver: "tight", Canonical: true}, nil
}
func (s *privacyMatteCaptureStub) CapturePrivacyMatteJPEG(maxWidth int, quality int) ([]byte, platform.CaptureInfo, error) {
	s.matteCalls++
	return []byte("matte"), platform.CaptureInfo{Driver: "matte", Canonical: true}, nil
}

func TestVideoParamsPrivacyMatte(t *testing.T) {
	r := httptest.NewRequest("GET", "/api/video.mjpeg?width=640&quality=75&fps=10&matte=1", nil)
	width, quality, fps, matte := videoParams(r)
	if width != 640 || quality != 75 || fps != 10 || !matte {
		t.Fatalf("unexpected video params: width=%d quality=%d fps=%d matte=%v", width, quality, fps, matte)
	}

	r = httptest.NewRequest("GET", "/api/video.mjpeg", nil)
	_, _, _, matte = videoParams(r)
	if matte {
		t.Fatal("privacy matte must remain opt-in at the HTTP/SDK level")
	}
}

func TestCaptureVideoJPEGRoutesMatteSeparately(t *testing.T) {
	stub := &privacyMatteCaptureStub{}
	b, _, err := captureVideoJPEG(stub, 960, 60, false)
	if err != nil || string(b) != "tight" {
		t.Fatalf("tight capture failed: %q %v", string(b), err)
	}
	if stub.tightCalls != 1 || stub.matteCalls != 0 {
		t.Fatalf("unexpected non-matte routing: tight=%d matte=%d", stub.tightCalls, stub.matteCalls)
	}

	b, _, err = captureVideoJPEG(stub, 960, 60, true)
	if err != nil || string(b) != "matte" {
		t.Fatalf("matte capture failed: %q %v", string(b), err)
	}
	if stub.tightCalls != 1 || stub.matteCalls != 1 {
		t.Fatalf("unexpected matte routing: tight=%d matte=%d", stub.tightCalls, stub.matteCalls)
	}
}
