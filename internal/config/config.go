package config

import (
	"codeberg.org/modelgarden/personal-connection-check/internal/auth"
	"encoding/base64"
	"fmt"
	"net/netip"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	PasswordHash                                             string
	SessionKeys                                              [][]byte
	OIDCIssuer, OIDCClientID, OIDCClientSecret, OIDCRedirect string
	OIDCEmails                                               []string
	Trusted                                                  []netip.Prefix
	MaxRuns, MaxStreams                                      int
	MaxDuration                                              time.Duration
	UploadLimit                                              int64
	GeoCity, GeoASN, Listen, PublicOrigin                    string
}

func Load() (Config, error) {
	c := Config{PasswordHash: os.Getenv("PCC_SHARED_PASSWORD_HASH"), OIDCIssuer: os.Getenv("PCC_OIDC_ISSUER"), OIDCClientID: os.Getenv("PCC_OIDC_CLIENT_ID"), OIDCClientSecret: os.Getenv("PCC_OIDC_CLIENT_SECRET"), OIDCRedirect: os.Getenv("PCC_OIDC_REDIRECT_URI"), PublicOrigin: strings.TrimSuffix(os.Getenv("PCC_PUBLIC_ORIGIN"), "/"), MaxRuns: integer("PCC_MAX_RUNS", 2), MaxStreams: integer("PCC_MAX_STREAMS", 8), MaxDuration: duration("PCC_MAX_DIRECTION_DURATION", 15*time.Second), UploadLimit: int64(integer("PCC_UPLOAD_LIMIT", 16<<20)), GeoCity: os.Getenv("PCC_GEOIP_CITY_DB"), GeoASN: os.Getenv("PCC_GEOIP_ASN_DB"), Listen: env("PCC_LISTEN", ":8080")}
	if c.PasswordHash == "" && c.OIDCIssuer == "" {
		return c, fmt.Errorf("authentication is required")
	}
	if c.PasswordHash != "" {
		if e := auth.ValidatePHC(c.PasswordHash); e != nil {
			return c, e
		}
	}
	oidc := []string{c.OIDCIssuer, c.OIDCClientID, c.OIDCClientSecret, c.OIDCRedirect}
	any, all := false, true
	for _, v := range oidc {
		any = any || v != ""
		all = all && v != ""
	}
	if any && !all {
		return c, fmt.Errorf("incomplete OIDC configuration")
	}
	if all {
		u, e := url.Parse(c.PublicOrigin)
		if e != nil || u.Scheme == "" || u.Host == "" {
			return c, fmt.Errorf("invalid public origin")
		}
	}
	for _, raw := range strings.Split(os.Getenv("PCC_SESSION_KEYS"), ",") {
		if raw == "" {
			continue
		}
		k, e := base64.RawURLEncoding.DecodeString(raw)
		if e != nil || len(k) < 32 {
			return c, fmt.Errorf("invalid session key")
		}
		c.SessionKeys = append(c.SessionKeys, k)
	}
	if len(c.SessionKeys) == 0 {
		return c, fmt.Errorf("session key is required")
	}
	if c.MaxRuns < 1 || c.MaxStreams < 1 || c.MaxStreams > 32 || c.MaxDuration <= 0 || c.MaxDuration > 15*time.Second {
		return c, fmt.Errorf("invalid measurement limits")
	}
	for _, s := range strings.Split(os.Getenv("PCC_TRUSTED_PROXY_CIDRS"), ",") {
		if s == "" {
			continue
		}
		p, e := netip.ParsePrefix(strings.TrimSpace(s))
		if e != nil {
			return c, fmt.Errorf("invalid trusted proxy CIDR")
		}
		c.Trusted = append(c.Trusted, p)
	}
	for _, e := range strings.Split(os.Getenv("PCC_OIDC_EMAIL_ALLOWLIST"), ",") {
		if e = strings.ToLower(strings.TrimSpace(e)); e != "" {
			c.OIDCEmails = append(c.OIDCEmails, e)
		}
	}
	if c.PublicOrigin == "" {
		return c, fmt.Errorf("public origin is required")
	}
	u, e := url.Parse(c.PublicOrigin)
	if e != nil || u.Scheme == "" || u.Host == "" {
		return c, fmt.Errorf("invalid public origin")
	}
	if all && len(c.OIDCEmails) == 0 {
		return c, fmt.Errorf("OIDC email allowlist is required")
	}
	return c, nil
}
func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func integer(k string, d int) int {
	v, e := strconv.Atoi(os.Getenv(k))
	if e == nil && v > 0 {
		return v
	}
	return d
}
func duration(k string, d time.Duration) time.Duration {
	v, e := time.ParseDuration(os.Getenv(k))
	if e == nil {
		return v
	}
	return d
}
