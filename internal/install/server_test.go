package install

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// started runs the setup page server and returns its address, a stop function and a channel
// that closes when the server has quit.
func started(t *testing.T, s *scratch, mode Mode) (string, context.CancelFunc, chan struct{}) {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	got := make(chan string, 1)
	s.env.OpenURL = func(u string) error { got <- u; return nil }
	done := make(chan struct{})
	go func() {
		defer close(done)
		Serve(ctx, s.env, mode, io.Discard)
	}()
	select {
	case u := <-got:
		t.Cleanup(func() { cancel(); <-done })
		return u, cancel, done
	case <-time.After(5 * time.Second):
		cancel()
		t.Fatal("the page was never offered to the browser")
	}
	return "", cancel, done
}

type req struct {
	method, path, host, origin, site, body string
	header                                 bool
}

func send(t *testing.T, base string, r req) *http.Response {
	t.Helper()
	u, _ := url.Parse(base)
	full := u.Scheme + "://" + u.Host + r.path
	hr, err := http.NewRequest(r.method, full, strings.NewReader(r.body))
	if err != nil {
		t.Fatal(err)
	}
	if r.host != "" {
		hr.Host = r.host
	}
	if r.origin != "" {
		hr.Header.Set("Origin", r.origin)
	}
	if r.site != "" {
		hr.Header.Set("Sec-Fetch-Site", r.site)
	}
	if r.header {
		hr.Header.Set("X-NetDoctor", "1")
	}
	res, err := http.DefaultClient.Do(hr)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { res.Body.Close() })
	return res
}

func framed(res *http.Response) bool {
	return res.Header.Get("X-Frame-Options") == "DENY" &&
		strings.Contains(res.Header.Get("Content-Security-Policy"), "frame-ancestors 'none'") &&
		res.Header.Get("Referrer-Policy") == "no-referrer"
}

// ND-271: the page is served on 127.0.0.1 at a port the system picks, two at once on two ports.
func TestInstallServesItsPageOnAFreeLocalPort(t *testing.T) {
	a, b := newScratch(t, "linux"), newScratch(t, "darwin")
	ua, _, _ := started(t, a, ModeInstall)
	ub, _, _ := started(t, b, ModeInstall)
	pa, _ := url.Parse(ua)
	pb, _ := url.Parse(ub)
	if pa.Hostname() != "127.0.0.1" || pb.Hostname() != "127.0.0.1" || pa.Port() == pb.Port() || pa.Port() == "" {
		t.Fatalf("addresses %s and %s", ua, ub)
	}
	if !regexp.MustCompile(`^/[0-9a-f]{32}/$`).MatchString(pa.Path) {
		t.Errorf("no token in the address: %s", ua)
	}
	res := send(t, ua, req{method: "GET", path: pa.Path, site: "none"})
	body, _ := io.ReadAll(res.Body)
	if res.StatusCode != 200 || !strings.Contains(string(body), "<html") || !framed(res) {
		t.Errorf("page: %d %v", res.StatusCode, res.Header)
	}
	res = send(t, ua, req{method: "GET", path: pa.Path + "api/texts"})
	var tx PageTexts
	if json.NewDecoder(res.Body).Decode(&tx) != nil || tx.Mode != "install" || len(tx.Parts) != 3 {
		t.Errorf("texts: %+v", tx)
	}
}

// ND-272: strangers are refused and change nothing; every answer carries the frame headers.
func TestInstallPageRefusesStrangers(t *testing.T) {
	for _, mode := range []Mode{ModeInstall, ModeUninstall} {
		s := newScratch(t, "linux")
		Install(s.env, Choice{}, io.Discard)
		s.p.plans, s.p.removes = nil, 0
		base, _, _ := started(t, s, mode)
		u, _ := url.Parse(base)
		tok, host := u.Path, u.Host
		api := tok + "api/install"
		body := `{"weekly":true}`
		if mode == ModeUninstall {
			api, body = tok+"api/uninstall", `{}`
		}
		own := "http://" + host
		bad := []req{
			{method: "GET", path: "/"},
			{method: "GET", path: "/0123456789abcdef0123456789abcdef/"},
			{method: "GET", path: tok, host: "evil.example:" + u.Port()},
			{method: "GET", path: tok, host: "localhost:1"},
			{method: "POST", path: api, body: body},
			{method: "POST", path: api, body: body, header: true, origin: "http://evil.example"},
			{method: "POST", path: api, body: body, header: true, origin: "http://localhost:1"},
			{method: "POST", path: api, body: body, header: true, site: "cross-site"},
			{method: "POST", path: api, body: body, header: true, site: "same-site"},
			{method: "GET", path: api},
			{method: "PUT", path: api, body: body, header: true},
			{method: "POST", path: "/x" + api[2:], body: body, header: true},
			{method: "GET", path: tok, site: "cross-site"},
		}
		for i, r := range bad {
			res := send(t, base, r)
			if res.StatusCode < 400 {
				t.Errorf("%v case %d (%+v): answered %d", mode, i, r, res.StatusCode)
			}
			if !framed(res) {
				t.Errorf("%v case %d: frame headers missing: %v", mode, i, res.Header)
			}
		}
		if len(s.p.plans) != 0 || s.p.removes != 0 {
			t.Fatalf("%v: a refused request changed something", mode)
		}
		res := send(t, base, req{method: "POST", path: api, body: body, header: true, origin: own, site: "same-origin"})
		if res.StatusCode != 200 || !framed(res) {
			t.Errorf("%v: the page's own request: %d", mode, res.StatusCode)
		}
		if len(s.p.plans)+s.p.removes != 1 {
			t.Errorf("%v: the page's own request did nothing", mode)
		}
		res = send(t, base, req{method: "POST", path: api, body: body, header: true, origin: own})
		if res.StatusCode < 400 {
			t.Errorf("%v: a second run on the same page was taken", mode)
		}
	}
}

// ND-272, ND-315: the page loads nothing from outside the computer.
func TestInstallPageLoadsNothingFromOutside(t *testing.T) {
	page := string(pageHTML)
	if regexp.MustCompile(`(?i)https?:|//[a-z0-9]|src\s*=|<link|url\(|@import`).MatchString(page) {
		t.Error("the page source refers to something outside")
	}
	s := newScratch(t, "linux")
	base, _, _ := started(t, s, ModeInstall)
	u, _ := url.Parse(base)
	res := send(t, base, req{method: "GET", path: u.Path})
	csp := res.Header.Get("Content-Security-Policy")
	for _, d := range []string{"default-src 'none'", "connect-src 'self'", "form-action 'none'"} {
		if !strings.Contains(csp, d) {
			t.Errorf("policy lacks %q: %s", d, csp)
		}
	}
	// The only outside addresses are Ookla's license and terms links, opened only when pressed,
	// with no address of this page passed on.
	b, _ := json.Marshal(pageTexts("linux", Layout{}))
	for _, m := range regexp.MustCompile(`https?://[^"\\]+`).FindAllString(string(b), -1) {
		if !strings.HasPrefix(m, "https://www.speedtest.net/") {
			t.Errorf("outside address %s", m)
		}
	}
	if !strings.Contains(page, `rel="noreferrer noopener"`) {
		t.Error("links pass this page's address on")
	}
}

// ND-272: the server quits after the finish screen is shown, and after the quiet time.
func TestInstallPageQuitsByItself(t *testing.T) {
	s := newScratch(t, "linux")
	s.env.Quiet = 300 * time.Millisecond
	_, _, done := started(t, s, ModeInstall)
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("did not quit after the quiet time")
	}

	s2 := newScratch(t, "linux")
	s2.env.Grace = 50 * time.Millisecond
	base, _, done2 := started(t, s2, ModeInstall)
	u, _ := url.Parse(base)
	send(t, base, req{method: "POST", path: u.Path + "api/install", body: `{}`, header: true})
	send(t, base, req{method: "POST", path: u.Path + "api/shown", header: true})
	select {
	case <-done2:
	case <-time.After(5 * time.Second):
		t.Fatal("did not quit after the finish screen")
	}
}

// ND-272, ND-279: the guide route answers the installed guide as plain text, only by GET, only
// after the install on that page has finished well, and passes every guard the other routes pass.
func TestInstallPageServesTheGuideOnlyAfterInstall(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		s := newScratch(t, goos)
		os.WriteFile(filepath.Join(s.setup, "GUIDE.md"), []byte("# Guide\nThe whole guide.\n"), 0o644)
		base, _, _ := started(t, s, ModeInstall)
		u, _ := url.Parse(base)
		tok, guide := u.Path, u.Path+"guide"
		if res := send(t, base, req{method: "GET", path: guide}); res.StatusCode != http.StatusNotFound || !framed(res) {
			t.Errorf("%s: before the install the guide answered %d", goos, res.StatusCode)
		}
		send(t, base, req{method: "POST", path: tok + "api/install", body: `{}`, header: true})
		// The route reads the guide beside the installed program, not the one in the setup folder.
		installed := filepath.Join(LayoutFor(s.env).ProgramDir, "GUIDE.md")
		if err := os.WriteFile(installed, []byte("# Installed guide\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		res := send(t, base, req{method: "GET", path: guide, site: "same-origin"})
		body, _ := io.ReadAll(res.Body)
		if res.StatusCode != 200 || string(body) != "# Installed guide\n" || !framed(res) ||
			!strings.HasPrefix(res.Header.Get("Content-Type"), "text/plain") ||
			res.Header.Get("X-Content-Type-Options") != "nosniff" {
			t.Errorf("%s: guide after the install: %d %q %v", goos, res.StatusCode, body, res.Header)
		}
		for i, r := range []req{
			{method: "GET", path: "/guide"},
			{method: "GET", path: "/0123456789abcdef0123456789abcdef/guide"},
			{method: "GET", path: guide, host: "evil.example:" + u.Port()},
			{method: "GET", path: guide, site: "cross-site"},
			{method: "GET", path: guide, origin: "http://evil.example"},
			{method: "POST", path: guide, header: true},
		} {
			if res := send(t, base, r); res.StatusCode < 400 || !framed(res) {
				t.Errorf("%s case %d (%+v): answered %d", goos, i, r, res.StatusCode)
			}
		}
	}
	// A failed install offers no guide, and the removal page has none.
	s := newScratch(t, "linux")
	s.p.applyErr = errors.New("simulated failure")
	base, _, _ := started(t, s, ModeInstall)
	u, _ := url.Parse(base)
	send(t, base, req{method: "POST", path: u.Path + "api/install", body: `{"weekly":true}`, header: true})
	if res := send(t, base, req{method: "GET", path: u.Path + "guide"}); res.StatusCode != http.StatusNotFound {
		t.Errorf("after a failed install the guide answered %d", res.StatusCode)
	}
	r := newScratch(t, "linux")
	Install(r.env, Choice{}, io.Discard)
	base, _, _ = started(t, r, ModeUninstall)
	u, _ = url.Parse(base)
	if res := send(t, base, req{method: "GET", path: u.Path + "guide"}); res.StatusCode != http.StatusNotFound {
		t.Errorf("the removal page answered the guide with %d", res.StatusCode)
	}
}

// ND-279: the finish screen fetches the guide before it tells the server the screen is shown (the
// server quits soon after), and shows the button only when that fetch worked.
func TestFinishPageOffersTheGuide(t *testing.T) {
	page := string(pageHTML)
	for _, want := range []string{`id="guideOpen"`, `id="guideText"`, `fetch('guide'`} {
		if !strings.Contains(page, want) {
			t.Errorf("the page lacks %s", want)
		}
	}
	g, shown := strings.Index(page, "fetch('guide'"), strings.Index(page, "post('api/shown')")
	if g < 0 || shown < 0 || g > shown {
		t.Error("the guide must be fetched before the page posts api/shown")
	}
	// The call order: a good install waits for the guide, then shows the finish screen (end posts
	// api/shown); the install's answer reaches end only through that path or as a failure.
	install := page[strings.Index(page, "post('api/install'"):strings.Index(page, "function setupUninstall")]
	if !regexp.MustCompile(`return loadGuide\(\)\.then\(function \(\) \{ end\(a\.lines, true`).MatchString(install) ||
		strings.Count(install, "end(") != 2 || !strings.Contains(install, "if (!a.ok) { end(a.lines, false); return; }") {
		t.Error("a good install must call end only after loadGuide has finished")
	}
	if !regexp.MustCompile(`id="guideOpen"[^>]*\bhidden\b`).MatchString(page) {
		t.Error("the guide button must start hidden and show only after the guide arrived")
	}
	for _, goos := range []string{"darwin", "linux"} {
		tx := pageTexts(goos, Layout{})
		if tx.GuideLine != "The full guide explains every part." || tx.GuideOpen != "Open the guide" {
			t.Errorf("%s: guide words %q %q", goos, tx.GuideLine, tx.GuideOpen)
		}
	}
}

// ND-279: long folder names wrap inside the card on every setup page.
func TestSetupPagesWrapLongFolderNames(t *testing.T) {
	for name, page := range map[string]string{"page.html": string(pageHTML), "macopen.html": repoFile(t,
		"internal/install/macopen.html")} {
		if !regexp.MustCompile(`body\{[^}]*overflow-wrap:anywhere`).MatchString(page) {
			t.Errorf("%s: body does not wrap long words (overflow-wrap:anywhere)", name)
		}
	}
}

// ND-301: the Ookla step starts unticked, needs the second tick, and fetches nothing without both.
func TestInstallPageOoklaStep(t *testing.T) {
	page := string(pageHTML)
	for _, id := range []string{"ooklaOffer", "ooklaAgree"} {
		m := regexp.MustCompile(`<input[^>]*id="` + id + `"[^>]*>`).FindString(page)
		if m == "" || strings.Contains(m, "checked") {
			t.Errorf("%s box: %q", id, m)
		}
	}
	s := newScratch(t, "linux")
	s.env.HTTP = failingClient(t)
	base, _, _ := started(t, s, ModeInstall)
	u, _ := url.Parse(base)
	res := send(t, base, req{method: "POST", path: u.Path + "api/install", body: `{"ookla_offer":true}`, header: true})
	if res.StatusCode != http.StatusBadRequest || len(s.p.plans) != 0 {
		t.Errorf("offer without the license tick: %d", res.StatusCode)
	}
	tx := pageTexts("linux", Layout{}).Ookla
	if tx.EulaURL == "" || tx.TermsURL == "" || tx.Agree == "" {
		t.Error("license links or second tick missing")
	}
}
