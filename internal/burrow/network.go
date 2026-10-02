package burrow

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

func inNetworks(ip netip.Addr, networks []netip.Prefix) bool {
	for _, network := range networks {
		if network.Contains(ip.Unmap()) {
			return true
		}
	}
	return false
}
func (b *Server) clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	peer, err := netip.ParseAddr(host)
	if err != nil || !inNetworks(peer, b.Config.TrustedProxies) {
		return host
	}
	chain := strings.Split(r.Header.Get("X-Forwarded-For"), ",")
	if len(chain) > 32 {
		return host
	}
	for i := len(chain) - 1; i >= 0; i-- {
		ip, err := netip.ParseAddr(strings.TrimSpace(chain[i]))
		if err != nil {
			return host
		}
		if !inNetworks(ip, b.Config.TrustedProxies) {
			return ip.String()
		}
	}
	return host
}
