package main

import (
	"codeberg.org/modelgarden/personal-connection-check/internal/app"
	"codeberg.org/modelgarden/personal-connection-check/internal/auth"
	"codeberg.org/modelgarden/personal-connection-check/internal/config"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"
)

func main() {
	if len(os.Args) > 1 && os.Args[1] == "smoke" {
		u := os.Getenv("PCC_SMOKE_URL")
		if u == "" {
			u = "http://127.0.0.1:8080"
		}
		if err := runSmoke(u); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		u := os.Getenv("PCC_HEALTHCHECK_URL")
		if u == "" {
			u = "http://127.0.0.1:8080/healthz"
		}
		if err := runHealthcheck(u); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		return
	}
	if len(os.Args) > 1 && os.Args[1] == "hash-password" {
		if len(os.Args) > 2 {
			fmt.Fprintln(os.Stderr, "password arguments are not accepted")
			os.Exit(2)
		}
		p, e := auth.ReadPassword(os.Stdin)
		if e != nil {
			panic(e)
		}
		h, e := auth.Hash(p)
		if e != nil {
			panic(e)
		}
		fmt.Println(h)
		return
	}
	c, e := config.Load()
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	fmt.Println("listening on", c.Listen)
	a, e := app.New(app.Config{Runtime: c})
	if e != nil {
		fmt.Fprintln(os.Stderr, e)
		os.Exit(2)
	}
	server := &http.Server{Addr: c.Listen, Handler: a, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 25 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	select {
	case <-ctx.Done():
		a.Close()
		drain, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(drain)
	case e = <-errCh:
		if e != nil && e != http.ErrServerClosed {
			panic(e)
		}
	}
}

func runHealthcheck(url string) error {
	client := &http.Client{Timeout: 2 * time.Second}
	response, err := client.Get(url)
	if err != nil {
		return fmt.Errorf("healthcheck request: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("healthcheck status: %s", response.Status)
	}
	return nil
}

var assetReference = regexp.MustCompile(`(?:src|href)="(/assets/[^" ]+\.(?:js|css))"`)

func runSmoke(baseURL string) error {
	baseURL = strings.TrimRight(baseURL, "/")
	if err := runHealthcheck(baseURL + "/healthz"); err != nil {
		return err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Get(baseURL + "/")
	if err != nil {
		return fmt.Errorf("smoke root request: %w", err)
	}
	body, readErr := io.ReadAll(io.LimitReader(response.Body, 1<<20))
	response.Body.Close()
	if readErr != nil || response.StatusCode != http.StatusOK {
		return fmt.Errorf("smoke root response: status=%s read=%v", response.Status, readErr)
	}
	matches := assetReference.FindAllSubmatch(body, -1)
	if len(matches) == 0 {
		return fmt.Errorf("smoke root contains no hashed assets")
	}
	seen := map[string]bool{}
	for _, match := range matches {
		asset := string(match[1])
		if seen[asset] {
			continue
		}
		seen[asset] = true
		assetResponse, err := client.Get(baseURL + asset)
		if err != nil {
			return fmt.Errorf("smoke asset %s: %w", asset, err)
		}
		_, copyErr := io.Copy(io.Discard, io.LimitReader(assetResponse.Body, 8<<20))
		assetResponse.Body.Close()
		if copyErr != nil || assetResponse.StatusCode != http.StatusOK {
			return fmt.Errorf("smoke asset %s: status=%s read=%v", asset, assetResponse.Status, copyErr)
		}
	}
	return nil
}
