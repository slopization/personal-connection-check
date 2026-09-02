package app

import (
	"codeberg.org/modelgarden/personal-connection-check/internal/auth"
	"codeberg.org/modelgarden/personal-connection-check/internal/config"
	"codeberg.org/modelgarden/personal-connection-check/internal/networkinfo"
	"codeberg.org/modelgarden/personal-connection-check/internal/speedtest"
	"codeberg.org/modelgarden/personal-connection-check/internal/webui"
	"context"
	"encoding/json"
	"fmt"
	"github.com/coder/websocket"
	"io"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

type Config struct{ Runtime config.Config }
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
}

var oidcDiscoveryTimeout = 5 * time.Second

func New(c Config) (*App, error) {
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
	a := &App{cfg: c.Runtime, sessions: auth.NewSessions(c.Runtime.SessionKeys, c.Runtime.SessionCookieSecure), runs: speedtest.New(c.Runtime.MaxRuns, 1, c.Runtime.MaxStreams, 2*c.Runtime.MaxDuration+time.Second), login: map[string]bucket{}, ws: make(chan struct{}, c.Runtime.MaxRuns*2), verify: make(chan struct{}, 2), wsClose: map[*websocket.Conn]func(){}, geo: networkinfo.Open(c.Runtime.GeoCity, c.Runtime.GeoASN), static: webui.Handler()}
	if c.Runtime.OIDCIssuer != "" {
		ctx, cancel := context.WithTimeout(context.Background(), oidcDiscoveryTimeout)
		defer cancel()
		var err error
		a.oidc, err = auth.NewOIDC(ctx, c.Runtime.OIDCIssuer, c.Runtime.OIDCClientID, c.Runtime.OIDCClientSecret, c.Runtime.OIDCRedirect, c.Runtime.OIDCEmails, c.Runtime.SessionKeys)
		if err != nil {
			a.geo.Close()
			return nil, fmt.Errorf("OIDC discovery: %w", err)
		}
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
		w.WriteHeader(204)
	case r.URL.Path == "/api/auth/oidc/begin" && r.Method == "GET":
		a.beginOIDC(w, r)
	case r.URL.Path == "/api/auth/oidc/callback" && r.Method == "GET":
		a.callbackOIDC(w, r)
	case r.URL.Path == "/api/auth/config":
		json.NewEncoder(w).Encode(map[string]bool{"password": a.cfg.PasswordHash != "", "oidc": a.oidc != nil})
	case r.URL.Path == "/api/network-info":
		a.protected(w, r, a.info)
	case r.URL.Path == "/api/test-runs" && r.Method == "POST":
		if a.origin(w, r) {
			a.protected(w, r, a.create)
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
		time.Sleep(150 * time.Millisecond)
		http.Error(w, "invalid credentials", 401)
		return
	}
	defer r.Body.Close()
	select {
	case a.verify <- struct{}{}:
		defer func() { <-a.verify }()
	default:
		http.Error(w, "login busy", http.StatusTooManyRequests)
		return
	}
	var q struct {
		Password string `json:"password"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&q) != nil || !auth.Verify(a.cfg.PasswordHash, []byte(q.Password)) {
		time.Sleep(150 * time.Millisecond)
		http.Error(w, "invalid credentials", 401)
		return
	}
	a.sessions.Set(w, auth.Claims{Subject: "password", Method: "password"})
	w.WriteHeader(204)
}
func (a *App) beginOIDC(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		http.NotFound(w, r)
		return
	}
	u, e := a.oidc.Begin(w, r)
	if e != nil {
		http.Error(w, "login unavailable", 503)
		return
	}
	http.Redirect(w, r, u, http.StatusFound)
}
func (a *App) callbackOIDC(w http.ResponseWriter, r *http.Request) {
	if a.oidc == nil {
		http.NotFound(w, r)
		return
	}
	sub, e := a.oidc.Callback(w, r)
	if e != nil {
		http.Error(w, "login denied", 401)
		return
	}
	a.sessions.Set(w, auth.Claims{Subject: sub, Method: "oidc"})
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
	deadline := x.Expires
	rc := http.NewResponseController(w)
	_ = rc.SetWriteDeadline(deadline)
	defer rc.SetWriteDeadline(time.Time{})
	w.Header().Set("Content-Encoding", "identity")
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Cache-Control", "no-store, no-transform")
	w.Header().Set("X-Accel-Buffering", "no")
	b := make([]byte, 64<<10)
	for {
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
	defer x.Release()
	defer r.Body.Close()
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(x.Expires)
	defer rc.SetReadDeadline(time.Time{})
	done := make(chan struct{})
	go func() {
		select {
		case <-x.Context().Done():
			r.Body.Close()
		case <-done:
		}
	}()
	defer close(done)
	r.Body = http.MaxBytesReader(w, r.Body, a.cfg.UploadLimit)
	start := time.Now()
	n, e := io.Copy(io.Discard, &contextReader{r: r.Body, ctx: x.Context()})
	if e != nil {
		http.Error(w, "upload rejected", 413)
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
