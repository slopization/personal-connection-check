package app

import (
	"codeberg.org/modelgarden/personal-connection-check/internal/auth"
	"codeberg.org/modelgarden/personal-connection-check/internal/config"
	"codeberg.org/modelgarden/personal-connection-check/internal/networkinfo"
	"codeberg.org/modelgarden/personal-connection-check/internal/speedtest"
	"codeberg.org/modelgarden/personal-connection-check/internal/webui"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/coder/websocket"

	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

type Config struct {
	Runtime config.Config
	Logger  *slog.Logger
}
type bucket struct {
	tokens float64
	at     time.Time
}
type App struct {
	cfg       config.Config
	sessions  *auth.Sessions
	runs      *speedtest.Registry
	oidc      *auth.OIDC
	geo       *networkinfo.Geo
	mu        sync.Mutex
	login     map[string]bucket
	ws        chan struct{}
	verify    chan struct{}
	wsMu      sync.Mutex
	wsClose   map[*websocket.Conn]func()
	closeOnce sync.Once
	static    http.Handler
	logger    *slog.Logger
}

var oidcDiscoveryTimeout = 5 * time.Second

const runCleanupMargin = 10 * time.Second
const uploadRequestTimeout = 2 * time.Second

func runTTL(directionLimit time.Duration) time.Duration {
	return 2*directionLimit + runCleanupMargin
}

func uploadDeadline(now, runExpiry time.Time) time.Time {
	deadline := now.Add(uploadRequestTimeout)
	if runExpiry.Before(deadline) {
		return runExpiry
	}
	return deadline
}

func uploadReadStatus(err, runErr error) int {
	if runErr != nil {
		return -1
	}
	if err == nil {
		return 0
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return -1
	}
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		return http.StatusRequestEntityTooLarge
	}
	var timeout interface{ Timeout() bool }
	if errors.As(err, &timeout) && timeout.Timeout() {
		return 0
	}
	return http.StatusBadRequest
}

func finishUpload(body io.Closer, rc *http.ResponseController, release func()) {
	_ = body.Close()
	_ = rc.SetReadDeadline(time.Time{})
	release()
}

func finishWatchedUpload(stop chan struct{}, watcherResult <-chan bool, body io.Closer, rc *http.ResponseController, release func()) {
	close(stop)
	if !<-watcherResult {
		finishUpload(body, rc, release)
		return
	}
	_ = rc.SetReadDeadline(time.Time{})
	release()
}

func New(c Config) (*App, error) {
	logger := c.Logger
	if logger == nil {
		logger = slog.New(slog.NewTextHandler(io.Discard, nil))
	}
	if c.Runtime.MaxRuns == 0 {
		c.Runtime.MaxRuns = 2
		c.Runtime.MaxStreams = 8
		c.Runtime.MaxDuration = 15 * time.Second
		c.Runtime.UploadLimit = 16 << 20
	}
	cookieSecure := true
	if c.Runtime.PublicOrigin != "" {
		var err error
		cookieSecure, err = config.SessionCookieSecure(c.Runtime.PublicOrigin)
		if err != nil {
			return nil, fmt.Errorf("invalid public origin")
		}
	}
	c.Runtime.SessionCookieSecure = cookieSecure
	a := &App{cfg: c.Runtime, sessions: auth.NewSessions(c.Runtime.SessionKeys, c.Runtime.SessionCookieSecure), runs: speedtest.New(c.Runtime.MaxRuns, 1, c.Runtime.MaxStreams, runTTL(c.Runtime.MaxDuration)), login: map[string]bucket{}, ws: make(chan struct{}, c.Runtime.MaxRuns*2), verify: make(chan struct{}, 2), wsClose: map[*websocket.Conn]func(){}, geo: networkinfo.Open(c.Runtime.GeoCity, c.Runtime.GeoASN), static: webui.Handler(), logger: logger}
	if c.Runtime.OIDCIssuer != "" {
		logger.Info("OIDC discovery started", "event", "oidc_discovery_started")
		ctx, cancel := context.WithTimeout(context.Background(), oidcDiscoveryTimeout)
		defer cancel()
		var err error
		a.oidc, err = auth.NewOIDC(ctx, c.Runtime.OIDCIssuer, c.Runtime.OIDCClientID, c.Runtime.OIDCClientSecret, c.Runtime.OIDCRedirect, c.Runtime.OIDCEmails, c.Runtime.SessionKeys)
		if err != nil {
			logger.Error("OIDC discovery failed", "event", "oidc_discovery_failed", "reason", "provider_discovery")
			a.geo.Close()
			return nil, fmt.Errorf("OIDC discovery: %w", err)
		}
		logger.Info("OIDC discovery completed", "event", "oidc_discovery_completed")
	}
	return a, nil
}

// Close cancels active measurements, closes WebSockets, and releases GeoIP readers.
func (a *App) Close() {
	a.closeOnce.Do(func() {
		a.runs.CloseAll()
		a.wsMu.Lock()
		for _, closeConn := range a.wsClose {
			closeConn()
		}
		a.wsClose = map[*websocket.Conn]func(){}
		a.wsMu.Unlock()
		a.geo.Close()
	})
}
func (a *App) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.headers(w)
	switch {
	case r.URL.Path == "/healthz" || r.URL.Path == "/readyz":
		w.WriteHeader(200)
	case r.URL.Path == "/api/login/password" && r.Method == "POST":
		if !a.origin(w, r) {
			return
		}
		a.loginPassword(w, r)
	case r.URL.Path == "/api/logout" && r.Method == "POST":
		if !a.origin(w, r) {
			return
		}
		a.sessions.Clear(w)
		a.logger.Info("logout completed", "event", "auth_logout")
		w.WriteHeader(204)
	case r.URL.Path == "/api/auth/oidc/begin" && r.Method == "GET":
		a.beginOIDC(w, r)
	case r.URL.Path == "/api/auth/oidc/callback" && r.Method == "GET":
		a.callbackOIDC(w, r)
	case r.URL.Path == "/api/auth/config":
		json.NewEncoder(w).Encode(struct {
			Password      bool   `json:"password"`
			OIDC          bool   `json:"oidc"`
			FooterMessage string `json:"footerMessage"`
		}{a.cfg.PasswordHash != "", a.oidc != nil, a.cfg.FooterMessage})
	case r.URL.Path == "/api/network-info":
		a.protected(w, r, a.info)
	case r.URL.Path == "/api/test-runs" && r.Method == "POST":
		if a.origin(w, r) {
			a.protected(w, r, a.create)
		}
	case strings.HasPrefix(r.URL.Path, "/api/test-runs/") && r.Method == "DELETE":
		if a.origin(w, r) {
			a.protected(w, r, a.closeRun)
		}
	case strings.HasSuffix(r.URL.Path, "/download") && r.Method == "GET":
		a.protected(w, r, a.download)
	case strings.HasSuffix(r.URL.Path, "/upload") && r.Method == "POST":
		if a.origin(w, r) {
			a.protected(w, r, a.upload)
		}
	case r.URL.Path == "/api/ping":
		a.protected(w, r, a.ping)
	default:
		if r.Method == "GET" {
			a.static.ServeHTTP(w, r)
		} else {
			http.NotFound(w, r)
		}
	}
}
func (a *App) headers(w http.ResponseWriter) {
	w.Header().Set("Content-Security-Policy", "default-src 'self'; connect-src 'self' ws: wss:; style-src 'self' 'unsafe-inline'; img-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Referrer-Policy", "no-referrer")
}
func (a *App) origin(w http.ResponseWriter, r *http.Request) bool {
	if a.cfg.PublicOrigin == "" {
		return true
	}
	o := r.Header.Get("Origin")
	if o == "" || o != a.cfg.PublicOrigin {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	u, e := url.Parse(a.cfg.PublicOrigin)
	if e != nil || r.Host != u.Host {
		http.Error(w, "forbidden", http.StatusForbidden)
		return false
	}
	return true
}
func (a *App) claims(r *http.Request) (auth.Claims, bool) {
	c, e := r.Cookie("pcc_session")
	if e != nil {
		return auth.Claims{}, false
	}
	x, e := a.sessions.Decode(c.Value)
	return x, e == nil
}
func (a *App) protected(w http.ResponseWriter, r *http.Request, h func(http.ResponseWriter, *http.Request)) {
	if _, ok := a.claims(r); !ok {
		http.Error(w, "unauthorized", 401)
		return
	}
	h(w, r)
}
func (a *App) allowLogin(ip string) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	b := a.login[ip]
	if len(a.login) > 2048 && b.at.IsZero() {
		return false
	}
	b.tokens = min(5, b.tokens+now.Sub(b.at).Seconds()/12)
	b.at = now
	if b.tokens < 1 {
		a.login[ip] = b
		return false
	}
	b.tokens--
	a.login[ip] = b
	return true
}
func (a *App) loginPassword(w http.ResponseWriter, r *http.Request) {
	ip := networkinfo.ClientIP(r, a.cfg.Trusted).String()
	if !a.allowLogin(ip) {
		a.logger.Warn("password login denied", "event", "auth_login_denied", "method", "password", "reason", "rate_limited")
		time.Sleep(150 * time.Millisecond)
		http.Error(w, "invalid credentials", 401)
		return
	}
	defer r.Body.Close()
	select {
	case a.verify <- struct{}{}:
		defer func() { <-a.verify }()
	default:
		a.logger.Warn("password login denied", "event", "auth_login_denied", "method", "password", "reason", "verifier_busy")
		http.Error(w, "login busy", http.StatusTooManyRequests)
		return
	}
	var q struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&q) != nil || !auth.Verify(a.cfg.PasswordHash, []byte(q.Password)) {
		a.logger.Warn("password login denied", "event", "auth_login_denied", "method", "password", "reason", "invalid_credentials")
		time.Sleep(150 * time.Millisecond)
		http.Error(w, "invalid credentials", 401)
		return
	}
	if e := a.sessions.Set(w, auth.Claims{Subject: "password", Method: "password"}); e != nil {
		a.logger.Error("session creation failed", "event", "auth_session_failed", "method", "password", "reason", "session_encode")
		http.Error(w, "login unavailable", http.StatusInternalServerError)
		return
	}
	a.logger.Info("login completed", "event", "auth_login_succeeded", "method", "password")
	w.WriteHeader(204)
}
func (a *App) beginOIDC(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		http.NotFound(w, r)
		return
	}
	u, e := a.oidc.Begin(w, r)
	if e != nil {
		a.logger.Error("OIDC login unavailable", "event", "oidc_begin_failed", "reason", "state_encode")
		http.Error(w, "login unavailable", 503)
		return
	}
	a.logger.Info("OIDC login started", "event", "oidc_begin_succeeded")
	http.Redirect(w, r, u, http.StatusFound)
}
func (a *App) callbackOIDC(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		http.NotFound(w, r)
		return
	}
	sub, e := a.oidc.Callback(w, r)
	if e != nil {
		a.logger.Warn("OIDC login denied", "event", "auth_login_denied", "method", "oidc", "reason", auth.OIDCFailureReason(e))
		http.Error(w, "login denied", 401)
		return
	}
	if e := a.sessions.Set(w, auth.Claims{Subject: sub, Method: "oidc"}); e != nil {
		a.logger.Error("session creation failed", "event", "auth_session_failed", "method", "oidc", "reason", "session_encode")
		http.Error(w, "login unavailable", http.StatusInternalServerError)
		return
	}
	a.logger.Info("login completed", "event", "auth_login_succeeded", "method", "oidc")
	http.Redirect(w, r, "/", http.StatusFound)
}
func (a *App) create(w http.ResponseWriter, r *http.Request) {
	c, _ := a.claims(r)
	x, e := a.runs.Create(c.SessionID)
	if e != nil {
		http.Error(w, "run unavailable", 429)
		return
	}
	w.Header().Set("Cache-Control", "no-store, no-transform")
	json.NewEncoder(w).Encode(map[string]any{"id": x.ID, "expiresAt": x.Expires})
}
func (a *App) closeRun(w http.ResponseWriter, r *http.Request) {
	c, ok := a.claims(r)
	id := strings.TrimPrefix(r.URL.Path, "/api/test-runs/")
	if !ok || id == "" || strings.Contains(id, "/") || a.runs.CloseOwned(id, c.SessionID) != nil {
		http.NotFound(w, r)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (a *App) run(r *http.Request) (*speedtest.Run, bool) {
	c, ok := a.claims(r)
	if !ok {
		return nil, false
	}
	p := strings.Split(r.URL.Path, "/")
	if len(p) < 4 {
		return nil, false
	}
	x, e := a.runs.Get(p[3], c.SessionID)
	return x, e == nil
}
func (a *App) download(w http.ResponseWriter, r *http.Request) {
	x, ok := a.run(r)
	if !ok || r.URL.Query().Get("nonce") == "" || !x.Acquire(r.Context()) {
		http.Error(w, "not found", 404)
		return
	}
	defer x.Release()
	phase := 1250 * time.Millisecond
	if raw := r.URL.Query().Get("durationMs"); raw != "" {
		milliseconds, err := strconv.Atoi(raw)
		if err != nil || milliseconds < 1 || milliseconds > 1250 {
			http.Error(w, "invalid duration", http.StatusBadRequest)
			return
		}
		phase = time.Duration(milliseconds) * time.Millisecond
	}
	phaseEnd := time.Now().Add(phase)
	deadline := phaseEnd.Add(250 * time.Millisecond)
	if x.Expires.Before(deadline) {
		deadline = x.Expires
	}
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(deadline)
	defer rc.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Encoding", "identity")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	b := make([]byte, 64<<10)
	for time.Now().Before(phaseEnd) {
		select {
		case <-x.Context().Done():
			return
		default:
		}
		if _, e := w.Write(b); e != nil {
			return
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
	}
}
func (a *App) upload(w http.ResponseWriter, r *http.Request) {
	x, ok := a.run(r)
	if !ok || !x.Acquire(r.Context()) {
		http.Error(w, "not found", 404)
		return
	}
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(uploadDeadline(time.Now(), x.Expires))
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.UploadLimit)
	body := r.Body
	stopWatcher := make(chan struct{})
	watcherResult := make(chan bool, 1)
	go func() {
		closedBody := false
		select {
		case <-x.Context().Done():
			_ = body.Close()
			closedBody = true
		case <-stopWatcher:
		}
		watcherResult <- closedBody
	}()
	defer finishWatchedUpload(stopWatcher, watcherResult, body, rc, x.Release)
	start := time.Now()
	n, readErr := io.Copy(io.Discard, &contextReader{r: r.Body, ctx: x.Context()})
	if status := uploadReadStatus(readErr, x.Context().Err()); status != 0 {
		if status > 0 {
			http.Error(w, http.StatusText(status), status)
		}
		return
	}
	w.Header().Set("Cache-Control", "no-store, no-transform")
	json.NewEncoder(w).Encode(map[string]any{"bytes": n, "durationMs": time.Since(start).Milliseconds()})
}

type contextReader struct {
	r   io.Reader
	ctx context.Context
}

func (c *contextReader) Read(p []byte) (int, error) {
	select {
	case <-c.ctx.Done():
		return 0, c.ctx.Err()
	default:
		return c.r.Read(p)
	}
}
func (a *App) info(w http.ResponseWriter, r *http.Request) {
	ip := networkinfo.ClientIP(r, a.cfg.Trusted)
	json.NewEncoder(w).Encode(a.geo.Lookup(ip.AsSlice()))
}
func (a *App) ping(w http.ResponseWriter, r *http.Request) {
	if a.cfg.PublicOrigin != "" && r.Header.Get("Origin") != a.cfg.PublicOrigin {
		http.Error(w, "forbidden", 403)
		return
	}
	select {
	case a.ws <- struct{}{}:
		defer func() { <-a.ws }()
	default:
		http.Error(w, "busy", 429)
		return
	}
	c, e := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{a.cfg.PublicOrigin}})
	if e != nil {
		return
	}
	defer c.CloseNow()
	a.wsMu.Lock()
	a.wsClose[c] = func() { _ = c.CloseNow() }
	a.wsMu.Unlock()
	defer func() { a.wsMu.Lock(); delete(a.wsClose, c); a.wsMu.Unlock() }()
	for {
		ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
		typ, b, e := c.Read(ctx)
		cancel()
		if e != nil || typ != websocket.MessageText || len(b) > 1024 {
			return
		}
		ctx, cancel = context.WithTimeout(r.Context(), 5*time.Second)
		e = c.Write(ctx, typ, b)
		cancel()
		if e != nil {
			return
		}
	}
}
