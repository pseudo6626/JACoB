package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestValidateWebSocketOriginAllowsSameHost(t *testing.T) {
	r := httptest.NewRequest("GET", "http://127.0.0.1:6626/ws", nil)
	r.Host = "127.0.0.1:6626"
	r.Header.Set("Origin", "http://127.0.0.1:6626")
	if err := validateWebSocketOrigin(r); err != nil {
		t.Fatal(err)
	}
}

func TestValidateWebSocketOriginRejectsCrossSite(t *testing.T) {
	r := httptest.NewRequest("GET", "http://127.0.0.1:6626/ws", nil)
	r.Host = "127.0.0.1:6626"
	r.Header.Set("Origin", "https://evil.example")
	if err := validateWebSocketOrigin(r); err == nil || !strings.Contains(err.Error(), "cross-origin") {
		t.Fatalf("expected cross-origin rejection, got %v", err)
	}
}

func TestValidateWebSocketOriginRejectsSandboxNullOrigin(t *testing.T) {
	r := httptest.NewRequest("GET", "http://127.0.0.1:6626/ws", nil)
	r.Host = "127.0.0.1:6626"
	r.Header.Set("Origin", "null")
	if err := validateWebSocketOrigin(r); err == nil {
		t.Fatal("expected null Origin to be rejected")
	}
}

func TestSecureRandomTokenStrength(t *testing.T) {
	token, err := secureRandomToken()
	if err != nil {
		t.Fatal(err)
	}
	if len(token) != 32 {
		t.Fatalf("token length=%d, want 32 hex chars", len(token))
	}
}

func TestValidJACoBHostRejectsDNSRebindingName(t *testing.T) {
	if validJACoBHost("attacker.example:6626") {
		t.Fatal("attacker-controlled DNS hostname must not be accepted as a JACoB host")
	}
	if !validJACoBHost("127.0.0.1:6626") {
		t.Fatal("loopback literal should be accepted")
	}
	if !validJACoBHost("localhost:6626") {
		t.Fatal("localhost should be accepted")
	}
}

func TestWebSocketOriginRejectsDNSRebindingHost(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "http://attacker.example:6626/ws", nil)
	r.Host = "attacker.example:6626"
	r.Header.Set("Origin", "http://attacker.example:6626")
	if err := validateWebSocketOrigin(r); err == nil {
		t.Fatal("same-origin attacker hostname must be rejected even if DNS could point it at loopback")
	}
}

func TestAuthorizeMediaRequiresScopedMediaToken(t *testing.T) {
	s := &Server{mediaToken: "0123456789ABCDEF0123456789ABCDEF", cfg: Config{LANEnabled: true, PairToken: "PAIRING-TOKEN-THAT-MUST-NOT-WORK"}}
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6626/api/video/frame.jpg", nil)
	r.RemoteAddr = "127.0.0.1:55000"
	if s.authorizeMedia(r) {
		t.Fatal("local media request without token must be rejected")
	}
	r = httptest.NewRequest(http.MethodGet, "http://127.0.0.1:6626/api/video/frame.jpg?token=0123456789ABCDEF0123456789ABCDEF", nil)
	r.RemoteAddr = "127.0.0.1:55000"
	if !s.authorizeMedia(r) {
		t.Fatal("local media request with correct per-process token should be accepted")
	}
	r = httptest.NewRequest(http.MethodGet, "http://192.168.1.5:6626/api/video/frame.jpg?token=PAIRING-TOKEN-THAT-MUST-NOT-WORK", nil)
	r.RemoteAddr = "192.168.1.25:55000"
	if s.authorizeMedia(r) {
		t.Fatal("LAN pairing token must not authorize media endpoints")
	}
	r = httptest.NewRequest(http.MethodGet, "http://192.168.1.5:6626/api/video/frame.jpg?token=0123456789ABCDEF0123456789ABCDEF", nil)
	r.RemoteAddr = "192.168.1.25:55000"
	if !s.authorizeMedia(r) {
		t.Fatal("scoped media token should authorize paired-client media requests")
	}
}
