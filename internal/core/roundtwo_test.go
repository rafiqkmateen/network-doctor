package core

// Checks for round two (ND-231 to ND-243), written before the
// build. They test only what the requirements ask.

import (
	"fmt"
	"strings"
	"testing"
)

func ndHas(list []string, name string) bool {
	for _, s := range list {
		if s == name {
			return true
		}
	}
	return false
}

func ndNoTurnOnLocation(t *testing.T, where string, texts ...string) {
	t.Helper()
	all := strings.ToLower(strings.Join(texts, " "))
	if strings.Contains(all, "turn on") && strings.Contains(all, "location") {
		t.Errorf("%s: text tells the person to turn on Location: %q", where, all)
	}
}

// ND-233: weak at or below -75 dBm, which is 50 percent on Windows' scale.
// 50 is weak, 51 is not; 0 (not read) never is.
func TestND233WeakSignalEdgeInAdvice(t *testing.T) {
	weak := func(pct int) bool {
		for _, a := range WiFiAdvice(pct, "5 GHz", 36, nil) {
			if strings.Contains(strings.ToLower(a), "weak") {
				return true
			}
		}
		return false
	}
	cases := []struct {
		pct  int
		want bool
	}{{49, true}, {50, true}, {51, false}, {0, false}, {80, false}}
	for _, c := range cases {
		if got := weak(c.pct); got != c.want {
			t.Errorf("signal %d percent: weak=%v, want %v", c.pct, got, c.want)
		}
	}
	ndNoTurnOnLocation(t, "Wi-Fi advice", WiFiAdvice(50, "2.4 GHz", 6, []int{6, 6})...)
}

// ND-233: the diagnosis uses the same edge: a slow router with the signal at 50 percent is the weak signal.
func TestND233WeakSignalEdgeInDiagnosis(t *testing.T) {
	base := Reading{Kind: WiFi, AdapterUp: true, Online: true, RouterMs: 60}
	for _, c := range []struct {
		pct  int
		want Cause
	}{{50, CauseCableOrSignal}, {49, CauseCableOrSignal}, {51, CauseRouter}, {0, CauseRouter}} {
		r := base
		r.SignalPct = c.pct
		if got := Diagnose(r).Cause; got != c.want {
			t.Errorf("signal %d percent with a slow router: cause %q, want %q", c.pct, got, c.want)
		}
	}
}

// ND-231: a sign-in page is named as the cause, ahead of the router and the provider,
// with the exact first check and exit code 14.
func TestND231SignInFinding(t *testing.T) {
	cases := map[string]Reading{
		"online":                    {Kind: WiFi, AdapterUp: true, Online: true, SignIn: true},
		"internet pings blocked":    {Kind: WiFi, AdapterUp: true, Online: false, InternetLoss: 100, SignIn: true},
		"slow router as well":       {Kind: WiFi, AdapterUp: true, Online: true, RouterMs: 80, SignIn: true},
		"slow provider as well":     {Kind: Wired, AdapterUp: true, Online: true, InternetMs: 400, SignIn: true},
		"names cannot be looked up": {Kind: WiFi, AdapterUp: true, Online: true, DNSFails: true, SignIn: true},
	}
	for name, r := range cases {
		f := Diagnose(r)
		if f.Cause != CauseSignIn {
			t.Errorf("%s: cause %q, want %q", name, f.Cause, CauseSignIn)
			continue
		}
		if f.First != "Open a web browser and look for the sign-in page." {
			t.Errorf("%s: first check %q", name, f.First)
		}
		low := strings.ToLower(f.Summary)
		if !strings.Contains(low, "sign-in page") || strings.Contains(low, "router") || strings.Contains(low, "provider") {
			t.Errorf("%s: summary must name the sign-in page and not the router or provider: %q", name, f.Summary)
		}
		if ExitCode(f) != 14 {
			t.Errorf("%s: exit code %d, want 14", name, ExitCode(f))
		}
		ndNoTurnOnLocation(t, name, f.Summary, f.First)
	}
	if Diagnose(Reading{Kind: WiFi, AdapterUp: true, Online: true}).Cause == CauseSignIn {
		t.Error("no sign-in page: the cause must not be the sign-in page")
	}
}

// ND-239: Wi-Fi joined with no address from the router: exact words, never the cable.
func TestND239NoAddressFinding(t *testing.T) {
	for name, r := range map[string]Reading{
		"offline":                 {Kind: WiFi, AdapterUp: true, NoAddress: true, Online: false, InternetLoss: 100},
		"router silent as well":   {Kind: WiFi, AdapterUp: true, NoAddress: true, Online: false, InternetLoss: 100, RouterLoss: 100},
		"has both kinds of links": {Kind: WiFi, AdapterUp: true, NoAddress: true, Online: false, InternetLoss: 100, HasWired: true, HasWiFi: true},
	} {
		f := Diagnose(r)
		if f.Summary != "The Wi-Fi is connected but got no address from the router." {
			t.Errorf("%s: summary %q", name, f.Summary)
		}
		if f.First != "Restart the router, or press the button that asks it for a new address." {
			t.Errorf("%s: first check %q", name, f.First)
		}
		if strings.Contains(strings.ToLower(f.Summary+f.First), "cable") {
			t.Errorf("%s: must never say the cable has no link: %+v", name, f)
		}
	}
}

// ND-235 to ND-238: when each new fix is suggested.
func TestND235To238SuggestFixesRules(t *testing.T) {
	none := Settings{}
	drops := DropReport{}
	cases := []struct {
		name string
		r    Reading
		fix  string
		want bool
	}{
		{"names fail, internet answers", Reading{AdapterUp: true, Online: true, DNSFails: true}, FixPublicDNS, true},
		{"names fail, internet silent", Reading{AdapterUp: true, Online: false, DNSFails: true}, FixPublicDNS, false},
		{"names work", Reading{AdapterUp: true, Online: true}, FixPublicDNS, false},
		{"up with no address", Reading{AdapterUp: true, NoAddress: true}, FixRenewAddress, true},
		{"down with no address", Reading{AdapterUp: false, NoAddress: true}, FixRenewAddress, false},
		{"up with an address", Reading{AdapterUp: true, Online: true}, FixRenewAddress, false},
		{"dead: no internet, router loss 100", Reading{AdapterUp: true, Online: false, RouterLoss: 100, InternetLoss: 100}, FixResetAdapter, true},
		{"router loss 99", Reading{AdapterUp: true, Online: false, RouterLoss: 99, InternetLoss: 100}, FixResetAdapter, false},
		{"router answers", Reading{AdapterUp: true, Online: false, RouterLoss: 0, InternetLoss: 100}, FixResetAdapter, false},
		{"internet answers, router silent", Reading{AdapterUp: true, Online: true, RouterLoss: 100}, FixResetAdapter, false},
	}
	for _, c := range cases {
		if got := ndHas(SuggestFixes(c.r, none, drops), c.fix); got != c.want {
			t.Errorf("%s: %s offered=%v, want %v", c.name, c.fix, got, c.want)
		}
	}
	healthy := SuggestFixes(Reading{Kind: Wired, AdapterUp: true, Online: true, RouterMs: 1, InternetMs: 20}, none, drops)
	for _, n := range []string{FixPublicDNS, FixRenewAddress, FixResetAdapter} {
		if ndHas(healthy, n) {
			t.Errorf("a working connection must never be offered %s", n)
		}
	}
	// The old rules stay.
	if !ndHas(SuggestFixes(Reading{Online: true, DNSFails: true}, none, drops), FixNameCache) {
		t.Error("the name list fix must still be offered when names fail")
	}
}

// ND-240: the driver's age in days appears when it was read, and nothing about age when not.
func TestND240DriverAge(t *testing.T) {
	driver := func(s Settings) Advice {
		for _, a := range ReviewSettings(s) {
			if a.Area == "Driver" {
				return a
			}
		}
		t.Fatal("no Driver row")
		return Advice{}
	}
	a := driver(Settings{DriverVersion: "22.1.0.3", DriverAgeDays: 400})
	if !strings.Contains(a.Finding+" "+a.Effect, "400") || !strings.Contains(strings.ToLower(a.Finding+" "+a.Effect), "day") {
		t.Errorf("age read: the row must give 400 days: %+v", a)
	}
	a = driver(Settings{DriverVersion: "22.1.0.3", DriverAgeDays: 0})
	low := strings.ToLower(a.Finding + " " + a.Effect)
	for _, w := range []string{"recent", " old", "day", "year", " age"} {
		if strings.Contains(low, w) {
			t.Errorf("age not read: the row must say nothing about the age, found %q in %+v", w, a)
		}
	}
	if len(ReviewSettings(Settings{})) != 6 {
		t.Error("the settings review still gives six rows")
	}
}

// ND-241: the VPN line with no VPN program running.
func TestND241VPNLine(t *testing.T) {
	const want = "No VPN program is running on this computer. A VPN on your router cannot be seen from here, and it can change the speed and the provider shown."
	for _, a := range ReviewSettings(Settings{DNS: []string{"192.168.1.1"}}) {
		if a.Area == "VPN" {
			if !strings.Contains(a.Finding+" "+a.Effect, want) {
				t.Errorf("VPN line: %+v", a)
			}
			return
		}
	}
	t.Fatal("no VPN row")
}

// ND-231: exit code 14 for the sign-in page (already in the stub; kept so it stays).
func TestND231ExitCode14(t *testing.T) {
	if ExitCode(Finding{Cause: CauseSignIn}) != 14 {
		t.Fatal("sign-in page must exit 14")
	}
}

// ND-233: the Wi-Fi link is slow under 20 megabits, with the speed and the exact advice;
// 20 is not; 0 (not read) never is.
func TestND233WiFiLinkAdvice(t *testing.T) {
	const move = "Move closer to the router, or try the 5 GHz band."
	slow := func(mbps int) (bool, []string) {
		out := WiFiLinkAdvice(mbps)
		for _, a := range out {
			if strings.Contains(a, move) && strings.Contains(a, fmt.Sprint(mbps)) {
				return true, out
			}
		}
		return false, out
	}
	for _, c := range []struct {
		mbps int
		want bool
	}{{19, true}, {1, true}, {20, false}, {300, false}, {0, false}} {
		got, out := slow(c.mbps)
		if got != c.want {
			t.Errorf("%d megabits: slow=%v, want %v: %v", c.mbps, got, c.want, out)
		}
		if !c.want && len(out) != 0 {
			t.Errorf("%d megabits: no advice expected, got %v", c.mbps, out)
		}
		ndNoTurnOnLocation(t, "link advice", out...)
		for _, a := range out {
			if g := GradeLevel(a); g > 8.0 {
				t.Errorf("grade %.1f too high: %q", g, a)
			}
		}
	}
}
