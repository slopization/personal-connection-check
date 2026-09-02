package app

import (
	"bytes"
	"codeberg.org/modelgarden/personal-connection-check/internal/config"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestHealthz(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	w := httptest.NewRecorder()
	a, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	a.ServeHTTP(w, r)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
}

func TestNewFailsWhenConfiguredOIDCDiscoveryFails(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	defer s.Close()
	old := oidcDiscoveryTimeout
	oidcDiscoveryTimeout = 40 * time.Millisecond
	defer func() { oidcDiscoveryTimeout = old }()
	start := time.Now()
	_, err := New(Config{Runtime: config.Config{OIDCIssuer: s.URL, OIDCClientID: "client", OIDCClientSecret: "secret", OIDCRedirect: "https://app.example/callback", OIDCEmails: []string{"a@example.com"}, SessionKeys: [][]byte{make([]byte, 32)}}})
	if err == nil || time.Since(start) > time.Second {
		t.Fatalf("New error=%v elapsed=%v, want bounded failure", err, time.Since(start))
	}
}

func TestCloseCancelsActiveRuns(t *testing.T) {
	a, err := New(Config{})
	if err != nil {
		t.Fatal(err)
	}
	run, err := a.runs.Create("session")
	if err != nil {
		t.Fatal(err)
	}
	a.Close()
	select {
	case <-run.Context().Done():
	case <-time.After(time.Second):
		t.Fatal("active run was not canceled")
	}
}

func TestLoginReturns429WhenGlobalVerifierIsSaturated(t *testing.T) {
	a, err := New(Config{Runtime: config.Config{PasswordHash: "$argon2id$v=19$m=8192,t=1,p=1$MDEyMzQ1Njc4OWFiY2RlZg$MDEyMzQ1Njc4OWFiY2RlZjAxMjM0NTY3ODlhYmNkZWY"}})
	if err != nil {
		t.Fatal(err)
	}
	a.verify <- struct{}{}
	a.verify <- struct{}{}
	req := httptest.NewRequest(http.MethodPost, "/api/login/password", bytes.NewBufferString(`{"password":"x"}`))
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("status=%d", w.Code)
	}
}
