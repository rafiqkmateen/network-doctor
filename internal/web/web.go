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
	return HandlerWith(run, allow, nil)
}

// HandlerWith is Handler that also calls onRemote with the address of any
// allowed device other than this computer that opens the page.
func HandlerWith(run func() ([]byte, error), allow []string, onRemote func(ip string)) http.Handler {
	return HandlerSave(run, allow, onRemote, nil)
}

// HandlerSave is HandlerWith that also takes the device choices and the new-device switch from
// the page on this computer (ND-264, ND-265). With a nil Saver those addresses are refused.
func HandlerSave(run func() ([]byte, error), allow []string, onRemote func(ip string), sv *Saver) http.Handler {
	var mu sync.Mutex
	mux := http.NewServeMux()
	mux.HandleFunc("/api/known", knownHandler(sv))
	mux.HandleFunc("/api/settings", settingsHandler(sv))
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
	// The page's heartbeat; only the open command counts it, the other pages just answer (ND-263).
	mux.HandleFunc("/api/alive", func(w http.ResponseWriter, r *http.Request) {
		if !fromPage(r) {
			http.Error(w, "not allowed", http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/check", func(w http.ResponseWriter, r *http.Request) {
		if !fromPage(r) {
			http.Error(w, "not allowed", http.StatusForbidden)
			return
		}
		if canSave(r, sv) {
			w.Header().Set("X-NetDoctor-Save", "1") // the page shows its buttons only then (ND-264)
		}
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
		noFrames(w)
		if !AllowedRemote(r.RemoteAddr, allow) {
			http.Error(w, "not allowed", http.StatusForbidden)
			return
		}
		if onRemote != nil && r.URL.Path == "/" {
			host, _, err := net.SplitHostPort(r.RemoteAddr)
			if err != nil {
				host = r.RemoteAddr
			}
			if ip := net.ParseIP(host); ip != nil && !ip.IsLoopback() {
				onRemote(host)
			}
		}
		mux.ServeHTTP(w, r)
	})
}

// noFrames tells the browser that no other website may show the page inside a frame, so a
// website cannot trick a person into pressing its buttons. Every response carries it (ND-263).
func noFrames(w http.ResponseWriter) {
	w.Header().Set("X-Frame-Options", "DENY")
	w.Header().Set("Content-Security-Policy", "frame-ancestors 'none'")
}

// fromPage is true when a request surely comes from the page itself: it carries the page's own
// header, which another website cannot add without asking first, or the browser says it comes
// from the same page. Starting a check or keeping the open page alive needs this (ND-263).
func fromPage(r *http.Request) bool {
	return r.Header.Get("X-NetDoctor") == "1" || r.Header.Get("Sec-Fetch-Site") == "same-origin"
}

// canSave is true when a save from this page would be accepted: there is a saver, and the request
// comes from this computer at an address naming this computer (see fromThisPage).
func canSave(r *http.Request, sv *Saver) bool {
	return sv != nil && sv.Devices != nil && AllowedRemote(r.RemoteAddr, nil) && namesThisComputer(r.Host)
}

// Serve runs the page until the context ends.
func Serve(ctx context.Context, addr string, allow []string, run func() ([]byte, error)) error {
	return ServeWith(ctx, addr, allow, run, nil)
}

// ServeWith is Serve that reports other devices opening the page (see HandlerWith).
func ServeWith(ctx context.Context, addr string, allow []string, run func() ([]byte, error), onRemote func(ip string)) error {
	return ServeSave(ctx, addr, allow, run, onRemote, nil)
}

// ServeSave is ServeWith that also takes the device choices and the switch (see HandlerSave).
func ServeSave(ctx context.Context, addr string, allow []string, run func() ([]byte, error), onRemote func(ip string), sv *Saver) error {
	if err := CheckListen(addr, allow); err != nil {
		return err
	}
	srv := &http.Server{Addr: addr, Handler: servingHandler(run, allow, onRemote, sv), ReadHeaderTimeout: 5 * time.Second}
	go func() { <-ctx.Done(); srv.Close() }()
	err := srv.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
