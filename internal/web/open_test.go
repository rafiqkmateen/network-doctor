package web

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"
)

// ND-263: the open page answers only when the address typed in the browser is this computer by
// name or number, with its own port, so a website cannot reach it through a DNS trick.
func TestOpenHostMustBeThisComputer(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:50000":              true,
		"localhost:50000":              true,
		"LOCALHOST:50000":              true,
		"[::1]:50000":                  true,
		"127.0.0.1:50001":              false, // another port
		"127.0.0.1":                    false, // no port
		"evil.example:50000":           false,
		"localhost.evil.example:50000": false,
		"192.168.1.20:50000":           false,
		"":                             false,
	}
	for host, want := range cases {
		if got := hostAllowed(host, "50000"); got != want {
			t.Errorf("open, Host %q: got %v want %v", host, got, want)
		}
	}
}

// ND-263: the serve and share pages keep working by number on the home network, but refuse any
// other name, which is what a DNS trick needs.
func TestServeHostMustBeANumberOrThisComputer(t *testing.T) {
	cases := map[string]bool{
		"127.0.0.1:8765":    true,
		"localhost:8765":    true,
		"[::1]:8765":        true,
		"192.168.1.20:8765": true,
		"192.168.1.20":      true,
		"evil.example:8765": false,
		"evil.example":      false,
		"":                  false,
	}
	for host, want := range cases {
		if got := hostAllowed(host, ""); got != want {
			t.Errorf("serve, Host %q: got %v want %v", host, got, want)
		}
	}
	h := servingHandler(func() ([]byte, error) { return []byte(`{}`), nil }, nil, nil, nil)
	for host, want := range map[string]int{"127.0.0.1:8765": 200, "evil.example:8765": http.StatusForbidden} {
		req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
		req.RemoteAddr, req.Host = "127.0.0.1:1", host
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		if rec.Code != want {
			t.Errorf("serve handler, Host %q: status %d want %d", host, rec.Code, want)
		}
	}
}

// ND-263: the page's heartbeat has its own address in every mode, so the serve page does not get errors.
func TestAliveAnswers(t *testing.T) {
	h := Handler(func() ([]byte, error) { return nil, nil }, nil)
	req := httptest.NewRequest(http.MethodGet, "/api/alive", nil)
	req.RemoteAddr = "127.0.0.1:1"
	req.Header.Set("X-NetDoctor", "1")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 {
		t.Errorf("/api/alive: status %d", rec.Code)
	}
}

// startOpen runs ServeOpen and gives its address and a channel that closes when it returns.
func startOpen(t *testing.T, run func() ([]byte, error), idle, noPage time.Duration) (string, chan error) {
	t.Helper()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- ServeOpen(context.Background(), run, OpenOptions{Idle: idle, NoPage: noPage, Ready: func(u string) { ready <- u }})
	}()
	select {
	case u := <-ready:
		return u, done
	case err := <-done:
		t.Fatalf("ServeOpen ended before it was ready: %v", err)
	case <-time.After(5 * time.Second):
		t.Fatal("ServeOpen never became ready")
	}
	return "", nil
}

func get(t *testing.T, u string) int {
	t.Helper()
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.Header.Set("X-NetDoctor", "1") // as the page sends it
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return resp.StatusCode
}

func okRun() ([]byte, error) { return []byte(`{}`), nil }

// ND-263: the system picks the port, on 127.0.0.1, so two open pages, or a running serve, never clash.
func TestOpenPicksAFreePort(t *testing.T) {
	a, doneA := startOpen(t, okRun, 300*time.Millisecond, 500*time.Millisecond)
	b, doneB := startOpen(t, okRun, 300*time.Millisecond, 500*time.Millisecond)
	ua, _ := url.Parse(a)
	ub, _ := url.Parse(b)
	if ua.Hostname() != "127.0.0.1" || ub.Hostname() != "127.0.0.1" {
		t.Errorf("open must listen on 127.0.0.1 only: %s %s", a, b)
	}
	if ua.Port() == ub.Port() || ua.Port() == "8765" || ub.Port() == "8765" {
		t.Errorf("each open needs its own free port, never 8765: %s %s", a, b)
	}
	<-doneA
	<-doneB
}

// ND-263: once the page has loaded and stops checking in, the server quits by itself, without error.
func TestOpenQuitsWhenThePageIsGone(t *testing.T) {
	u, done := startOpen(t, okRun, 300*time.Millisecond, 10*time.Second)
	if get(t, u) != 200 {
		t.Fatal("the page should load")
	}
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a normal quit is not an error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no heartbeat must make the server quit")
	}
}

// ND-263: while the heartbeat comes, the server stays, well past one quiet period.
func TestOpenStaysWhileThePageChecksIn(t *testing.T) {
	u, done := startOpen(t, okRun, 300*time.Millisecond, 10*time.Second)
	get(t, u)
	end := time.Now().Add(1200 * time.Millisecond)
	for time.Now().Before(end) {
		if code := get(t, u+"api/alive"); code != 200 {
			t.Fatalf("the heartbeat must be answered while the page is open, got %d", code)
		}
		select {
		case <-done:
			t.Fatal("the server quit although the page kept checking in")
		default:
		}
		time.Sleep(100 * time.Millisecond)
	}
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the server must quit once the heartbeat stops")
	}
}

// ND-263: a page that never loads (no browser, or it was closed at once) still lets the server quit.
func TestOpenQuitsWhenThePageNeverLoads(t *testing.T) {
	_, done := startOpen(t, okRun, 10*time.Second, 300*time.Millisecond)
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("a normal quit is not an error: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("a page that never loaded must still let the server quit")
	}
}

// ND-263: a check takes longer than a quiet period; the server must not quit in the middle of one.
func TestOpenWaitsForARunningCheck(t *testing.T) {
	slow := func() ([]byte, error) { time.Sleep(900 * time.Millisecond); return []byte(`{"ok":1}`), nil }
	u, done := startOpen(t, slow, 300*time.Millisecond, 10*time.Second)
	get(t, u)
	if code := get(t, u+"api/check"); code != 200 {
		t.Errorf("the check must finish even with no heartbeat during it, got %d", code)
	}
	<-done
}

// ND-263: the open server refuses a foreign Host and any client that is not this computer.
func TestOpenRefusesForeignHostAndOtherClients(t *testing.T) {
	u, done := startOpen(t, okRun, 300*time.Millisecond, 600*time.Millisecond)
	p, _ := url.Parse(u)
	req, _ := http.NewRequest(http.MethodGet, u+"api/health", nil)
	req.Host = "evil.example:" + p.Port()
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("a foreign Host must be refused, got %d", resp.StatusCode)
	}
	h := openHandler(okRun, p.Port(), func(string) {}, func(bool) {}, nil)
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	r.Host, r.RemoteAddr = "127.0.0.1:"+p.Port(), "192.168.1.9:4000"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	if rec.Code != http.StatusForbidden {
		t.Errorf("a client on another device must be refused, got %d", rec.Code)
	}
	<-done
}

// ND-263: loading the page without its header does not count as a heartbeat, so another website
// that loads it cannot keep `open` running; the never-loaded timer still ends it.
func TestPlainPageLoadsDoNotKeepOpenAlive(t *testing.T) {
	u, done := startOpen(t, okRun, 300*time.Millisecond, 900*time.Millisecond)
	stop := time.Now().Add(3 * time.Second)
	for time.Now().Before(stop) {
		select {
		case <-done:
			return // it ended although plain loads kept coming: right
		default:
		}
		if resp, err := http.Get(u); err == nil { // no X-NetDoctor header
			resp.Body.Close()
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("plain loads of the page must not keep the open server running")
}
