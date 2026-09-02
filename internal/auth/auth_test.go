package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

func TestPasswordHashRoundTripAndRejectsWrong(t *testing.T) {
	h, e := Hash([]byte("correct"))
	if e != nil || !Verify(h, []byte("correct")) || Verify(h, []byte("wrong")) {
		t.Fatal("password verification contract failed")
	}
}
func TestSessionRejectsTamperAndExpiry(t *testing.T) {
	s := NewSessions([][]byte{make([]byte, 32)})
	s.Now = func() time.Time { return time.Unix(100, 0) }
	v, _ := s.Encode(Claims{Subject: "x", Expires: 101})
	if _, e := s.Decode(v + "x"); e == nil {
		t.Fatal("tamper accepted")
	}
	s.Now = func() time.Time { return time.Unix(102, 0) }
	if _, e := s.Decode(v); e == nil {
		t.Fatal("expired accepted")
	}
}

func TestNewOIDCDiscoveryHonorsContextDeadline(t *testing.T) {
	discoveryStarted := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		close(discoveryStarted)
		<-r.Context().Done()
	}))
	defer s.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 40*time.Millisecond)
	defer cancel()
	start := time.Now()
	_, err := NewOIDC(ctx, s.URL, "client", "secret", "https://app.example/callback", []string{"a@example.com"}, [][]byte{make([]byte, 32)})
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("discovery error=%v elapsed=%v, want bounded failure", err, time.Since(start))
	}
	select {
	case <-discoveryStarted:
	default:
		t.Fatal("discovery endpoint was not called")
	}
}

func TestOIDCCallbackRejectsInvalidSecurityClaims(t *testing.T) {
	for _, tc := range []struct {
		name   string
		mutate func(map[string]any, *url.Values)
	}{
		{"signature", func(c map[string]any, _ *url.Values) { c["signWithWrongKey"] = true }},
		{"audience", func(c map[string]any, _ *url.Values) { c["aud"] = "other-client" }},
		{"expiry", func(c map[string]any, _ *url.Values) { c["exp"] = time.Now().Add(-time.Minute).Unix() }},
		{"state", func(_ map[string]any, q *url.Values) { q.Set("state", "wrong") }},
		{"nonce", func(c map[string]any, _ *url.Values) { c["nonce"] = "wrong" }},
		{"email_verified", func(c map[string]any, _ *url.Values) { c["email_verified"] = false }},
		{"allowlist", func(c map[string]any, _ *url.Values) { c["email"] = "not-allowed@example.com" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			h := newOIDCTestProvider(t)
			defer h.Close()
			o, err := NewOIDC(context.Background(), h.URL, "client", "secret", "https://app.example/callback", []string{"allowed@example.com"}, [][]byte{make([]byte, 32)})
			if err != nil {
				t.Fatal(err)
			}
			begin := httptest.NewRecorder()
			u, err := o.Begin(begin, httptest.NewRequest(http.MethodGet, "/begin", nil))
			if err != nil {
				t.Fatal(err)
			}
			q, _ := url.Parse(u)
			claims := map[string]any{"iss": h.URL, "sub": "subject", "aud": "client", "exp": time.Now().Add(time.Minute).Unix(), "nonce": q.Query().Get("nonce"), "email": "allowed@example.com", "email_verified": true}
			callbackQ := q.Query()
			tc.mutate(claims, &callbackQ)
			h.claims = claims
			callbackQ.Set("code", q.Query().Get("code_challenge"))
			req := httptest.NewRequest(http.MethodGet, "/callback?"+callbackQ.Encode(), nil)
			req.AddCookie(begin.Result().Cookies()[0])
			if _, err := o.Callback(httptest.NewRecorder(), req); err == nil {
				t.Fatal("invalid OIDC response accepted")
			}
		})
	}
}

func TestOIDCCallbackUsesPKCEAndAcceptsValidAllowedEmail(t *testing.T) {
	h := newOIDCTestProvider(t)
	defer h.Close()
	o, err := NewOIDC(context.Background(), h.URL, "client", "secret", "https://app.example/callback", []string{" ALLOWED@example.com "}, [][]byte{make([]byte, 32)})
	if err != nil {
		t.Fatal(err)
	}
	begin := httptest.NewRecorder()
	u, err := o.Begin(begin, httptest.NewRequest(http.MethodGet, "/begin", nil))
	if err != nil {
		t.Fatal(err)
	}
	q, _ := url.Parse(u)
	if q.Query().Get("code_challenge_method") != "S256" || q.Query().Get("code_challenge") == "" {
		t.Fatal("PKCE S256 omitted")
	}
	h.expectedChallenge = q.Query().Get("code_challenge")
	h.claims = map[string]any{"iss": h.URL, "sub": "subject", "aud": "client", "exp": time.Now().Add(time.Minute).Unix(), "nonce": q.Query().Get("nonce"), "email": "allowed@example.com", "email_verified": true}
	callbackQ := q.Query()
	callbackQ.Set("code", "code")
	req := httptest.NewRequest(http.MethodGet, "/callback?"+callbackQ.Encode(), nil)
	req.AddCookie(begin.Result().Cookies()[0])
	sub, err := o.Callback(httptest.NewRecorder(), req)
	if err != nil || sub != "allowed@example.com" || !h.pkceOK {
		t.Fatalf("callback sub=%q err=%v pkce=%v", sub, err, h.pkceOK)
	}
}

type oidcTestProvider struct {
	*httptest.Server
	key               *rsa.PrivateKey
	claims            map[string]any
	pkceOK            bool
	expectedChallenge string
}

func newOIDCTestProvider(t *testing.T) *oidcTestProvider {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	h := &oidcTestProvider{key: key}
	h.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			json.NewEncoder(w).Encode(map[string]string{"issuer": h.URL, "authorization_endpoint": h.URL + "/authorize", "token_endpoint": h.URL + "/token", "jwks_uri": h.URL + "/jwks"})
		case "/jwks":
			json.NewEncoder(w).Encode(map[string]any{"keys": []any{map[string]string{"kty": "RSA", "kid": "key", "n": base64.RawURLEncoding.EncodeToString(h.key.PublicKey.N.Bytes()), "e": "AQAB"}}})
		case "/token":
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			verifier := r.Form.Get("code_verifier")
			sum := sha256.Sum256([]byte(verifier))
			h.pkceOK = verifier != "" && base64.RawURLEncoding.EncodeToString(sum[:]) == h.expectedChallenge
			// The expected challenge is supplied through the authorization query encoded in the test's code value.
			if h.claims == nil {
				http.Error(w, "claims absent", 500)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(map[string]any{"access_token": "a", "token_type": "Bearer", "id_token": h.signedToken(t, h.claims)})
		default:
			http.NotFound(w, r)
		}
	}))
	return h
}
func (h *oidcTestProvider) signedToken(t *testing.T, claims map[string]any) string {
	t.Helper()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"key"}`))
	payload, _ := json.Marshal(claims)
	body := header + "." + base64.RawURLEncoding.EncodeToString(payload)
	key := h.key
	if claims["signWithWrongKey"] == true {
		key, _ = rsa.GenerateKey(rand.Reader, 2048)
	}
	hash := sha256.Sum256([]byte(body))
	sig, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, hash[:])
	if err != nil {
		t.Fatal(err)
	}
	return body + "." + base64.RawURLEncoding.EncodeToString(sig)
}
