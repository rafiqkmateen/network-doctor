package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ND-262: the too-soon speed note keeps its words for the terminal, and the time it names also goes
// to the page as a timestamp, so the page can show it in the viewer's own time format.
func TestTooSoonNoteAlsoSendsTheTime(t *testing.T) {
	f := healthy()
	last := &SpeedSample{At: time.Now().Add(-3 * time.Minute), Down: 811, Up: 700}
	r := Check(context.Background(), f, Options{LastSpeed: last})
	if r.SpeedRetryAt == nil {
		t.Fatal("speed_retry_at must be set while an earlier speed stands in")
	}
	want := r.Time.Add(time.Duration(r.SpeedWaitSeconds) * time.Second)
	if d := r.SpeedRetryAt.Sub(want); d < -2*time.Second || d > 2*time.Second {
		t.Errorf("retry time %v, want about %v", r.SpeedRetryAt, want)
	}
	var note string
	for _, n := range r.Notes {
		if strings.HasPrefix(n, speedTooSoonNote) {
			note = n
		}
	}
	if note == "" {
		t.Fatalf("the terminal note must still start %q: %v", speedTooSoonNote, r.Notes)
	}
	if !strings.Contains(note, "(at "+r.SpeedRetryAt.Format("1504")+").") {
		t.Errorf("the note and the timestamp must name the same time: %q vs %v", note, r.SpeedRetryAt)
	}
	b, _ := json.Marshal(r)
	if !strings.Contains(string(b), `"speed_retry_at":"`) {
		t.Errorf("the JSON must carry speed_retry_at: %s", b)
	}

	fresh := Check(context.Background(), healthy(), Options{})
	b, _ = json.Marshal(fresh)
	if fresh.SpeedRetryAt != nil || strings.Contains(string(b), "speed_retry_at") {
		t.Error("a newly measured speed has no retry time")
	}
}

// ND-262: the page hides the terminal's too-soon note by its opening words, so the two must match.
func TestPageKnowsTheTooSoonWords(t *testing.T) {
	b, err := os.ReadFile(filepath.Join("..", "web", "index.html"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(b), speedTooSoonNote) {
		t.Errorf("the page must look for the note that starts %q", speedTooSoonNote)
	}
}

// ND-263: the browser opens with the system's own launcher; on Windows one that shows no window.
func TestBrowserCommand(t *testing.T) {
	u := "http://127.0.0.1:50000/"
	cases := map[string][]string{
		"windows": {"rundll32", "url.dll,FileProtocolHandler", u},
		"darwin":  {"open", u},
		"linux":   {"xdg-open", u},
		"freebsd": {"xdg-open", u},
	}
	for goos, want := range cases {
		name, args := browserCommand(goos, u)
		got := append([]string{name}, args...)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("%s: got %v want %v", goos, got, want)
		}
	}
}

// shortOpen makes the open command quit fast and never start a real browser.
func shortOpen(t *testing.T, browser func(string) error) {
	t.Helper()
	ob, oi, on, nb := openBrowser, openIdle, openNoPage, noBrowser
	openBrowser, openIdle, openNoPage = browser, 300*time.Millisecond, 600*time.Millisecond
	noBrowser = printNoBrowser // never a real message box in a test
	t.Cleanup(func() { openBrowser, openIdle, openNoPage, noBrowser = ob, oi, on, nb })
}

// ND-263: open starts the page on this computer at a port the system picks, opens the browser to it,
// and quits by itself with exit code 0 once the page stops checking in.
func TestOpenCommandOpensThePageAndQuitsByItself(t *testing.T) {
	var opened string
	shortOpen(t, func(u string) error {
		opened = u
		go func() {
			if resp, err := http.Get(u); err == nil {
				resp.Body.Close()
			}
		}()
		return nil
	})
	var out bytes.Buffer
	done := make(chan int, 1)
	go func() { done <- Run(context.Background(), []string{"open"}, healthy(), t.TempDir(), &out) }()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code %d, want 0; output %s", code, out.String())
		}
	case <-time.After(10 * time.Second):
		t.Fatal("open must quit by itself when the page stops checking in")
	}
	p, err := url.Parse(opened)
	if err != nil || p.Hostname() != "127.0.0.1" || p.Port() == "" || p.Port() == "8765" {
		t.Errorf("the page must be on 127.0.0.1 at a port the system picks, not 8765: %q", opened)
	}
}

// ND-263: when no browser can be started, the address is printed and it still quits by itself.
func TestOpenPrintsTheAddressWhenNoBrowserStarts(t *testing.T) {
	shortOpen(t, func(string) error { return errors.New("no browser") })
	var out bytes.Buffer
	done := make(chan int, 1)
	go func() { done <- Run(context.Background(), []string{"open"}, healthy(), t.TempDir(), &out) }()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code %d, want 0", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("open must quit by itself when the page never loads")
	}
	if !strings.Contains(out.String(), "http://127.0.0.1:") {
		t.Errorf("the address must be printed when the browser cannot start: %q", out.String())
	}
}

// ND-263: the help names the open command.
func TestHelpNamesOpen(t *testing.T) {
	var out bytes.Buffer
	Run(context.Background(), []string{"help"}, healthy(), t.TempDir(), &out)
	if !strings.Contains(out.String(), "\n  open ") {
		t.Errorf("help must list open: %s", out.String())
	}
}

// One page at a time: while open runs, its port and token are noted in the user's own data folder,
// so the next press can retire it; the note is gone after it quits.
func TestOpenNotesItselfForTheNextPress(t *testing.T) {
	dir := t.TempDir()
	state := filepath.Join(dir, openStateName)
	seen := make(chan bool, 1)
	shortOpen(t, func(u string) error {
		_, err := os.Stat(state)
		seen <- err == nil
		return nil
	})
	var out bytes.Buffer
	code := Run(context.Background(), []string{"open"}, healthy(), dir, &out)
	if code != 0 {
		t.Fatalf("exit code %d; output %s", code, out.String())
	}
	if !<-seen {
		t.Error("open must note its port and token in the data folder before the page opens")
	}
	if _, err := os.Stat(state); !os.IsNotExist(err) {
		t.Error("the note must be removed when open quits")
	}
}
