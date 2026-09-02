package networkinfo

import (
	"net"
	"net/http"
	"net/netip"
	"strings"
)

func trusted(a netip.Addr, ps []netip.Prefix) bool {
	for _, p := range ps {
		if p.Contains(a) {
			return true
		}
	}
	return false
}
func ClientIP(r *http.Request, trustedCIDRs []netip.Prefix) netip.Addr {
	host, _, e := net.SplitHostPort(r.RemoteAddr)
	if e != nil {
		host = r.RemoteAddr
	}
	peer, e := netip.ParseAddr(host)
	if e != nil {
		return netip.Addr{}
	}
	peer = peer.Unmap()
	if !trusted(peer, trustedCIDRs) {
		return peer
	}
	raw := r.Header.Values("X-Forwarded-For")
	if len(raw) != 1 {
		return peer
	}
	parts := strings.Split(raw[0], ",")
	if len(parts) == 0 {
		return peer
	}
	for i := len(parts) - 1; i >= 0; i-- {
		a, e := netip.ParseAddr(strings.TrimSpace(parts[i]))
		if e != nil {
			return peer
		}
		a = a.Unmap()
		if !trusted(a, trustedCIDRs) {
			return a
		}
	}
	return peer
}

type Info struct {
	IP      string `json:"ip"`
	City    string `json:"city,omitempty"`
	Country string `json:"country,omitempty"`
	ASN     string `json:"asn,omitempty"`
	ISP     string `json:"isp,omitempty"`
}
