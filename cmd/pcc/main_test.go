package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRunHealthcheck(t *testing.T) {
	good := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer good.Close()
	if err := runHealthcheck(good.URL + "/healthz"); err != nil {
		t.Fatalf("healthy service rejected: %v", err)
	}

	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	if err := runHealthcheck(bad.URL); err == nil {
		t.Fatal("unhealthy service accepted")
	}
}

func TestRunSmokeFetchesRootAndHashedAssets(t *testing.T) {
	assets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
		case "/":
			_, _ = w.Write([]byte(`<link rel="stylesheet" href="/assets/app-a1.css"><script src="/assets/app-b2.js"></script>`))
		case "/assets/app-a1.css", "/assets/app-b2.js":
			assets++
			w.WriteHeader(http.StatusOK)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	if err := runSmoke(server.URL); err != nil {
		t.Fatal(err)
	}
	if assets != 2 {
		t.Fatalf("expected two assets, got %d", assets)
	}
}
