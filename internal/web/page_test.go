package web

import (
	"regexp"
	"strings"
	"testing"
)

// ND-262: data goes on the page as text only, never as HTML, and the page asks nothing of the internet.
func TestPageWritesTextOnlyAndLoadsNothingFromOutside(t *testing.T) {
	s := string(page)
	for _, bad := range []string{"innerHTML", "outerHTML", "insertAdjacentHTML", "document.write", "eval(", "<link", "@import", "<script src", "http://", "https://", "url("} {
		if strings.Contains(s, bad) {
			t.Errorf("the page must not use %q", bad)
		}
	}
	if !strings.Contains(s, "textContent") {
		t.Error("the page should write data with textContent")
	}
	for _, want := range []string{"prefers-color-scheme:dark", `name="viewport"`} {
		if !strings.Contains(s, want) {
			t.Errorf("the page must have %q (dark mode, phone width)", want)
		}
	}
}

// ND-262: times follow this computer's clock setting (clock_24h) and otherwise the browser; the page
// sets the 12- or 24-hour clock only from that field and never forces a language. It builds the drops
// hour and the speed retry time from the numbers, not the terminal's words.
func TestPageShowsTimesInTheSystemFormat(t *testing.T) {
	s := string(page)
	for _, want := range []string{"toLocaleTimeString", "pattern_hour", "speed_retry_at", "speed_wait_seconds", "speed_age_seconds", "clock_24h", "o.hour12 = !c24"} {
		if !strings.Contains(s, want) {
			t.Errorf("the page must use %q", want)
		}
	}
	if n := strings.Count(s, "hour12"); n != 1 {
		t.Errorf("hour12 may be set only from clock_24h, in one place; found %d", n)
	}
	for _, bad := range []string{"hourCycle", "h23", "'en-US'", `"en-US"`, "'en-GB'"} {
		if strings.Contains(s, bad) {
			t.Errorf("the page must not force a clock or a language: found %q", bad)
		}
	}
}

// ND-262: the page runs the check when it loads, says it is checking, keeps a Check again button,
// and sends the heartbeat that lets the open command quit when the page is gone.
func TestPageRunsTheCheckAndChecksIn(t *testing.T) {
	s := string(page)
	for _, want := range []string{"Checking... about 30 seconds", "Check again", "/api/check", "/api/alive", "setInterval", "never changes your network", "It saves only your device choices and the new-device switch.", "misses >= 3", "Network Doctor has stopped. Start it again.", "A check is already running"} {
		if !strings.Contains(s, want) {
			t.Errorf("the page must contain %q", want)
		}
	}
}

// ND-262: everything the old page showed is still shown, and a speed that was not measured is never 0.
func TestPageKeepsWhatTheOldPageShowed(t *testing.T) {
	s := string(page)
	for _, field := range []string{
		"finding", "summary", "check_first", "cause", "kind", "link_mbps",
		"router_ms", "router_loss_pct", "internet_ms", "internet_loss_pct",
		"down_mbps", "up_mbps", "plan_mbps", "speed_measured", "link_advice",
		"drops", "brief_blips", ".pattern", "bandwidth_advice",
		"devices", ".name", ".maker", ".ip", ".mac", ".known",
		"settings", ".area", ".finding", ".effect", "notes",
	} {
		if !strings.Contains(s, field) {
			t.Errorf("the page no longer uses %q", field)
		}
	}
	if !regexp.MustCompile(`speed_measured\s*\?`).MatchString(s) && !strings.Contains(s, "!rd.speed_measured") {
		t.Error("the page must decide on speed_measured before it shows a speed")
	}
}

// ND-262: the page shows only what the check really produces. These came from the design mock-up
// and have no field behind them, so they must not appear.
func TestPageHasNothingTheCheckDoesNotProduce(t *testing.T) {
	s := string(page)
	words := []*regexp.Regexp{
		regexp.MustCompile(`\bFast\b`), regexp.MustCompile(`\bQuick\b`), regexp.MustCompile(`(?i)\bslow\b`),
		regexp.MustCompile(`(?i)joined`), regexp.MustCompile(`(?i)lap` + `top`),
		regexp.MustCompile(`(?i)which are yours`), regexp.MustCompile(`(?i)not marked known`),
		regexp.MustCompile(`(?i)nothing leaves`), regexp.MustCompile(`(?i)mock-?up`),
		regexp.MustCompile(`(?i)living room`), regexp.MustCompile(`--warn`),
		regexp.MustCompile(`/api/fix`), regexp.MustCompile(`(?i)>\s*fix`),
	}
	for _, re := range words {
		if re.MatchString(s) {
			t.Errorf("the page shows %q, which no field of the check produces", re.String())
		}
	}
	// Device-type pictures: no emoji or picture characters at all.
	for _, r := range s {
		if r >= 0x2190 && r != 0xFEFF {
			t.Errorf("the page holds the picture character %U; devices have no type to show", r)
			break
		}
	}
	if regexp.MustCompile(`&#[0-9]{4,};`).MatchString(s) {
		t.Error("the page holds a picture character written as an HTML number")
	}
	for _, n := range regexp.MustCompile(`\d+(\.\d+)+`).FindAllString(s, -1) {
		if strings.Count(n, ".") == 2 { // X.Y.Z; a four-part address such as 127.0.0.1 is not a version
			t.Errorf("the page must not write a version number of its own (%s); none comes from the check", n)
		}
	}
}

// ND-264 and ND-265: the page offers the three choices and the name box on this computer only, words
// the empty list neutrally, explains what to do about a device that is not yours, and keeps the
// new-device switch in its own Settings area with the plain warning before it goes off.
func TestPageOffersDeviceChoicesAndTheSwitch(t *testing.T) {
	s := string(page)
	for _, want := range []string{
		"This one is mine", "All of these are mine", "Not mine", "Ignore", "Name (optional)", "maxLength = 40",
		"Tell Network Doctor which of these are yours and it will warn you about new ones.",
		"Yours", "Not marked yours", " ignored", "Bring back", "undecided",
		"/api/known", "/api/settings", "X-NetDoctor", "application/json",
		"Network Doctor only looks and cannot remove a device.", "change the Wi-Fi password",
		"Open my router's page", "router_page", "noopener",
		"Warn me about new devices",
		"Network Doctor will stop warning you about new devices on your network. Anyone could join and you would not be told.",
		"warn_new_devices", "known_count", "X-NetDoctor-Save", "Your choices", "Working now.", "id=\"go2\"",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the page must contain %q", want)
		}
	}
}

// ND-263: every request the page makes to start work carries its own header.
func TestPageSendsItsHeader(t *testing.T) {
	s := string(page)
	if strings.Count(s, "headers: headers(") < 3 {
		t.Error("the check, the heartbeat and the saves must all send the page's header")
	}
	if strings.Contains(s, "These are mine") || strings.Contains(s, "only looks at your connection. It never changes anything") {
		t.Error("old wording is still on the page")
	}
}

// One page at a time: a page told it is retired (410) wipes itself and shows only the notice.
func TestPageGivesWayToANewerPage(t *testing.T) {
	s := string(page)
	for _, want := range []string{"A newer Network Doctor page is open. Use that one.", "=== 410",
		"document.body.replaceChildren("} {
		if !strings.Contains(s, want) {
			t.Errorf("the page must have %q", want)
		}
	}
}

// The status line under Check again is for check messages only: save messages sit by their row.
func TestSavedMessageIsNotUnderCheckAgain(t *testing.T) {
	s := string(page)
	for _, bad := range []string{"state.textContent = 'Saved.'", "state.textContent = 'Could not save"} {
		if strings.Contains(s, bad) {
			t.Errorf("the check status line must not show %q", bad)
		}
	}
	if !strings.Contains(s, "warnNote") {
		t.Error("the switch must show its own saved message beside it")
	}
}

// The drops card shows the note for drops before the 7 day bars.
func TestDropsChartExplainsEarlierDrops(t *testing.T) {
	if !strings.Contains(string(page), "L.offChartNote(") {
		t.Error("the drops chart must use offChartNote")
	}
}
