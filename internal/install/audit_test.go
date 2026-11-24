package install

import (
	"context"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/jobs"
)

// Removing results only removes a folder named NetworkDoctor, never another folder the results
// setting points to, now that every removal deletes them: by the function, by a plain
// `uninstall --silent`, by the old switch and by the page (ND-291). A NetworkDoctor folder
// outside home is refused too.
func TestDeleteResultsOnlyNetworkDoctorFolder(t *testing.T) {
	for _, how := range []string{"function", "silent", "old switch", "page", "outside home"} {
		s := newScratch(t, "linux")
		s.env.ResultsDir = filepath.Join(s.home, "Documents")
		if how == "outside home" {
			s.env.ResultsDir = filepath.Join(s.root, "NetworkDoctor")
		}
		os.MkdirAll(s.env.ResultsDir, 0o755)
		mine := filepath.Join(s.env.ResultsDir, "taxes.txt")
		os.WriteFile(mine, []byte("mine"), 0o644)
		Install(s.env, Choice{}, io.Discard)
		var faults string
		switch how {
		case "function", "outside home":
			if err := Uninstall(s.env, false, io.Discard); err != nil {
				faults = err.Error()
			}
		case "silent", "old switch":
			args := []string{"--silent"}
			if how == "old switch" {
				args = append(args, "--delete-results")
			}
			var out strings.Builder
			if code := Main(t.Context(), "uninstall", args, s.env, &out); code == 0 {
				t.Errorf("%s: exit 0 although the results were not deleted", how)
			}
			if !exists(mine) {
				t.Fatalf("%s: a folder not named NetworkDoctor was deleted", how)
			}
			continue // the silent route says only that not every part went (probRemove)
		case "page":
			base, _, _ := started(t, s, ModeUninstall)
			u, _ := url.Parse(base)
			r := send(t, base, req{method: "POST", path: u.Path + "api/uninstall", body: `{}`, header: true})
			var a answer
			json.NewDecoder(r.Body).Decode(&a)
			faults = strings.Join(a.Lines, " ")
		}
		if !exists(mine) {
			t.Fatalf("%s: a folder not named NetworkDoctor, or outside home, was deleted", how)
		}
		if how != "outside home" && !strings.Contains(faults, probResultsName) {
			t.Errorf("%s: no plain refusal: %q", how, faults)
		}
		if how == "outside home" && faults == "" {
			t.Errorf("%s: the refusal was not reported", how)
		}
	}
	if g := strings.Join(removedLines([]string{probResultsName}), " "); !strings.Contains(g, probResultsName) {
		t.Errorf("the page does not say why: %q", g)
	}
}

// Pressing OK on the no-browser box waits for a running install to finish before the page stops.
func TestNoBrowserOKWaitsForWork(t *testing.T) {
	s := newScratch(t, "linux")
	release := make(chan struct{})
	var once sync.Once
	applied := make(chan struct{})
	inner := s.env.Apply
	s.env.Apply = func(g, h string, p jobs.Plan, r jobs.Runner) error {
		once.Do(func() { close(applied) })
		<-release
		return inner(g, h, p, r)
	}
	got := make(chan string, 1)
	okPressed := make(chan struct{})
	s.env.OpenURL = func(string) error { return os.ErrNotExist }
	s.env.NoBrowser = func(u string, _ io.Writer) bool { got <- u; <-okPressed; return true }
	done := make(chan struct{})
	go func() { defer close(done); Serve(context.Background(), s.env, ModeInstall, io.Discard) }()
	base := <-got
	u, _ := url.Parse(base)
	resp := make(chan struct{})
	go func() {
		send(t, base, req{method: "POST", path: u.Path + "api/install", body: `{}`, header: true})
		close(resp)
	}()
	<-applied
	close(okPressed)
	select {
	case <-done:
		t.Fatal("the page stopped in the middle of the install")
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	<-resp
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("the page did not stop after the install finished")
	}
}

// The do-it-yourself Ookla line sends nobody into a folder inside the app.
func TestOoklaSelfLine(t *testing.T) {
	if strings.Contains(ooklaSelf, "folder") || !strings.Contains(ooklaSelf, "tick both boxes") ||
		!strings.Contains(ooklaSelf, "setup again") || banned(ooklaSelf, true) != "" {
		t.Errorf("Ookla line: %q", ooklaSelf)
	}
}

// A repair run with the Ookla boxes left empty keeps ookla true while the tool is still there.
func TestRepairKeepsOoklaWhenThere(t *testing.T) {
	s := newScratch(t, "linux")
	Install(s.env, Choice{}, io.Discard)
	l := LayoutFor(s.env)
	os.MkdirAll(filepath.Join(l.ProgramDir, "ookla"), 0o755)
	os.WriteFile(filepath.Join(l.ProgramDir, "ookla", "speedtest"), []byte("x"), 0o755)
	Install(s.env, Choice{Weekly: true}, io.Discard)
	if !choicesIn(t, l.ProgramDir)["ookla"] {
		t.Error("ookla turned false while the tool is there")
	}
}

// A Desktop that is a link to a place outside home is never written through.
func TestLinkedFolderOutsideHomeRefused(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		s := newScratch(t, goos)
		outside := filepath.Join(s.root, "outside")
		os.MkdirAll(outside, 0o755)
		if err := os.Symlink(outside, filepath.Join(s.home, "Desktop")); err != nil {
			t.Skipf("this system cannot make a link here (%v), so the check cannot run", err)
		}
		s.env.Symlink = os.Symlink
		Install(s.env, Choice{}, io.Discard)
		Uninstall(s.env, false, io.Discard)
		left, _ := os.ReadDir(outside)
		if len(left) != 0 {
			t.Errorf("%s: written outside home through a link: %v", goos, left)
		}
	}
}

// After a failed install the page still says the end screen was shown, so the server stops.
func TestPageReportsShownAfterFailure(t *testing.T) {
	page := string(pageHTML)
	i := strings.Index(page, "function end(")
	j := strings.Index(page[i:], "post('api/shown')")
	k := strings.Index(page[i:], "if (ok) { post('api/shown')")
	if i < 0 || j < 0 || k >= 0 {
		t.Error("the end screen tells the server only after a success")
	}
	s := newScratch(t, "linux")
	s.p.applyErr = os.ErrPermission
	s.env.Grace = 50 * time.Millisecond
	base, _, done := started(t, s, ModeInstall)
	u, _ := url.Parse(base)
	send(t, base, req{method: "POST", path: u.Path + "api/install", body: `{}`, header: true})
	send(t, base, req{method: "POST", path: u.Path + "api/shown", header: true})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("did not stop after a failed install was shown")
	}
}

// With nothing installed the removal page says so itself, and stops once that is shown.
func TestRemovalPageSaysNotInstalled(t *testing.T) {
	s := newScratch(t, "linux")
	s.env.Grace = 50 * time.Millisecond
	base, _, done := started(t, s, ModeUninstall)
	u, _ := url.Parse(base)
	res := send(t, base, req{method: "GET", path: u.Path + "api/texts"})
	var b strings.Builder
	io.Copy(&b, res.Body)
	if !strings.Contains(b.String(), `"installed":false`) || !strings.Contains(b.String(), unNotThere) {
		t.Errorf("texts: %s", b.String())
	}
	send(t, base, req{method: "POST", path: u.Path + "api/shown", header: true})
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("did not stop")
	}
}

// After the wait, the Finder request is stopped before the link is made.
func TestFinderStoppedBeforeLink(t *testing.T) {
	s := newScratch(t, "darwin")
	s.env.AliasWait = 100 * time.Millisecond
	var mu sync.Mutex
	var order []string
	s.env.RunContext = func(ctx context.Context, name string, args ...string) (string, error) {
		if name != "osascript" {
			return s.p.run(name, args...)
		}
		<-ctx.Done()
		mu.Lock()
		order = append(order, "finder stopped")
		mu.Unlock()
		return "", ctx.Err()
	}
	s.env.Symlink = func(target, link string) error {
		mu.Lock()
		order = append(order, "link")
		mu.Unlock()
		return os.WriteFile(link, []byte("link to "+target), 0o644)
	}
	Install(s.env, Choice{}, io.Discard)
	if strings.Join(order, ",") != "finder stopped,link" {
		t.Errorf("order: %v", order)
	}
}
