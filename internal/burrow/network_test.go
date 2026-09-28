package burrow

import (
	"net"
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestExplicitNetworkTrust(t *testing.T) {
	b := &Server{Store: &Store{Config: Config{Env: "prod", ProviderAllowedCIDRs: []netip.Prefix{netip.MustParsePrefix("10.20.0.0/16")}, TrustedProxies: []netip.Prefix{netip.MustParsePrefix("127.0.0.1/32")}}}}
	for addr, allowed := range map[string]bool{"10.20.1.5": true, "10.21.1.5": false, "169.254.169.254": false, "127.0.0.1": false, "8.8.8.8": true} {
		if b.providerAddressAllowed(net.ParseIP(addr)) != allowed {
			t.Fatalf("wrong provider policy for %s", addr)
		}
	}
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
