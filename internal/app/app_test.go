package app

import (
	"bytes"
	"codeberg.org/modelgarden/personal-connection-check/internal/auth"
	"codeberg.org/modelgarden/personal-connection-check/internal/config"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"
	"time"
)

type timeoutReadError struct{}

func (timeoutReadError) Error() string   { return "read timeout" }
func (timeoutReadError) Timeout() bool   { return true }
func (timeoutReadError) Temporary() bool { return true }

func TestUploadReadStatusAcknowledgesDeadlineBytes(t *testing.T) {
	for name, tc := range map[string]struct {
		readErr error
		runErr  error
		want    int
	}{
		"deadline is partial ACK":       {timeoutReadError{}, nil, 0},
		"run cancellation stops":        {context.Canceled, nil, -1},
		"body close after cancellation": {errors.New("body closed"), context.Canceled, -1},
		"oversize is rejected":          {&http.MaxBytesError{Limit: 1}, nil, http.StatusRequestEntityTooLarge},
		"malformed read rejected":       {errors.New("read failed"), nil, http.StatusBadRequest},
	} {
		t.Run(name, func(t *testing.T) {
			if got := uploadReadStatus(tc.readErr, tc.runErr); got != tc.want {
				t.Fatalf("uploadReadStatus(%v, %v) = %d, want %d", tc.readErr, tc.runErr, got, tc.want)
			}
		})
	}
}

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

func TestAuthConfigPublishesFooterMessage(t *testing.T) {
	a, err := New(Config{Runtime: config.Config{FooterMessage: "IP Geolocation by DB-IP: https://db-ip.com/"}})
	if err != nil {
		t.Fatal(err)
	}
	defer a.Close()

	r := httptest.NewRequest(http.MethodGet, "/api/auth/config", nil)
	w := httptest.NewRecorder()
	a.ServeHTTP(w, r)
	var got struct {
		FooterMessage string `json:"footerMessage"`
	}
	if err := json.NewDecoder(w.Body).Decode(&got); err != nil {
		t.Fatal(err)
	}
	if got.FooterMessage != "IP Geolocation by DB-IP: https://db-ip.com/" {
		t.Fatalf("footerMessage = %q", got.FooterMessage)
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
	if remaining, minimum := time.Until(run.Expires), runTTL(direction)-100*time.Millisecond; remaining < minimum {
		t.Fatalf("run lifetime %v, want at least %v", remaining, minimum)
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

func TestUploadDeadlineIsIndependentOfWholeRunTTL(t *testing.T) {
	now := time.Unix(1_000, 0)
	if got, want := uploadDeadline(now, now.Add(40*time.Second)), now.Add(2*time.Second); !got.Equal(want) {
		t.Fatalf("upload deadline = %v, want %v", got, want)
	}
	if got, want := uploadDeadline(now, now.Add(time.Second)), now.Add(time.Second); !got.Equal(want) {
		t.Fatalf("upload deadline past run expiry = %v, want %v", got, want)
	}
}

type orderedBody struct{ events *[]string }

func (b orderedBody) Close() error {
	*b.events = append(*b.events, "body-close")
	return nil
}

type deadlineWriter struct {
	header http.Header
	events *[]string
}

func (w *deadlineWriter) Header() http.Header       { return w.header }
func (w *deadlineWriter) Write([]byte) (int, error) { return 0, nil }
func (w *deadlineWriter) WriteHeader(int)           {}
func (w *deadlineWriter) SetReadDeadline(time.Time) error {
	*w.events = append(*w.events, "deadline-reset")
	return nil
}

func TestFinishUploadKeepsDeadlineThroughBodyClose(t *testing.T) {
	events := []string{}
	w := &deadlineWriter{header: http.Header{}, events: &events}
	finishUpload(orderedBody{events: &events}, http.NewResponseController(w), func() {
		events = append(events, "release")
	})
	want := []string{"body-close", "deadline-reset", "release"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("cleanup order = %v, want %v", events, want)
	}
}

func TestFinishWatchedUploadJoinsCancellationBeforeCleanup(t *testing.T) {
	events := []string{}
	stop := make(chan struct{})
	watcherResult := make(chan bool, 1)
	go func() {
		<-stop
		events = append(events, "watch-stopped")
		watcherResult <- false
	}()
	w := &deadlineWriter{header: http.Header{}, events: &events}
	finishWatchedUpload(stop, watcherResult, orderedBody{events: &events}, http.NewResponseController(w), func() {
		events = append(events, "release")
	})
	want := []string{"watch-stopped", "body-close", "deadline-reset", "release"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("watched cleanup order = %v, want %v", events, want)
	}
}

func TestFinishWatchedUploadDoesNotCloseBodyTwiceAfterCancellation(t *testing.T) {
	events := []string{"watcher-body-close"}
	stop := make(chan struct{})
	watcherResult := make(chan bool, 1)
	watcherResult <- true
	w := &deadlineWriter{header: http.Header{}, events: &events}
	finishWatchedUpload(stop, watcherResult, orderedBody{events: &events}, http.NewResponseController(w), func() {
		events = append(events, "release")
	})
	want := []string{"watcher-body-close", "deadline-reset", "release"}
	if !reflect.DeepEqual(events, want) {
		t.Fatalf("cancelled cleanup order = %v, want %v", events, want)
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

func TestRunTTLIncludesBothDirectionsAndCleanupMargin(t *testing.T) {
	if got, want := runTTL(15*time.Second), 40*time.Second; got != want {
		t.Fatalf("runTTL(15s) = %v, want %v", got, want)
	}
}
