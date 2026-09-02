package app

import (
	"bytes"
	"codeberg.org/modelgarden/personal-connection-check/internal/auth"
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

func TestNewRejectsNonLoopbackHTTPOrigin(t *testing.T) {
	_, err := New(Config{Runtime: config.Config{PublicOrigin: "http://connection.example.com", MaxRuns: 1, MaxStreams: 1, MaxDuration: time.Second, UploadLimit: 1}})
	if err == nil {
		t.Fatal("New accepted non-loopback HTTP origin")
	}
}

func TestRunLifetimeAllowsBothDirectionCaps(t *testing.T) {
	const direction = 40 * time.Millisecond
	a, err := New(Config{Runtime: config.Config{MaxRuns: 1, MaxStreams: 1, MaxDuration: direction, UploadLimit: 1}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	run, err := a.runs.Create("session")
	if err != nil {
		t.Fatal(err)
	}
	if remaining := time.Until(run.Expires); remaining < 2*direction+900*time.Millisecond {
		t.Fatalf("run lifetime %v, want enough for both directions plus bounded overhead", remaining)
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

func TestDownloadPhaseEndsAndReleasesStreamSlot(t *testing.T) {
	a, err := New(Config{Runtime: config.Config{SessionKeys: [][]byte{make([]byte, 32)}, MaxRuns: 1, MaxStreams: 1, MaxDuration: time.Second, UploadLimit: 1}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	const sessionID = "session"
	run, err := a.runs.Create(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	token, err := a.sessions.Encode(auth.Claims{Subject: "test", SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodGet, "/api/test-runs/"+run.ID+"/download?nonce=x&durationMs=10", nil)
	req.AddCookie(&http.Cookie{Name: "pcc_session", Value: token})
	w := httptest.NewRecorder()
	started := time.Now()
	a.ServeHTTP(w, req)
	if w.Code != http.StatusOK || time.Since(started) > 250*time.Millisecond {
		t.Fatalf("download status=%d elapsed=%v, want bounded successful phase", w.Code, time.Since(started))
	}
	if !run.Acquire(req.Context()) {
		t.Fatal("download phase did not release its stream slot")
	}
	run.Release()
}

func TestDeleteRunClosesOwnedRunAndRestoresCapacity(t *testing.T) {
	a, err := New(Config{Runtime: config.Config{SessionKeys: [][]byte{make([]byte, 32)}, MaxRuns: 1, MaxStreams: 1, MaxDuration: time.Second, UploadLimit: 1}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()
	const sessionID = "session"
	run, err := a.runs.Create(sessionID)
	if err != nil {
		t.Fatal(err)
	}
	token, err := a.sessions.Encode(auth.Claims{Subject: "test", SessionID: sessionID})
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequest(http.MethodDelete, "/api/test-runs/"+run.ID, nil)
	req.AddCookie(&http.Cookie{Name: "pcc_session", Value: token})
	w := httptest.NewRecorder()
	a.ServeHTTP(w, req)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status=%d, want 204", w.Code)
	}
	if _, err := a.runs.Create("next"); err != nil {
		t.Fatalf("delete did not restore capacity: %v", err)
	}
}
