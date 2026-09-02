package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
	"net/http"
	"strings"
	"time"
)

type oidcFailure struct{ reason string }

func (e *oidcFailure) Error() string { return "OIDC login denied" }

func oidcFailed(reason string) error { return &oidcFailure{reason: reason} }

// OIDCFailureReason returns a bounded, credential-free reason suitable for
// operational logs. It never returns provider response bodies or token data.
func OIDCFailureReason(err error) string {
	var failure *oidcFailure
	if errors.As(err, &failure) {
		return failure.reason
	}
	return "unknown"
}

type OIDC struct {
	Issuer, ClientID, ClientSecret, Redirect string
	Emails                                   map[string]bool
	provider                                 *oidc.Provider
	verifier                                 *oidc.IDTokenVerifier
	oauth                                    oauth2.Config
	states                                   *Sessions
}
type oidcState struct {
	State, Nonce, Verifier string `json:"-"`
	Expires                int64  `json:"exp"`
}

func random(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}
func NewOIDC(ctx context.Context, issuer, clientID, secret, redirect string, emails []string, keys [][]byte) (*OIDC, error) {
	p, e := oidc.NewProvider(ctx, issuer)
	if e != nil {
		return nil, e
	}
	m := map[string]bool{}
	for _, x := range emails {
		m[strings.ToLower(strings.TrimSpace(x))] = true
	}
	o := &OIDC{Issuer: issuer, ClientID: clientID, ClientSecret: secret, Redirect: redirect, Emails: m, provider: p, verifier: p.Verifier(&oidc.Config{ClientID: clientID}), states: NewSessions(keys)}
	o.oauth = oauth2.Config{ClientID: clientID, ClientSecret: secret, RedirectURL: redirect, Endpoint: p.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "email"}}
	return o, nil
}
func (o *OIDC) Begin(w http.ResponseWriter, r *http.Request) (string, error) {
	st := oidcState{State: random(24), Nonce: random(24), Verifier: random(48), Expires: time.Now().Add(5 * time.Minute).Unix()}
	v, e := o.states.Encode(Claims{Subject: st.State, SessionID: st.Nonce, Method: st.Verifier, Expires: st.Expires})
	if e != nil {
		return "", e
	}
	http.SetCookie(w, &http.Cookie{Name: "pcc_oidc", Value: v, Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: 300})
	h := sha256.Sum256([]byte(st.Verifier))
	return o.oauth.AuthCodeURL(st.State, oauth2.SetAuthURLParam("nonce", st.Nonce), oauth2.SetAuthURLParam("code_challenge", base64.RawURLEncoding.EncodeToString(h[:])), oauth2.SetAuthURLParam("code_challenge_method", "S256")), nil
}
func (o *OIDC) Callback(w http.ResponseWriter, r *http.Request) (string, error) {
	defer func() {
		http.SetCookie(w, &http.Cookie{Name: "pcc_oidc", Value: "", Path: "/", HttpOnly: true, Secure: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
	}()
	c, e := r.Cookie("pcc_oidc")
	if e != nil {
		return "", oidcFailed("state")
	}
	st, e := o.states.Decode(c.Value)
	if e != nil || r.URL.Query().Get("state") == "" || r.URL.Query().Get("state") != st.Subject {
		return "", oidcFailed("state")
	}
	tok, e := o.oauth.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.SetAuthURLParam("code_verifier", st.Method))
	if e != nil {
		return "", oidcFailed("token_exchange")
	}
	raw, ok := tok.Extra("id_token").(string)
	if !ok {
		return "", oidcFailed("id_token_missing")
	}
	id, e := o.verifier.Verify(r.Context(), raw)
	if e != nil {
		return "", oidcFailed("id_token_verification")
	}
	var cl struct {
		Email           string `json:"email"`
		EmailVerified   bool   `json:"email_verified"`
		Nonce           string `json:"nonce"`
		AuthorizedParty string `json:"azp"`
	}
	if e = id.Claims(&cl); e != nil {
		return "", oidcFailed("claims")
	}
	if cl.Nonce != st.SessionID {
		return "", oidcFailed("nonce")
	}
	if !cl.EmailVerified {
		return "", oidcFailed("email_unverified")
	}
	email := strings.ToLower(strings.TrimSpace(cl.Email))
	if email == "" || !o.Emails[email] {
		return "", oidcFailed("email_not_allowed")
	}
	if cl.AuthorizedParty != "" && cl.AuthorizedParty != o.ClientID {
		return "", oidcFailed("authorized_party")
	}
	return email, nil
}
