package webfetch

import (
	"net"
	"net/url"
	"testing"
)

func TestRejectsLocalTargets(t *testing.T) {
	for _, raw := range []string{"http://localhost/x", "http://127.0.0.1/x", "http://192.168.1.10/x", "http://host.local/x", "http://intranet/x", "https://example.com:444/x"} {
		u, _ := url.Parse(raw)
		if err := validateURL(u); err == nil {
			t.Fatalf("expected %s to be rejected", raw)
		}
	}
}

func TestAllowsPublicHTTPS(t *testing.T) {
	u, _ := url.Parse("https://spansh.co.uk/api/system/123")
	if err := validateURL(u); err != nil {
		t.Fatal(err)
	}
	if unsafeIP(net.ParseIP("8.8.8.8")) {
		t.Fatal("public IP marked unsafe")
	}
}
