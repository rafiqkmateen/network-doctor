package core

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ND-12: four fault cases plus a healthy line.
func TestDiagnoseCauses(t *testing.T) {
	cases := []struct {
		name string
		r    Reading
		want Cause
	}{
		{"healthy", Reading{Kind: Wired, Online: true, AdapterUp: true, RouterMs: 1, InternetMs: 20, LinkMbps: 1000, Duplex: "full", DownMbps: 300, PlanMbps: 300}, CauseNone},
		{"computer busy", Reading{Kind: Wired, Online: true, AdapterUp: true, RouterMs: 2, InternetMs: 25, LinkMbps: 1000, Duplex: "full", LocalBusyPct: 92}, CauseComputer},
		{"cable errors", Reading{Kind: Wired, Online: true, AdapterUp: true, RouterMs: 40, RouterLoss: 6, LinkMbps: 100, Duplex: "half", ErrorCount: 900}, CauseCableOrSignal},
		{"weak wifi", Reading{Kind: WiFi, Online: true, AdapterUp: true, RouterMs: 60, RouterLoss: 5, SignalPct: 30}, CauseCableOrSignal},
		{"router slow", Reading{Kind: Wired, Online: true, AdapterUp: true, RouterMs: 90, RouterLoss: 4, LinkMbps: 1000, Duplex: "full"}, CauseRouter},
		{"provider slow", Reading{Kind: Wired, Online: true, AdapterUp: true, RouterMs: 1, InternetMs: 260, InternetLoss: 8, LinkMbps: 1000, Duplex: "full"}, CauseProvider},
		{"provider under plan", Reading{Kind: Wired, Online: true, AdapterUp: true, RouterMs: 1, InternetMs: 30, LinkMbps: 1000, Duplex: "full", DownMbps: 40, PlanMbps: 300}, CauseProvider},
	}
	for _, c := range cases {
		f := Diagnose(c.r)
		if f.Cause != c.want {
			t.Errorf("%s: got %q want %q", c.name, f.Cause, c.want)
		}
		if f.Cause != CauseNone && (f.Summary == "" || f.First == "") {
			t.Errorf("%s: missing summary or first step", c.name)
		}
	}
}

func TestExitCodes(t *testing.T) {
	seen := map[int]bool{}
	for _, c := range []Cause{CauseNone, CauseComputer, CauseCableOrSignal, CauseRouter, CauseProvider} {
		code := ExitCode(Finding{Cause: c})
		if seen[code] {
			t.Fatalf("duplicate exit code %d", code)
		}
		seen[code] = true
	}
	if ExitCode(Finding{Cause: CauseNone}) != 0 {
		t.Fatal("healthy must be 0")
	}
}

// ND-30 and ND-31.
func TestDropsCountTimesAndLengths(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	d := func(day, h, m int) time.Time { return time.Date(2026, 11, day, h, m, 0, 0, time.UTC) }
	ev := []Event{
		{d(5, 2, 15), false}, {d(5, 2, 20), true},
		{d(7, 2, 10), false}, {d(7, 2, 40), true},
		{d(9, 2, 5), false}, {d(9, 2, 6), true},
		{d(1, 2, 0), false}, {d(1, 2, 30), true}, // older than 7 days: ignored
	}
	r := AnalyzeDrops(ev, now)
	if r.Count != 3 {
		t.Fatalf("count %d want 3", r.Count)
	}
	if r.Drops[0].Length != 5*time.Minute || r.Drops[1].Length != 30*time.Minute || r.Drops[2].Length != time.Minute {
		t.Fatalf("bad lengths %+v", r.Drops)
	}
	if !r.Drops[0].Start.Equal(d(5, 2, 15)) {
		t.Fatal("bad start")
	}
	if !strings.Contains(r.Pattern, "02") {
		t.Fatalf("pattern should name the 0200 hour: %q", r.Pattern)
	}
}

// Found on the live laptop: the log holds many sub-second flaps. They are
// counted apart so they do not bury the real outages.
func TestBriefBlipsAreCountedApart(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	d := func(m, s int) time.Time { return time.Date(2026, 11, 10, 3, m, s, 0, time.UTC) }
	ev := []Event{
		{d(0, 0), false}, {d(0, 0), true}, // 0 s blip
		{d(5, 0), false}, {d(5, 2), true}, // 2 s blip
		{d(10, 0), false}, {d(11, 0), true}, // real: 1 minute
	}
	r := AnalyzeDrops(ev, now)
	if r.Count != 1 || r.BriefBlips != 2 {
		t.Fatalf("want 1 real drop and 2 blips, got %d and %d", r.Count, r.BriefBlips)
	}
}

func TestDropStillDownCountsToNow(t *testing.T) {
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	r := AnalyzeDrops([]Event{{now.Add(-10 * time.Minute), false}}, now)
	if r.Count != 1 || r.Drops[0].Length != 10*time.Minute {
		t.Fatalf("%+v", r)
	}
}

// ND-71 and ND-72.
func TestWeeklyDue(t *testing.T) {
	mon := time.Date(2026, 11, 9, 9, 0, 0, 0, time.Local)
	if !WeeklyDue(mon, mon.AddDate(0, 0, -7), time.Monday, 9) {
		t.Fatal("should be due Monday 0900")
	}
	if WeeklyDue(mon.Add(-time.Hour), mon.AddDate(0, 0, -7), time.Monday, 9) {
		t.Fatal("not due at 0800")
	}
	if WeeklyDue(mon.Add(time.Hour), mon.Add(time.Minute), time.Monday, 9) {
		t.Fatal("already ran this slot")
	}
}

func TestComplaintNeedsThreeInARow(t *testing.T) {
	w := func(down float64) Week { return Week{DownMbps: down, PlanMbps: 300} }
	if !ShouldDraftComplaint([]Week{w(300), w(100), w(90), w(140)}) {
		t.Fatal("three low weeks in a row should draft")
	}
	if ShouldDraftComplaint([]Week{w(100), w(100), w(250), w(100)}) {
		t.Fatal("broken run must not draft")
	}
	if ShouldDraftComplaint([]Week{w(100), w(100)}) {
		t.Fatal("two weeks is not enough")
	}
	d := ComplaintDraft([]Week{w(100), w(90), w(140)}, "Rafiq")
	if !strings.Contains(d, "300") || !strings.Contains(d, "DRAFT") {
		t.Fatalf("draft should show the plan and be marked draft: %q", d)
	}
}

// ND-73, ND-74, ND-75.
func TestAlerter(t *testing.T) {
	noon := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	a := NewAlerter(DefaultAlertConfig())
	if !a.Offer(AlertDrop, "link", noon) {
		t.Fatal("first alert should show")
	}
	if a.Offer(AlertDrop, "link", noon.Add(time.Minute)) {
		t.Fatal("one alert per outage")
	}
	a.Clear(AlertDrop, "link")
	if !a.Offer(AlertDrop, "link", noon.Add(time.Hour)) {
		t.Fatal("new outage should alert again")
	}
	cfg := DefaultAlertConfig()
	cfg.Enabled[AlertSlow] = false
	b := NewAlerter(cfg)
	if b.Offer(AlertSlow, "x", noon) {
		t.Fatal("switched-off alert must be silent")
	}
	cfg = DefaultAlertConfig()
	cfg.QuietFromHour, cfg.QuietToHour = 22, 6
	q := NewAlerter(cfg)
	if q.Offer(AlertNewDevice, "aa", time.Date(2026, 11, 11, 23, 30, 0, 0, time.UTC)) {
		t.Fatal("quiet hours must hold alerts at 2330")
	}
	if q.Offer(AlertNewDevice, "aa", time.Date(2026, 11, 11, 3, 0, 0, 0, time.UTC)) {
		t.Fatal("quiet hours must hold alerts at 0300")
	}
	if !q.Offer(AlertNewDevice, "aa", noon) {
		t.Fatal("outside quiet hours should alert")
	}
	if len(DefaultAlertConfig().Sinks) != 1 || DefaultAlertConfig().Sinks[0] != SinkConsole {
		t.Fatal("default sink must be the local console only")
	}
}

// ND-50.
func TestFlagUnknown(t *testing.T) {
	devs := []Device{{Name: "laptop", MAC: "aa-bb-cc-00-00-01"}, {Name: "?", MAC: "AA:BB:CC:00:00:02"}}
	known := map[string]bool{NormalizeMAC("AA:BB:CC:00:00:01"): true}
	f := FlagUnknown(devs, known)
	if len(f) != 1 || NormalizeMAC(f[0].MAC) != "AA:BB:CC:00:00:02" {
		t.Fatalf("%+v", f)
	}
}

// ND-20 and ND-21. Found on the live laptop: Windows gives no per-program byte
// counts without administrator rights, so the line total is exact and programs
// are ranked by open connections.
func TestBandwidthRankAndAdvice(t *testing.T) {
	ps := []Proc{{"chrome", 12}, {"onedrive", 40}, {"zoom", 3}}
	r := RankProcs(ps)
	if r[0].Name != "onedrive" {
		t.Fatalf("top should be onedrive: %+v", r)
	}
	busy := strings.Join(BandwidthAdvice(21.5, 0.4, r), " ")
	if !strings.Contains(busy, "onedrive") || !strings.Contains(busy, "21.5") {
		t.Fatalf("busy advice should give the line total and name the top program: %q", busy)
	}
	quiet := strings.Join(BandwidthAdvice(0.01, 0.01, r), " ")
	if strings.Contains(quiet, "onedrive") {
		t.Fatalf("a quiet line needs no names: %q", quiet)
	}
}

// ND-60.
// Found on the live laptop: Windows refused to report power saving for the
// wired adapter, and the tool wrongly said "off". Unknown must say unknown.
func TestUnreadablePowerSavingSaysSo(t *testing.T) {
	adv := ReviewSettings(Settings{DNS: []string{"1.1.1.1"}, PowerKnown: false})
	if !strings.Contains(adv[1].Finding, "could not read") {
		t.Fatalf("should say it could not read the setting: %+v", adv[1])
	}
	if strings.Contains(adv[1].Finding, "is off") {
		t.Fatal("must not claim it is off")
	}
	if GradeLevel(adv[1].Finding+" "+adv[1].Effect) > 8 {
		t.Fatal("too hard to read")
	}
}

func TestSettingsCoverSixAreas(t *testing.T) {
	adv := ReviewSettings(Settings{DNS: []string{"1.1.1.1"}, PowerKnown: true, PowerSaving: true, VPNActive: true, ProxyOn: true, DriverVersion: "1.0", DriverAgeDays: 1500, ConfigErrors: []string{"code 10"}})
	want := []string{"DNS", "Power saving", "VPN", "Proxy", "Driver", "Errors"}
	if len(adv) != len(want) {
		t.Fatalf("%d areas", len(adv))
	}
	for i, a := range adv {
		if a.Area != want[i] || a.Finding == "" || a.Effect == "" {
			t.Fatalf("area %d bad: %+v", i, a)
		}
	}
}

// ND-40 and ND-41.
func TestLinkAdvice(t *testing.T) {
	if len(WiredAdvice(100, "half", 900)) < 2 {
		t.Fatal("slow, half duplex and errors should each be flagged")
	}
	if len(WiredAdvice(1000, "full", 0)) != 0 {
		t.Fatal("healthy wired link needs no advice")
	}
	adv := WiFiAdvice(35, "2.4 GHz", 6, []int{6, 6, 1})
	joined := strings.Join(adv, " ")
	if !strings.Contains(joined, "5 GHz") || !strings.Contains(joined, "channel") {
		t.Fatalf("expected band and channel advice: %v", adv)
	}
}

// ND-05: every sentence shown to people reads at grade 8 or lower.
func TestReadability(t *testing.T) {
	var texts []string
	for _, c := range []Reading{
		{Kind: Wired, Online: true, AdapterUp: true, RouterMs: 2, LocalBusyPct: 95},
		{Kind: Wired, Online: true, AdapterUp: true, RouterMs: 40, RouterLoss: 6, LinkMbps: 100, Duplex: "half", ErrorCount: 9},
		{Kind: WiFi, Online: true, AdapterUp: true, RouterMs: 60, RouterLoss: 5, SignalPct: 30},
		{Kind: Wired, Online: true, AdapterUp: true, RouterMs: 90, RouterLoss: 4, LinkMbps: 1000, Duplex: "full"},
		{Kind: Wired, Online: true, AdapterUp: true, RouterMs: 1, InternetMs: 260, InternetLoss: 8, LinkMbps: 1000, Duplex: "full"},
		{Kind: Wired, Online: true, AdapterUp: true, RouterMs: 1, InternetMs: 20, LinkMbps: 1000, Duplex: "full"},
	} {
		f := Diagnose(c)
		texts = append(texts, f.Summary, f.First)
	}
	for _, a := range ReviewSettings(Settings{PowerKnown: false}) {
		texts = append(texts, a.Finding, a.Effect)
	}
	for _, a := range ReviewSettings(Settings{PowerKnown: true, PowerSaving: true, VPNActive: true, ProxyOn: true, DriverAgeDays: 1500, ConfigErrors: []string{"x"}}) {
		texts = append(texts, a.Finding, a.Effect)
	}
	for _, a := range ReviewSettings(Settings{DNS: []string{"8.8.8.8"}}) {
		texts = append(texts, a.Finding, a.Effect)
	}
	texts = append(texts, WiredAdvice(100, "half", 900)...)
	texts = append(texts, WiFiAdvice(35, "2.4 GHz", 6, []int{6, 6, 1})...)
	texts = append(texts, BandwidthAdvice(21.5, 0.4, RankProcs([]Proc{{"onedrive", 40}}))...)
	texts = append(texts, BandwidthAdvice(0, 0, nil)...)
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	texts = append(texts, AnalyzeDrops([]Event{{now.Add(-48 * time.Hour), false}, {now.Add(-47 * time.Hour), true}, {now.Add(-72 * time.Hour), false}, {now.Add(-71 * time.Hour), true}, {now.Add(-96 * time.Hour), false}, {now.Add(-95 * time.Hour), true}}, now).Pattern)
	for _, s := range texts {
		if s == "" {
			continue
		}
		if g := GradeLevel(s); g > 8.0 {
			t.Errorf("grade %.1f too high: %q", g, s)
		}
	}
}

// ND-01, ND-04, ND-51: scan the source.
func sourceFiles(t *testing.T) map[string]string {
	t.Helper()
	root := filepath.Join("..", "..")
	out := map[string]string{}
	filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err == nil && strings.HasSuffix(p, ".go") && !strings.HasSuffix(p, "_test.go") {
			b, _ := os.ReadFile(p)
			out[filepath.ToSlash(p)] = string(b)
		}
		return nil
	})
	return out
}

func TestNoRouterPasswordAnywhere(t *testing.T) {
	re := regexp.MustCompile(`(?i)password|passwd|credential`)
	for p, src := range sourceFiles(t) {
		if re.MatchString(src) {
			t.Errorf("%s mentions a password or credential", p)
		}
	}
}

func TestNothingSendsEmailOrPostsData(t *testing.T) {
	re := regexp.MustCompile(`net/smtp|http\.Post|\.PostForm`)
	for p, src := range sourceFiles(t) {
		if strings.Contains(p, "speedtest") {
			continue
		}
		if re.MatchString(src) {
			t.Errorf("%s sends data out", p)
		}
	}
}
