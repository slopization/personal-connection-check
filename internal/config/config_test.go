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

func TestLoadBuildsOIDCRedirectFromPublicOrigin(t *testing.T) {
	t.Setenv("PCC_OIDC_ISSUER", "https://idp.example.com/oidc")
	t.Setenv("PCC_OIDC_CLIENT_ID", "pcc-client")
	t.Setenv("PCC_OIDC_CLIENT_SECRET", "test-only-secret")
	t.Setenv("PCC_OIDC_EMAIL_ALLOWLIST", "admin@example.com")
	t.Setenv("PCC_SESSION_KEYS", base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("PCC_PUBLIC_ORIGIN", "https://connection.example.com/")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.OIDCRedirect, "https://connection.example.com/api/auth/oidc/callback"; got != want {
		t.Fatalf("OIDCRedirect = %q, want %q", got, want)
	}
}

func TestLoadRejectsSessionKeysThatAESCannotUse(t *testing.T) {
	t.Setenv("PCC_SHARED_PASSWORD_HASH", "$argon2id$v=19$m=8192,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY")
	t.Setenv("PCC_SESSION_KEYS", base64.RawURLEncoding.EncodeToString(make([]byte, 48)))
	t.Setenv("PCC_PUBLIC_ORIGIN", "https://connection.example.com")
	_, err := Load()
	if err == nil || !strings.Contains(err.Error(), "session key") {
		t.Fatalf("Load() error = %v, want unusable AES key rejection", err)
	}
}

func TestLoadReadsFooterMessage(t *testing.T) {
	t.Setenv("PCC_SHARED_PASSWORD_HASH", "$argon2id$v=19$m=8192,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY")
	t.Setenv("PCC_SESSION_KEYS", base64.RawURLEncoding.EncodeToString(make([]byte, 32)))
	t.Setenv("PCC_PUBLIC_ORIGIN", "https://connection.example.com")
	t.Setenv("PCC_FOOTER_MESSAGE", "IP Geolocation by DB-IP: https://db-ip.com/")

	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got, want := c.FooterMessage, "IP Geolocation by DB-IP: https://db-ip.com/"; got != want {
		t.Fatalf("FooterMessage = %q, want %q", got, want)
	}
}
