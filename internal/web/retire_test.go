package web

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// startOpenState runs ServeOpen with a state file and returns its address and done channel.
func startOpenState(t *testing.T, run func() ([]byte, error), state string) (string, chan error) {
	t.Helper()
	ready := make(chan string, 1)
	done := make(chan error, 1)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go func() {
		done <- ServeOpen(ctx, run, OpenOptions{Idle: time.Minute, NoPage: time.Minute,
			Grace: 400 * time.Millisecond, StateFile: state, Ready: func(u string) { ready <- u }})
	}()
	select {
	case u := <-ready:
		return u, done
	case <-time.After(5 * time.Second):
		t.Fatal("the server did not start")
	}
	return "", done
}

func readState(t *testing.T, path string) openState {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the state file must exist: %v", err)
	}
	var s openState
	if err := json.Unmarshal(b, &s); err != nil {
		t.Fatalf("the state file must be JSON: %v", err)
	}
	return s
}

func aliveCode(t *testing.T, u string) int {
	t.Helper()
	req, _ := http.NewRequest("GET", u+"api/alive", nil)
	req.Header.Set("X-NetDoctor", "1")
	res, err := http.DefaultClient.Do(req)
	if err != nil {
		return -1
	}
	res.Body.Close()
	return res.StatusCode
}

// One page at a time: a second open retires the first. The old page hears "retired" on its next
// heartbeat and the old server quits within a few seconds; the state file then names the new one.
func TestNewOpenRetiresTheOldOne(t *testing.T) {
	state := filepath.Join(t.TempDir(), "open.json")
	u1, done1 := startOpenState(t, okRun, state)
	s1 := readState(t, state)
	if s1.Port == "" || len(s1.Token) < 16 {
		t.Fatalf("the state file must hold the port and a random token, got %+v", s1)
	}
	if aliveCode(t, u1) != 200 {
		t.Fatal("the first page must be alive")
	}
	u2, done2 := startOpenState(t, okRun, state)
	if u2 == u1 {
		t.Fatal("the new run must have its own server")
	}
	if code := aliveCode(t, u1); code != http.StatusGone {
		t.Fatalf("the old page must hear it is retired (410), got %d", code)
	}
	select {
	case err := <-done1:
		if err != nil {
			t.Errorf("a retired server quits normally: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("the old server must quit within a few seconds")
	}
	if s2 := readState(t, state); s2.Token == s1.Token || s2.Port == s1.Port {
		t.Error("the state file must name the new server")
	}
	if aliveCode(t, u2) != 200 {
		t.Error("the new page must be alive")
	}
	_ = done2
}

// The retired server quits even if its page never hears, and never during a running check.
func TestRetiredServerFinishesARunningCheck(t *testing.T) {
	state := filepath.Join(t.TempDir(), "open.json")
	slow := func() ([]byte, error) { time.Sleep(1500 * time.Millisecond); return []byte(`{"ok":1}`), nil }
	u1, done1 := startOpenState(t, slow, state)
	got := make(chan int, 1)
	go func() { got <- get(t, u1+"api/check") }()
	time.Sleep(200 * time.Millisecond)
	startOpenState(t, okRun, state)
	if code := <-got; code != 200 {
		t.Errorf("a check already running must finish, got %d", code)
	}
	select {
	case <-done1:
	case <-time.After(5 * time.Second):
		t.Fatal("the retired server must quit after the check")
	}
}

// A stale or foreign state file never blocks a new run, and is overwritten.
func TestStaleStateFileDoesNotBlock(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, "open.json")
	for _, junk := range []string{`{"port":"1","token":"x"}`, `not json`, `{"port":"99999"}`} {
		if err := os.WriteFile(state, []byte(junk), 0o600); err != nil {
			t.Fatal(err)
		}
		u, _ := startOpenState(t, okRun, state)
		if aliveCode(t, u) != 200 {
			t.Fatalf("a stale file %q must not block the new run", junk)
		}
		if s := readState(t, state); !strings.HasSuffix(u, ":"+s.Port+"/") {
			t.Errorf("the stale file must be overwritten, got %+v for %s", s, u)
		}
	}
}

// The retire call needs the token in its own header: a wrong token, a cross-site page (Origin or
// Sec-Fetch-Site), a GET, or no token at all retires nothing.
func TestRetireRefusesStrangers(t *testing.T) {
	state := filepath.Join(t.TempDir(), "open.json")
	u, _ := startOpenState(t, okRun, state)
	tok := readState(t, state).Token
	try := func(method, token string, extra map[string]string) int {
		req, _ := http.NewRequest(method, u+"api/retire", nil)
		if token != "" {
			req.Header.Set(retireHeader, token)
		}
		for k, v := range extra {
			req.Header.Set(k, v)
		}
		res, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		return res.StatusCode
	}
	cases := []struct {
		name, method, token string
		extra               map[string]string
	}{
		{"no token", "POST", "", nil},
		{"wrong token", "POST", "nope", nil},
		{"GET", "GET", tok, nil},
		{"from a website", "POST", tok, map[string]string{"Origin": "https://evil.example"}},
		{"cross-site mark", "POST", tok, map[string]string{"Sec-Fetch-Site": "cross-site"}},
		{"page header only", "POST", "", map[string]string{"X-NetDoctor": "1"}},
	}
	for _, c := range cases {
		if code := try(c.method, c.token, c.extra); code < 400 {
			t.Errorf("%s: must be refused, got %d", c.name, code)
		}
	}
	if aliveCode(t, u) != 200 {
		t.Fatal("a refused retire must leave the page alive")
	}
}

// Quitting removes the state file it wrote.
func TestStateFileRemovedOnQuit(t *testing.T) {
	state := filepath.Join(t.TempDir(), "open.json")
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- ServeOpen(context.Background(), okRun, OpenOptions{Idle: time.Minute,
			NoPage: 200 * time.Millisecond, StateFile: state, Ready: func(u string) { ready <- u }})
	}()
	<-ready
	readState(t, state)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the server must quit")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Error("the state file must be removed on quit")
	}
}

// A hidden old tab sends its heartbeat about once a minute, so the retire wait must be longer than
// that by default; scaled down, a heartbeat well after the retire still hears "retired".
func TestRetiredServerWaitsForASlowHeartbeat(t *testing.T) {
	if defaultGrace < 70*time.Second {
		t.Fatalf("the default retire wait must outlast a hidden tab's heartbeat, got %v", defaultGrace)
	}
	state := filepath.Join(t.TempDir(), "open.json")
	ready := make(chan string, 1)
	done := make(chan error, 1)
	go func() {
		done <- ServeOpen(context.Background(), okRun, OpenOptions{Idle: time.Minute, NoPage: time.Minute,
			Grace: 3 * time.Second, StateFile: state, Ready: func(u string) { ready <- u }})
	}()
	u1 := <-ready
	retireOld(state)                   // the retire request, as a new press sends it
	time.Sleep(800 * time.Millisecond) // 20 seconds scaled to a 75 second wait
	if code := aliveCode(t, u1); code != http.StatusGone {
		t.Fatalf("a late heartbeat must still hear it is retired, got %d", code)
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("once its page has been told, the server quits at once")
	}
}

// Two presses at nearly the same time: exactly one server stays and the note names it.
func TestTwoPressesAtOnceLeaveOnePage(t *testing.T) {
	state := filepath.Join(t.TempDir(), "open.json")
	old, _ := startOpenState(t, okRun, state)
	type res struct {
		u    string
		done chan error
	}
	out := make(chan res, 2)
	for i := 0; i < 2; i++ {
		go func() {
			ready := make(chan string, 1)
			done := make(chan error, 1)
			go func() {
				done <- ServeOpen(context.Background(), okRun, OpenOptions{Idle: time.Minute,
					NoPage: time.Minute, Grace: 400 * time.Millisecond, StateFile: state,
					Ready: func(u string) { ready <- u }})
			}()
			out <- res{<-ready, done}
		}()
	}
	a, b := <-out, <-out
	time.Sleep(2 * time.Second)
	live := []string{}
	for _, r := range []res{a, b} {
		select {
		case <-r.done:
		default:
			live = append(live, r.u)
		}
	}
	if len(live) != 1 {
		t.Fatalf("exactly one server must stay, got %d", len(live))
	}
	if s := readState(t, state); !strings.HasSuffix(live[0], ":"+s.Port+"/") {
		t.Errorf("the note must name the live server %s, got %+v", live[0], s)
	}
	if aliveCode(t, old) == 200 {
		t.Error("the oldest page must be retired")
	}
	if _, err := os.Stat(state + ".lock"); !os.IsNotExist(err) {
		t.Error("the lock must be removed when done")
	}
}
