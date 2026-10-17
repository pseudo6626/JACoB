package core

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"html"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"jacob/internal/platform"

	qrcode "github.com/skip2/go-qrcode"
)

const (
	quickPairHUDLayer     = "__jacob:boot-pair"
	quickPairWindow       = 30 * time.Second
	quickPairReadyTimeout = 2 * time.Minute
)

type quickPairState struct {
	mu      sync.Mutex
	token   string
	expires time.Time
	used    bool
}

func (q *quickPairState) arm(token string, expires time.Time) {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.token = token
	q.expires = expires
	q.used = false
}

func (q *quickPairState) consume(token string, now time.Time) bool {
	q.mu.Lock()
	defer q.mu.Unlock()
	if q.used || q.token == "" || now.After(q.expires) || len(token) != len(q.token) {
		return false
	}
	if subtle.ConstantTimeCompare([]byte(token), []byte(q.token)) != 1 {
		return false
	}
	q.used = true
	q.token = ""
	return true
}

func (q *quickPairState) invalidate() {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.used = true
	q.token = ""
	q.expires = time.Time{}
}

func newQuickPairToken() (string, error) {
	b := make([]byte, 24)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

func (s *Server) startQuickPairHUD(ctx context.Context) {
	if !s.cfg.LANEnabled || s.devices == nil || s.overlay == nil {
		return
	}

	// JACoB can start before Elite has created its HUD surface, especially on
	// Steam Deck. Wait for a usable surface, then give the commander a full
	// 30-second scan window. Cap the wait so a standalone host never leaves a
	// dormant quick-pair goroutine/token around indefinitely.
	readyDeadline := time.Now().Add(quickPairReadyTimeout)
	var info platform.OverlayInfo
	for time.Now().Before(readyDeadline) {
		select {
		case <-ctx.Done():
			return
		default:
		}
		info = s.overlay.Info()
		if quickPairSurfaceReady(info) {
			break
		}
		t := time.NewTimer(250 * time.Millisecond)
		select {
		case <-ctx.Done():
			t.Stop()
			return
		case <-t.C:
		}
	}
	if !quickPairSurfaceReady(info) {
		return
	}

	addresses := localIPv4s()
	preferred := preferredLocalIPv4(addresses)
	port := portOf(s.cfg.Bind)
	if preferred == "" || port == "" {
		log.Printf("security/quickpair: startup QR skipped because no usable LAN address was found")
		return
	}
	token, err := newQuickPairToken()
	if err != nil {
		log.Printf("security/quickpair: startup QR skipped because a secure token could not be generated: %v", err)
		return
	}
	pairURL := fmt.Sprintf("http://%s:%s/q/%s", preferred, port, token)
	scene, err := quickPairScene(pairURL, info.Width, info.Height)
	if err != nil {
		log.Printf("security/quickpair: startup QR scene failed: %v", err)
		return
	}

	expires := time.Now().Add(quickPairWindow)
	s.quickPair.arm(token, expires)
	if err := s.overlay.SetLayer(quickPairHUDLayer, scene); err != nil {
		s.quickPair.invalidate()
		log.Printf("security/quickpair: startup QR could not be displayed: %v", err)
		return
	}
	log.Printf("security/quickpair: one-time startup QR active on %s for %s", preferred, quickPairWindow)

	t := time.NewTimer(time.Until(expires))
	defer t.Stop()
	select {
	case <-ctx.Done():
	case <-t.C:
	}
	s.quickPair.invalidate()
	_ = s.overlay.ClearLayer(quickPairHUDLayer)
}

func quickPairSurfaceReady(info platform.OverlayInfo) bool {
	if !info.Available || info.Width < 320 || info.Height < 240 {
		return false
	}
	// The Vulkan transport reports itself available before Elite has connected
	// the in-game renderer. Other current drivers only become available once
	// their usable surface exists.
	if strings.Contains(strings.ToLower(info.Driver), "vulkan") {
		return info.SurfaceCreated
	}
	return true
}

func (s *Server) handleQuickPair(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Referrer-Policy", "no-referrer")
	if r.Method != http.MethodGet {
		http.Error(w, "GET required", http.StatusMethodNotAllowed)
		return
	}
	if isLoopbackRequest(r) {
		http.Redirect(w, r, "/", http.StatusSeeOther)
		return
	}
	if !s.cfg.LANEnabled || s.devices == nil {
		quickPairError(w, http.StatusForbidden, "Quick pairing is unavailable", "LAN pairing is not enabled on this JACoB host.")
		return
	}
	token := strings.TrimSpace(strings.TrimPrefix(r.URL.Path, "/q/"))
	if token == "" || strings.Contains(token, "/") || !s.quickPair.consume(token, time.Now()) {
		quickPairError(w, http.StatusGone, "Quick-pair code expired", "This startup QR code is expired or has already been used. Use Settings → LAN access for normal pairing.")
		return
	}
	defer func() {
		if s.overlay != nil {
			_ = s.overlay.ClearLayer(quickPairHUDLayer)
		}
	}()

	if cookie, err := r.Cookie(deviceCookieName); err == nil && strings.TrimSpace(cookie.Value) != "" {
		if _, ok := s.devices.Authenticate(cookie.Value); ok {
			http.Redirect(w, r, "/?paired=qr", http.StatusSeeOther)
			return
		}
	}

	device, credential, err := s.devices.Enroll(quickPairDeviceName(r.UserAgent()))
	if err != nil {
		quickPairError(w, http.StatusInternalServerError, "Pairing failed", "JACoB could not create a device credential. Pair manually from Settings.")
		return
	}
	setDeviceCookie(w, r, credential, false)
	log.Printf("security/quickpair: enrolled device id=%q name=%q remote=%q", device.ID, device.Name, remoteAddressKey(r))
	s.broadcastDevicesChanged()
	http.Redirect(w, r, "/?paired=qr", http.StatusSeeOther)
}

func quickPairDeviceName(userAgent string) string {
	ua := strings.ToLower(userAgent)
	switch {
	case strings.Contains(ua, "iphone"):
		return "QR · iPhone"
	case strings.Contains(ua, "ipad"):
		return "QR · iPad"
	case strings.Contains(ua, "android"):
		return "QR · Android"
	case strings.Contains(ua, "windows"):
		return "QR · Windows"
	case strings.Contains(ua, "macintosh") || strings.Contains(ua, "mac os"):
		return "QR · Mac"
	case strings.Contains(ua, "linux"):
		return "QR · Linux"
	default:
		return "QR paired device"
	}
}

func quickPairError(w http.ResponseWriter, status int, title, message string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	_, _ = fmt.Fprintf(w, `<!doctype html><meta name="viewport" content="width=device-width,initial-scale=1"><title>%s</title><body style="font:16px system-ui;background:#090a0b;color:#eee;padding:32px;max-width:720px;margin:auto"><h1>%s</h1><p>%s</p><p><a style="color:#ff8a2b" href="/">Open JACoB</a></p></body>`, html.EscapeString(title), html.EscapeString(title), html.EscapeString(message))
}

func quickPairScene(pairURL string, width, height int) (platform.OverlayScene, error) {
	if width < 320 || height < 240 {
		return platform.OverlayScene{}, fmt.Errorf("overlay dimensions are too small: %dx%d", width, height)
	}
	code, err := qrcode.New(pairURL, qrcode.Medium)
	if err != nil {
		return platform.OverlayScene{}, err
	}
	bitmap := code.Bitmap()
	if len(bitmap) == 0 || len(bitmap[0]) == 0 {
		return platform.OverlayScene{}, fmt.Errorf("QR encoder returned an empty bitmap")
	}

	qrSize := float64(height) * 0.30
	if qrSize < 180 {
		qrSize = 180
	}
	if qrSize > 280 {
		qrSize = 280
	}
	cardW := qrSize + 36
	cardH := qrSize + 92
	cardX := float64(width) - cardW - 24
	cardY := 24.0
	if cardX < 12 {
		cardX = 12
	}
	qrX := cardX + 18
	qrY := cardY + 42
	module := qrSize / float64(len(bitmap))

	items := []platform.OverlayItem{
		{Type: "rect", X: cardX, Y: cardY, W: cardW, H: cardH, Fill: "#071016E8", Stroke: "#FF8A2BA0", LineWidth: 1.5, CornerRadius: 8},
		{Type: "text", X: cardX + cardW/2, Y: cardY + 14, Text: "SCAN TO PAIR JACoB", Align: "center", Color: "#FFFFFFFF", FontSize: 18, FontWeight: 600},
		{Type: "rect", X: qrX, Y: qrY, W: qrSize, H: qrSize, Fill: "#FFFFFFFF"},
		{Type: "text", X: cardX + cardW/2, Y: qrY + qrSize + 14, Text: "ONE-TIME LINK · EXPIRES IN 30 SEC", Align: "center", Color: "#C9D1D9FF", FontSize: 12, FontWeight: 500},
	}

	for y, row := range bitmap {
		for x := 0; x < len(row); {
			if !row[x] {
				x++
				continue
			}
			start := x
			for x < len(row) && row[x] {
				x++
			}
			items = append(items, platform.OverlayItem{
				Type: "rect",
				X:    qrX + float64(start)*module,
				Y:    qrY + float64(y)*module,
				W:    float64(x-start) * module,
				H:    module,
				Fill: "#000000FF",
			})
		}
	}
	if len(items) > 2000 {
		return platform.OverlayScene{}, fmt.Errorf("QR scene requires %d items", len(items))
	}

	scene := platform.OverlayScene{Z: 1000000, Space: "pixels", Width: width, Height: height, Theme: "clean", Items: items}
	if err := platform.ValidateOverlayScene(&scene); err != nil {
		return platform.OverlayScene{}, err
	}
	return scene, nil
}
