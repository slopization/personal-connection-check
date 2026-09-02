package main

import (
	"codeberg.org/modelgarden/personal-connection-check/internal/app"
	"codeberg.org/modelgarden/personal-connection-check/internal/auth"
	"codeberg.org/modelgarden/personal-connection-check/internal/config"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
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
