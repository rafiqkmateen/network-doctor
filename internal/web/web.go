// Package web serves the local page and the check results.
package web

import (
	"context"
	_ "embed"
	"errors"
	"net"
	"net/http"
	"sync"
	"time"
)

//go:embed index.html
var page []byte

// AllowedRemote is true for this machine, and for any address in the allow list.
func AllowedRemote(remote string, allow []string) bool {
	host, _, err := net.SplitHostPort(remote)
	if err != nil {
		host = remote
	}
	ip := net.ParseIP(host)
	if ip == nil {
		return false
	}
	if ip.IsLoopback() {
		return true
	}
	for _, c := range allow {
		if _, n, err := net.ParseCIDR(c); err == nil && n.Contains(ip) {
			return true
		}
		if a := net.ParseIP(c); a != nil && a.Equal(ip) {
			return true
		}
	}
	return false
}

// CheckListen refuses to open the page to the network without an allow list.
func CheckListen(addr string, allow []string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return err
	}
	if ip := net.ParseIP(host); host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return nil
	}
	if len(allow) == 0 {
		return errors.New("listening beyond this machine needs --allow with the addresses that may connect")
	}
	return nil
}

// Handler serves the page and /api/check. run does one full check as JSON.
func Handler(run func() ([]byte, error), allow []string) http.Handler {
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(page)
	})
	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/check", func(w http.ResponseWriter, r *http.Request) {
		if !mu.TryLock() {
			http.Error(w, `{"error":"a check is already running"}`, http.StatusTooManyRequests)
			return
		}
		defer mu.Unlock()
		b, err := run()
		if err != nil {
			http.Error(w, `{"error":"the check failed"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Write(b)
	})
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !AllowedRemote(r.RemoteAddr, allow) {
			http.Error(w, "not allowed", http.StatusForbidden)
			return
		}
		mux.ServeHTTP(w, r)
	})
}

// Serve runs the page until the context ends.
func Serve(ctx context.Context, addr string, allow []string, run func() ([]byte, error)) error {
	if err := CheckListen(addr, allow); err != nil {
		return err
	}
	srv := &http.Server{Addr: addr, Handler: Handler(run, allow), ReadHeaderTimeout: 5 * time.Second}
	go func() { <-ctx.Done(); srv.Close() }()
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
