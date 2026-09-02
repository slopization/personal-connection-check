package auth

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"time"
)

type Claims struct {
	Subject   string `json:"sub"`
	SessionID string `json:"sid"`
	Method    string `json:"method"`
	Expires   int64  `json:"exp"`
}
type Sessions struct {
	keys   [][]byte
	secure bool
	Now    func() time.Time
}

func NewSessions(keys [][]byte, secure ...bool) *Sessions {
	cookieSecure := true
	if len(secure) > 0 {
		cookieSecure = secure[0]
	}
	return &Sessions{keys: keys, secure: cookieSecure, Now: time.Now}
}
func randomID() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func (s *Sessions) Encode(c Claims) (string, error) {
	if c.Expires == 0 {
		c.Expires = s.Now().Add(8 * time.Hour).Unix()
	}
	if c.SessionID == "" {
		c.SessionID = randomID()
	}
	b, e := json.Marshal(c)
	if e != nil {
		return "", e
	}
	a, e := aes.NewCipher(s.keys[0])
	if e != nil {
		return "", e
	}
	g, e := cipher.NewGCM(a)
	if e != nil {
		return "", e
	}
	n := make([]byte, g.NonceSize())
	if _, e = rand.Read(n); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(append(n, g.Seal(nil, n, b, nil)...)), nil
}
func (s *Sessions) Decode(v string) (Claims, error) {
	raw, e := base64.RawURLEncoding.DecodeString(v)
	if e != nil {
		return Claims{}, e
	}
	for _, k := range s.keys {
		a, e := aes.NewCipher(k)
		if e != nil {
			continue
		}
		g, e := cipher.NewGCM(a)
		if e != nil || len(raw) < g.NonceSize() {
			continue
		}
		b, e := g.Open(nil, raw[:g.NonceSize()], raw[g.NonceSize():], nil)
		if e != nil {
			continue
		}
		var c Claims
		if json.Unmarshal(b, &c) == nil && c.Expires > s.Now().Unix() && c.SessionID != "" {
			return c, nil
		}
	}
	return Claims{}, errors.New("invalid session")
}
func (s *Sessions) Set(w http.ResponseWriter, c Claims) error {
	v, e := s.Encode(c)
	if e != nil {
		return e
	}
	http.SetCookie(w, &http.Cookie{Name: "pcc_session", Value: v, Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode, MaxAge: 8 * 3600})
	return nil
}
func (s *Sessions) Clear(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{Name: "pcc_session", Value: "", Path: "/", HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}

// Clear preserves the secure production default for callers that have no session configuration.
func Clear(w http.ResponseWriter) {
	NewSessions(nil).Clear(w)
}
