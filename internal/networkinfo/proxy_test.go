package networkinfo

import (
	"net/http/httptest"
	"net/netip"
	"testing"
)

func TestClientIPOnlyTrustsConfiguredProxy(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	r.RemoteAddr = "198.51.100.2:2"
	r.Header.Set("X-Forwarded-For", "203.0.113.9")
	if got := ClientIP(r, nil).String(); got != "198.51.100.2" {
		t.Fatal(got)
	}
	p := netip.MustParsePrefix("198.51.100.0/24")
	if got := ClientIP(r, []netip.Prefix{p}).String(); got != "203.0.113.9" {
		t.Fatal(got)
	}
}
