package burrow

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestExplicitProxyTrust(t *testing.T) {
	b := &Server{Store: &Store{Config: Config{TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}}}
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "203.0.113.1:5000"
	r.Header.Set("X-Forwarded-For", "1.2.3.4")
	if b.clientIP(r) != "203.0.113.1" {
		t.Fatal("untrusted peer spoofed client address")
	}
	r.RemoteAddr = "127.0.0.1:5000"
	r.Header.Set("X-Forwarded-For", "1.2.3.4, 198.51.100.4")
	if b.clientIP(r) != "198.51.100.4" {
		t.Fatal("trusted proxy did not pick rightmost untrusted address")
	}
}
