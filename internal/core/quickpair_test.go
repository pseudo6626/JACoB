package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jacob/internal/deviceauth"
	"jacob/internal/platform"
)

func TestQuickPairStateIsSingleUseAndExpires(t *testing.T) {
	now := time.Unix(100, 0)
	var q quickPairState
	q.arm("secret", now.Add(30*time.Second))
	if q.consume("wrong", now) {
		t.Fatal("wrong token was accepted")
	}
	if !q.consume("secret", now) {
		t.Fatal("valid token was rejected")
	}
	if q.consume("secret", now) {
		t.Fatal("one-time token was accepted twice")
	}
	q.arm("later", now.Add(time.Second))
	if q.consume("later", now.Add(2*time.Second)) {
		t.Fatal("expired token was accepted")
	}
}

func TestQuickPairSurfaceReadyWaitsForVulkanRenderer(t *testing.T) {
	base := platform.OverlayInfo{Available: true, Width: 1280, Height: 800}
	win := base
	win.Driver = "windows-layered-gdiplus-hud2"
	if !quickPairSurfaceReady(win) {
		t.Fatal("usable Windows overlay should be ready")
	}
	vk := base
	vk.Driver = "linux-vulkan-hud2-v10"
	if quickPairSurfaceReady(vk) {
		t.Fatal("Vulkan transport without an Elite renderer should not start the timer")
	}
	vk.SurfaceCreated = true
	if !quickPairSurfaceReady(vk) {
		t.Fatal("connected Vulkan renderer should be ready")
	}
}

func TestQuickPairHandlerEnrollsOrdinaryDeviceAndConsumesToken(t *testing.T) {
	store, err := deviceauth.New("")
	if err != nil {
		t.Fatal(err)
	}
	s := &Server{
		cfg:           Config{LANEnabled: true},
		devices:       store,
		clients:       map[*wsClient]struct{}{},
		clientDevices: map[*wsClient]string{},
	}
	s.quickPair.arm("one-shot", time.Now().Add(30*time.Second))

	req := httptest.NewRequest(http.MethodGet, "http://192.168.1.10/q/one-shot", nil)
	req.RemoteAddr = "192.168.1.20:54321"
	req.Header.Set("User-Agent", "Mozilla/5.0 (Linux; Android 16)")
	rr := httptest.NewRecorder()
	s.handleQuickPair(rr, req)
	if rr.Code != http.StatusSeeOther || rr.Header().Get("Location") != "/?paired=qr" {
		t.Fatalf("unexpected quick-pair response: status=%d location=%q", rr.Code, rr.Header().Get("Location"))
	}
	var credential string
	for _, c := range rr.Result().Cookies() {
		if c.Name == deviceCookieName {
			credential = c.Value
			if c.MaxAge != 0 {
				t.Fatalf("quick-pair credential should not be remembered: maxAge=%d", c.MaxAge)
			}
		}
	}
	if credential == "" {
		t.Fatal("quick-pair response did not set a device credential")
	}
	device, ok := store.Authenticate(credential)
	if !ok {
		t.Fatal("quick-pair credential did not authenticate")
	}
	if device.Admin || device.Remember || device.Name != "QR · Android" {
		t.Fatalf("unexpected paired device: %+v", device)
	}

	req2 := httptest.NewRequest(http.MethodGet, "http://192.168.1.10/q/one-shot", nil)
	req2.RemoteAddr = "192.168.1.21:54322"
	rr2 := httptest.NewRecorder()
	s.handleQuickPair(rr2, req2)
	if rr2.Code != http.StatusGone {
		t.Fatalf("one-time token was reusable: status=%d", rr2.Code)
	}
}

func TestQuickPairSceneIsBoundedAndValid(t *testing.T) {
	scene, err := quickPairScene("http://192.168.50.219:4510/q/"+strings.Repeat("A", 32), 1280, 800)
	if err != nil {
		t.Fatal(err)
	}
	if scene.Space != "pixels" || scene.Width != 1280 || scene.Height != 800 {
		t.Fatalf("unexpected scene geometry: %+v", scene)
	}
	if len(scene.Items) < 100 || len(scene.Items) > 2000 {
		t.Fatalf("unexpected QR scene item count: %d", len(scene.Items))
	}
}

func TestQuickPairDeviceName(t *testing.T) {
	for ua, want := range map[string]string{
		"Mozilla/5.0 (Linux; Android 16)":        "QR · Android",
		"Mozilla/5.0 (iPhone; CPU iPhone OS 18)": "QR · iPhone",
		"unknown":                                "QR paired device",
	} {
		if got := quickPairDeviceName(ua); got != want {
			t.Fatalf("quickPairDeviceName(%q) = %q, want %q", ua, got, want)
		}
	}
}
