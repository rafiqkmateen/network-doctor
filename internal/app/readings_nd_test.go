package app

// Checks for the round-two readings (ND-231, ND-232, ND-233, ND-239, ND-240, the VPN sentence of
// ND-241), written before the build. They go through Check and Run
// with a pretend machine that also has the newer reads (collect.Extras).
// The helpers here (nd...) are shared by speedrecheck_nd_test.go and fixoffers_nd_test.go.

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// ndSource is a pretend machine with the newer reads. It counts every sign-in request and
// every second speed test, so a test can prove each is made once at most.
type ndSource struct {
	*fake
	wifi     collect.WiFiInfo
	wifiErr  error
	dnsFails bool

	probe     collect.ProbeResult
	probeErr  error
	dateShift time.Duration // when set, the Date header is the time of the request plus this
	noDate    bool
	probes    atomic.Int32

	cf      collect.SpeedPair
	cfErr   error
	cfCalls atomic.Int32
	region  string

	country      string // what the speed host's tiny country answer says
	countryCalls atomic.Int32
}

func (s *ndSource) WiFi(context.Context) (collect.WiFiInfo, error) { return s.wifi, s.wifiErr }
func (s *ndSource) DNSWorks(context.Context) bool                  { return !s.dnsFails }
func (s *ndSource) Probe(context.Context) (collect.ProbeResult, error) {
	s.probes.Add(1)
	r := s.probe
	switch {
	case s.noDate:
		r.Date = time.Time{}
	case s.dateShift != 0:
		r.Date = time.Now().Add(s.dateShift)
	}
	return r, s.probeErr
}
func (s *ndSource) SpeedCloudflare(context.Context) (collect.SpeedPair, error) {
	s.cfCalls.Add(1)
	return s.cf, s.cfErr
}
func (s *ndSource) Region(context.Context) string { return s.region }
func (s *ndSource) SpeedCountry(context.Context) string {
	s.countryCalls.Add(1)
	return s.country
}

var _ collect.Extras = (*ndSource)(nil)

// ndExpected is an answer the source marks as the expected one (no sign-in page in the way).
func ndExpected() collect.ProbeResult {
	return collect.ProbeResult{Status: 200, Body: "Microsoft Connect Test", Expected: true}
}

func ndRedirect() collect.ProbeResult {
	return collect.ProbeResult{Status: 302, Location: "http://login.hotel-wifi.example/portal?mac=00"}
}

// ndWiFi is a healthy Wi-Fi machine whose details were read, with no sign-in page in the way.
func ndWiFi() *ndSource {
	f := healthy()
	f.adapter.Kind = core.WiFi
	f.adapter.WiFiJoined = true
	f.adapter.LinkMbps = 866
	return &ndSource{fake: f, wifi: collect.WiFiInfo{SignalPct: 80, Band: "5 GHz", Channel: 36}, probe: ndExpected()}
}

// ndNoLocationAsk fails when any text in the report tells the person to turn on Location.
func ndNoLocationAsk(t *testing.T, where string, rep Report) {
	t.Helper()
	b, _ := json.Marshal(rep)
	low := strings.ToLower(string(b))
	if strings.Contains(low, "turn on") && strings.Contains(low, "location") {
		t.Errorf("%s: the report tells the person to turn on Location:\n%s", where, b)
	}
}

const (
	ndSignInHeadline = "This Wi-Fi needs you to sign in through a web browser."
	ndSignInFirst    = "Open a web browser and look for the sign-in page."
)

func ndCheckSignIn(t *testing.T, where string, r Report) {
	t.Helper()
	if r.SignIn == nil {
		t.Errorf("%s: sign_in must be set", where)
		return
	}
	if r.SignIn.Headline != ndSignInHeadline || strings.TrimSpace(r.SignIn.Why) == "" {
		t.Errorf("%s: sign_in words: %+v", where, *r.SignIn)
	}
	if !r.Reading.SignIn {
		t.Errorf("%s: reading.sign_in must be true", where)
	}
	if r.Finding.Cause != core.CauseSignIn || r.Finding.First != ndSignInFirst {
		t.Errorf("%s: the finding must name the sign-in page: %+v", where, r.Finding)
	}
	low := strings.ToLower(r.Finding.Summary)
	if strings.Contains(low, "router") || strings.Contains(low, "provider") {
		t.Errorf("%s: the finding must not blame the router or the provider: %q", where, r.Finding.Summary)
	}
	ndNoLocationAsk(t, where, r)
}

// ND-231: a redirect is a sign-in page, even when the sign-in page blocks pings to the internet
// (the usual hotel case), in a full and a quick check, with one request each time.
func TestND231SignInPageFound(t *testing.T) {
	for _, quick := range []bool{false, true} {
		s := ndWiFi()
		s.netP = collect.PingResult{LossPct: 100}
		s.probe = ndRedirect()
		r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: quick})
		where := "pings blocked, quick=" + map[bool]string{false: "no", true: "yes"}[quick]
		ndCheckSignIn(t, where, r)
		if n := s.probes.Load(); n != 1 {
			t.Errorf("%s: one sign-in request per check, got %d", where, n)
		}
	}
	// Online, and the answer is a page that is not the expected one.
	s := ndWiFi()
	s.probe = collect.ProbeResult{Status: 200, Body: "<html><title>Welcome to Hotel Wi-Fi</title><form>Room number</form></html>"}
	r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	ndCheckSignIn(t, "online, other page", r)
}

// ND-231: nothing is said when the expected answer comes back, when the request cannot be made,
// or on a machine without the newer reads.
func TestND231QuietWhenNoSignInPage(t *testing.T) {
	s := ndWiFi()
	r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	if r.SignIn != nil || r.Reading.SignIn || r.Finding.Cause == core.CauseSignIn {
		t.Errorf("expected answer: nothing to say, got %+v / %+v", r.SignIn, r.Finding)
	}
	if n := s.probes.Load(); n != 1 {
		t.Errorf("one sign-in request per check, got %d", n)
	}
	s = ndWiFi()
	s.probe = collect.ProbeResult{}
	s.probeErr = context.DeadlineExceeded
	r = Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	if r.SignIn != nil || r.Finding.Cause == core.CauseSignIn {
		t.Errorf("request could not be made: nothing to say, got %+v / %+v", r.SignIn, r.Finding)
	}
	// No status came back (no error either): nothing to say.
	s = ndWiFi()
	s.probe = collect.ProbeResult{}
	if r = Check(context.Background(), s, Options{PlanMbps: 300, Quick: true}); r.SignIn != nil || r.Finding.Cause == core.CauseSignIn {
		t.Errorf("no status: nothing to say, got %+v / %+v", r.SignIn, r.Finding)
	}
	// The source says the answer was the expected one: the check trusts it, even for a redirect.
	s = ndWiFi()
	s.probe = collect.ProbeResult{Status: 302, Location: "http://www.msftconnecttest.com/redirect", Expected: true}
	if r = Check(context.Background(), s, Options{PlanMbps: 300, Quick: true}); r.SignIn != nil || r.Finding.Cause == core.CauseSignIn {
		t.Errorf("expected answer marked by the source: nothing to say, got %+v / %+v", r.SignIn, r.Finding)
	}
	r = Check(context.Background(), healthy(), Options{PlanMbps: 300, Quick: true})
	if r.SignIn != nil || r.Clock != nil || r.VPNHint != nil || r.SpeedRecheck != nil {
		t.Error("a machine without the newer reads gets none of the new notes")
	}
}

// ND-231: the command exits 14 and the JSON carries sign_in.
func TestND231RunExits14(t *testing.T) {
	s := ndWiFi()
	s.netP = collect.PingResult{LossPct: 100}
	s.probe = ndRedirect()
	var out bytes.Buffer
	code := Run(context.Background(), []string{"check", "--json", "--quick"}, s, t.TempDir(), &out)
	if code != 14 {
		t.Errorf("exit code %d, want 14", code)
	}
	var raw map[string]any
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	si, ok := raw["sign_in"].(map[string]any)
	if !ok || si["headline"] != ndSignInHeadline {
		t.Errorf("JSON sign_in: %v", raw["sign_in"])
	}
	out.Reset()
	Run(context.Background(), []string{"check", "--json", "--quick"}, ndWiFi(), t.TempDir(), &out)
	if strings.Contains(out.String(), `"sign_in"`) || strings.Contains(out.String(), `"clock"`) {
		t.Errorf("no sign-in page and a right clock: no sign_in or clock key:\n%s", out.String())
	}
}

// ND-232: the clock note appears only when the Date header differs by more than 5 minutes,
// ahead or behind. The Date header has whole seconds and Check reads the clock itself, so the
// edge is tested with a 5-second margin each side (an exact 5:00 needs a clock seam).
func TestND232ClockCompare(t *testing.T) {
	cases := []struct {
		name  string
		shift time.Duration
		want  bool
	}{
		{"server 4m55s ahead", 4*time.Minute + 55*time.Second, false},
		{"server 5m05s ahead", 5*time.Minute + 5*time.Second, true},
		{"server 4m55s behind", -(4*time.Minute + 55*time.Second), false},
		{"server 5m05s behind", -(5*time.Minute + 5*time.Second), true},
		{"server 12 minutes behind", -12 * time.Minute, true},
		{"server 40 seconds ahead", 40 * time.Second, false},
	}
	for _, c := range cases {
		s := ndWiFi()
		s.dateShift = c.shift
		r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
		if (r.Clock != nil) != c.want {
			t.Errorf("%s: clock note %v, want %v", c.name, r.Clock, c.want)
			continue
		}
		if r.Clock == nil {
			continue
		}
		low := strings.ToLower(r.Clock.Headline)
		if !strings.Contains(low, "minute") || !strings.Contains(low, "clock") {
			t.Errorf("%s: headline must say the clock is wrong by about so many minutes: %q", c.name, r.Clock.Headline)
		}
		why := strings.ToLower(r.Clock.Why)
		if !strings.Contains(why, "website") || !strings.Contains(why, "sign") {
			t.Errorf("%s: the reason must say websites and sign-ins fail: %q", c.name, r.Clock.Why)
		}
		if c.shift == -12*time.Minute && !strings.Contains(r.Clock.Headline, "12") {
			t.Errorf("%s: headline must give about 12 minutes: %q", c.name, r.Clock.Headline)
		}
		ndNoLocationAsk(t, c.name, r)
	}
	s := ndWiFi()
	s.noDate = true
	if r := Check(context.Background(), s, Options{Quick: true}); r.Clock != nil {
		t.Error("no Date header: nothing to say")
	}
	s = ndWiFi()
	s.dateShift = -time.Hour
	s.probeErr = context.DeadlineExceeded
	if r := Check(context.Background(), s, Options{Quick: true}); r.Clock != nil {
		t.Error("no answer: nothing to say")
	}
}

// ND-233: Wi-Fi link speed under 20 megabits is slow, with the speed and the exact advice;
// 20 is not; a speed of 0 (not read) never is. A signal of 50 percent is weak; 0 never is.
func TestND233SlowLinkAndWeakSignalThroughCheck(t *testing.T) {
	const move = "Move closer to the router, or try the 5 GHz band."
	advice := func(link, signal int) []string {
		s := ndWiFi()
		s.adapter.LinkMbps = link
		s.wifi.SignalPct = signal
		r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
		ndNoLocationAsk(t, "Wi-Fi advice", r)
		return r.LinkAdvice
	}
	has := func(list []string, words ...string) bool {
		for _, a := range list {
			ok := true
			for _, w := range words {
				ok = ok && strings.Contains(a, w)
			}
			if ok {
				return true
			}
		}
		return false
	}
	if a := advice(19, 80); !has(a, "19", move) {
		t.Errorf("link 19 megabits: must say slow with the speed and %q: %v", move, a)
	}
	if a := advice(20, 80); has(a, move) {
		t.Errorf("link 20 megabits is not slow: %v", a)
	}
	if a := advice(0, 80); has(a, move) {
		t.Errorf("link not read (0) is never slow: %v", a)
	}
	weak := func(a []string) bool {
		for _, s := range a {
			if strings.Contains(strings.ToLower(s), "weak") {
				return true
			}
		}
		return false
	}
	if !weak(advice(866, 50)) {
		t.Error("signal 50 percent (-75 dBm) must count as weak")
	}
	if weak(advice(866, 51)) {
		t.Error("signal 51 percent is not weak")
	}
	if weak(advice(866, 0)) {
		t.Error("signal not read (0) is never weak")
	}
}

// ND-239: Wi-Fi joined, up, with no usable address: the exact finding, never the cable.
func TestND239NoAddressThroughCheck(t *testing.T) {
	s := ndWiFi()
	s.adapter.NoAddress = true
	s.adapter.IP = "169.254.20.7"
	s.adapter.Gateway = ""
	s.netP = collect.PingResult{LossPct: 100}
	s.probeErr = context.DeadlineExceeded
	r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	if r.Finding.Summary != "The Wi-Fi is connected but got no address from the router." || r.Finding.First != "Restart the router, or press the button that asks it for a new address." {
		t.Errorf("finding: %+v", r.Finding)
	}
	if strings.Contains(strings.ToLower(r.Finding.Summary+r.Finding.First), "cable") {
		t.Errorf("must never say the cable has no link: %+v", r.Finding)
	}
	if !r.Reading.NoAddress {
		t.Error("reading.no_address must be true")
	}
	ndNoLocationAsk(t, "no address", r)
}

// ND-240: the driver's age in days shows when it was read; nothing about age when not.
func TestND240DriverAgeThroughCheck(t *testing.T) {
	row := func(age int) core.Advice {
		s := ndWiFi()
		s.settings = core.Settings{DNS: []string{"192.168.1.1"}, DriverVersion: "22.1.0.3", DriverAgeDays: age}
		s.adapter.DriverAgeDays = age
		r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
		for _, a := range r.Settings {
			if a.Area == "Driver" {
				return a
			}
		}
		t.Fatal("no Driver row")
		return core.Advice{}
	}
	if a := row(400); !strings.Contains(a.Finding+" "+a.Effect, "400") {
		t.Errorf("age read: must give 400 days: %+v", a)
	}
	a := row(0)
	low := strings.ToLower(a.Finding + " " + a.Effect)
	for _, w := range []string{"recent", " old", "day", "year"} {
		if strings.Contains(low, w) {
			t.Errorf("age not read: nothing about age, found %q in %+v", w, a)
		}
	}
}

// ND-241: the VPN line in the check.
func TestND241VPNSentenceThroughCheck(t *testing.T) {
	const want = "No VPN program is running on this computer. A VPN on your router cannot be seen from here, and it can change the speed and the provider shown."
	r := Check(context.Background(), ndWiFi(), Options{PlanMbps: 300, Quick: true})
	for _, a := range r.Settings {
		if a.Area == "VPN" {
			if !strings.Contains(a.Finding+" "+a.Effect, want) {
				t.Errorf("VPN line: %+v", a)
			}
			return
		}
	}
	t.Fatal("no VPN row")
}

// Plain wording (project rule 4): every new text the person reads is at grade 8 or below,
// measured the same way as the setup texts (core.GradeLevel). Texts come from the scenarios above.
func TestNDNewTextsArePlain(t *testing.T) {
	var texts []string
	add := func(n *Note) {
		if n != nil {
			texts = append(texts, n.Headline, n.Why)
		}
	}
	s := ndWiFi()
	s.netP = collect.PingResult{LossPct: 100}
	s.probe = ndRedirect()
	r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	add(r.SignIn)
	texts = append(texts, r.Finding.Summary, r.Finding.First)

	s = ndWiFi()
	s.dateShift = -12 * time.Minute
	add(Check(context.Background(), s, Options{PlanMbps: 300, Quick: true}).Clock)

	s = ndSpeed(10, 100, collect.SpeedPair{Down: 280, Up: 95, Country: "NL"})
	s.region = "US"
	r = Check(context.Background(), s, Options{PlanMbps: 300})
	add(r.VPNHint)
	if r.SpeedRecheck != nil {
		texts = append(texts, r.SpeedRecheck.Headline, r.SpeedRecheck.Why)
	}
	s = ndSpeed(10, 100, collect.SpeedPair{})
	s.cfErr = context.DeadlineExceeded
	if rc := Check(context.Background(), s, Options{PlanMbps: 300}).SpeedRecheck; rc != nil {
		texts = append(texts, rc.Headline, rc.Why)
	}

	s = ndWiFi()
	s.adapter.NoAddress, s.adapter.IP, s.adapter.Gateway = true, "169.254.1.2", ""
	s.netP = collect.PingResult{LossPct: 100}
	s.probeErr = context.DeadlineExceeded
	p := newNDFixPC()
	ndWithFixSystem(t, p)
	r = Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	texts = append(texts, r.Finding.Summary, r.Finding.First)
	for _, o := range r.FixOffers {
		texts = append(texts, o.Title, o.After)
	}
	s = ndWiFi()
	s.dnsFails = true
	for _, o := range Check(context.Background(), s, Options{PlanMbps: 300, Quick: true}).FixOffers {
		texts = append(texts, o.Title, o.After)
	}
	s = ndWiFi()
	s.routerP = collect.PingResult{LossPct: 100}
	s.netP = collect.PingResult{LossPct: 100}
	s.probeErr = context.DeadlineExceeded
	for _, o := range Check(context.Background(), s, Options{PlanMbps: 300, Quick: true}).FixOffers {
		texts = append(texts, o.Title, o.After)
	}

	for _, txt := range texts {
		if strings.TrimSpace(txt) == "" {
			continue
		}
		if g := core.GradeLevel(txt); g > 8.0 {
			t.Errorf("grade %.1f too high: %q", g, txt)
		}
	}
	// The fixed words in the requirements must pass too, or the rule cannot be met.
	for _, txt := range []string{ndSignInHeadline, ndSignInFirst,
		"The Wi-Fi is connected but got no address from the router.", "Restart the router, or press the button that asks it for a new address.",
		"Move closer to the router, or try the 5 GHz band.", "Windows did not share the Wi-Fi details.",
		"No VPN program is running on this computer. A VPN on your router cannot be seen from here, and it can change the speed and the provider shown."} {
		if g := core.GradeLevel(txt); g > 8.0 {
			t.Errorf("fixed requirement words score grade %.1f: %q", g, txt)
		}
	}
}

// ND-242: when only the failed netsh call and the consent value say the details are hidden, the
// report carries the neutral words, which name no setting; with the English refusal the old words stay.
func TestND242NeutralHiddenWords(t *testing.T) {
	const neutral = "Windows did not share the Wi-Fi details."
	s := ndWiFi()
	s.wifi = collect.WiFiInfo{Hidden: true, HiddenNeutral: true}
	r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	if r.WiFiHidden == nil {
		t.Fatal("wifi_hidden must be set")
	}
	if r.WiFiHidden.Headline != neutral {
		t.Errorf("neutral headline: got %q, want %q", r.WiFiHidden.Headline, neutral)
	}
	if strings.Contains(strings.ToLower(r.WiFiHidden.Headline+" "+r.WiFiHidden.Why), "location") {
		t.Errorf("the neutral words must not name Location: %+v", *r.WiFiHidden)
	}
	ndNoLocationAsk(t, "neutral words", r)
	if g := core.GradeLevel(r.WiFiHidden.Headline + " " + r.WiFiHidden.Why); g > 8.0 {
		t.Errorf("grade %.1f too high: %+v", g, *r.WiFiHidden)
	}
	s = ndWiFi()
	s.wifi = collect.WiFiInfo{Hidden: true}
	r = Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	if r.WiFiHidden == nil || r.WiFiHidden.Headline != wantHeadline || r.WiFiHidden.Why != wantWhy {
		t.Errorf("English refusal: the existing words stay, got %+v", r.WiFiHidden)
	}
}
