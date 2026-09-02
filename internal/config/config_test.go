package config

import (
	"encoding/base64"
	"strings"
	"testing"
)

func TestLoadOnlyAllowsHTTPPublicOriginOnLoopback(t *testing.T) {
	for _, tc := range []struct {
		name    string
		origin  string
		secure  bool
		wantErr bool
	}{
		{name: "loopback IPv4", origin: "http://127.0.0.1:8080", secure: false},
		{name: "loopback IPv6", origin: "http://[::1]:8080", secure: false},
		{name: "HTTPS production", origin: "https://connection.example.com", secure: true},
		{name: "hostname HTTP", origin: "http://localhost:8080", wantErr: true},
		{name: "non-loopback HTTP", origin: "http://connection.example.com", wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv("PCC_SHARED_PASSWORD_HASH", "$argon2id$v=19$m=8192,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY")
			t.Setenv("PCC_SESSION_KEYS", base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
			t.Setenv("PCC_PUBLIC_ORIGIN", tc.origin)
			c, err := Load()
			if tc.wantErr {
				if err == nil || !strings.Contains(err.Error(), "public origin") {
					t.Fatalf("Load() error = %v, want public-origin rejection", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if c.SessionCookieSecure != tc.secure {
				t.Fatalf("SessionCookieSecure = %v, want %v", c.SessionCookieSecure, tc.secure)
			}
		})
	}
}
