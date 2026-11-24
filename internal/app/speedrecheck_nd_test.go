package app

// Checks for the second speed test (ND-234) and that the region setting never sets a VPN hint
// (ND-241), written before the build. The first test goes through Source.Speed (the path that may use
// Ookla's tool); the second must go through Extras.SpeedCloudflare only, at most once per check.

import (
	"bytes"
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/store"
)

// ndSpeed is a wired machine whose first test gives down and up, and whose second test gives cf.
func ndSpeed(down, up float64, cf collect.SpeedPair) *ndSource {
	f := healthy()
	f.down, f.up = down, up
	return &ndSource{fake: f, probe: ndExpected(), cf: cf}
}

// ndOnce checks the first test ran once and the second at most once.
func ndOnce(t *testing.T, where string, s *ndSource) {
	t.Helper()
	if n := speedCalls(s.fake); n > 1 {
		t.Errorf("%s: the first speed test (which may use Ookla) ran %d times; once at most", where, n)
	}
	if n := s.cfCalls.Load(); n > 1 {
		t.Errorf("%s: the second test ran %d times; once per check at most", where, n)
	}
}

// ND-234: the second test runs when download is under one fifth of upload and upload is at
// least 20; the edges are exactly one fifth and upload just under 20.
func TestND234RatioTrigger(t *testing.T) {
	cases := []struct {
		name     string
		down, up float64
		want     bool
	}{
		{"3.9 down, 20 up", 3.9, 20, true},
		{"exactly one fifth: 4 down, 20 up", 4, 20, false},
		{"upload just under 20: 3.9 down, 19.9 up", 3.9, 19.9, false},
		{"100 down, 600 up", 100, 600, true},
		{"exactly one fifth: 120 down, 600 up", 120, 600, false},
		{"healthy: 290 down, 20 up", 290, 20, false},
	}
	for _, c := range cases {
		s := ndSpeed(c.down, c.up, collect.SpeedPair{Down: 280, Up: 20})
		r := Check(context.Background(), s, Options{PlanMbps: 300})
		ndOnce(t, c.name, s)
		if speedCalls(s.fake) != 1 {
			t.Errorf("%s: the first test runs once", c.name)
		}
		ran := s.cfCalls.Load() == 1
		if ran != c.want || (r.SpeedRecheck != nil) != c.want {
			t.Errorf("%s: second test ran=%v, speed_recheck=%v; want %v", c.name, ran, r.SpeedRecheck != nil, c.want)
		}
		ndNoLocationAsk(t, c.name, r)
	}
}

// ND-234: what speed_recheck holds, and the plan comparison uses the higher download.
func TestND234RecheckContentAndHigherNumber(t *testing.T) {
	s := ndSpeed(10, 100, collect.SpeedPair{Down: 280, Up: 95})
	r := Check(context.Background(), s, Options{PlanMbps: 300})
	ndOnce(t, "recheck", s)
	rc := r.SpeedRecheck
	if rc == nil {
		t.Fatal("speed_recheck must be set")
	}
	if rc.FirstDown != 10 || rc.FirstUp != 100 || rc.SecondDown != 280 || rc.SecondUp != 95 {
		t.Errorf("both pairs of numbers: %+v", *rc)
	}
	if rc.Tool != collect.ToolCloudflare || !rc.Finished {
		t.Errorf("tool must be Cloudflare and finished: %+v", *rc)
	}
	if !strings.Contains(rc.Headline, "280") || !strings.Contains(strings.ToLower(rc.Headline), "first") {
		t.Errorf("headline must say the first number looks wrong and give 280: %q", rc.Headline)
	}
	if r.Finding.Cause == core.CauseProvider {
		t.Errorf("280 of a 300 plan is fine; the odd 10 must not be the answer: %+v", r.Finding)
	}
	ndNoLocationAsk(t, "recheck", r)
}

// ND-234: when the second test cannot finish, the check says so and keeps the first number.
func TestND234SecondTestFails(t *testing.T) {
	s := ndSpeed(10, 100, collect.SpeedPair{})
	s.cfErr = context.DeadlineExceeded
	r := Check(context.Background(), s, Options{PlanMbps: 300})
	ndOnce(t, "second fails", s)
	rc := r.SpeedRecheck
	if rc == nil {
		t.Fatal("speed_recheck must be set even when the second test fails")
	}
	if rc.Finished || !strings.Contains(strings.ToLower(rc.Headline), "could not finish") {
		t.Errorf("must say the second test could not finish: %+v", *rc)
	}
	if rc.FirstDown != 10 || rc.FirstUp != 100 || rc.Tool != collect.ToolCloudflare {
		t.Errorf("first pair and the tool tried: %+v", *rc)
	}
	if r.Finding.Cause != core.CauseProvider {
		t.Errorf("with only the first number, 10 of 300 is still the provider: %+v", r.Finding)
	}
}

// ND-234: never in quick mode, never inside a speed hold, never after a first test that did not finish.
func TestND234NotInQuickHoldOrFailedFirst(t *testing.T) {
	odd := collect.SpeedPair{Down: 280, Up: 95}
	s := ndSpeed(10, 100, odd)
	if r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true}); r.SpeedRecheck != nil || s.cfCalls.Load() != 0 {
		t.Error("quick mode: no second test")
	}
	s = ndSpeed(10, 100, odd)
	if r := Check(context.Background(), s, Options{PlanMbps: 300, SpeedHoldUntil: time.Now().Add(time.Hour)}); r.SpeedRecheck != nil || s.cfCalls.Load() != 0 {
		t.Error("speed hold from the server: no second test")
	}
	s = ndSpeed(10, 100, odd)
	last := &SpeedSample{At: time.Now().Add(-time.Minute), Down: 10, Up: 100, Tool: collect.ToolCloudflare}
	if r := Check(context.Background(), s, Options{PlanMbps: 300, LastSpeed: last}); r.SpeedRecheck != nil || s.cfCalls.Load() != 0 {
		t.Error("inside the hold after a recent reading: no second test")
	}
	s = ndSpeed(0, 0, odd)
	s.speedErr = context.DeadlineExceeded
	s.dnsFails = false
	if r := Check(context.Background(), s, Options{PlanMbps: 300}); r.SpeedRecheck != nil || s.cfCalls.Load() != 0 {
		t.Error("first test did not finish: no second test")
	}
	s = ndSpeed(10, 100, odd)
	s.speedErr = &collect.SpeedRefusedError{RetryAfter: time.Minute}
	if r := Check(context.Background(), s, Options{PlanMbps: 300}); r.SpeedRecheck != nil || s.cfCalls.Load() != 0 {
		t.Error("first test refused by the server: no second test")
	}
}

func ndRunCheck(t *testing.T, s *ndSource, dir string) Report {
	t.Helper()
	var out bytes.Buffer
	Run(context.Background(), []string{"check", "--json", "--plan", "300"}, s, dir, &out)
	var r Report
	if err := json.Unmarshal(out.Bytes(), &r); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, out.String())
	}
	return r
}

// ND-234: the second test never changes the speed hold, even when its server refuses.
func TestND234HoldNeverChanged(t *testing.T) {
	dir := t.TempDir()
	s := ndSpeed(10, 100, collect.SpeedPair{})
	s.cfErr = &collect.SpeedRefusedError{RetryAfter: time.Hour}
	r := ndRunCheck(t, s, dir)
	ndOnce(t, "second refused", s)
	if s.cfCalls.Load() != 1 {
		t.Fatalf("the second test should have been tried once, got %d", s.cfCalls.Load())
	}
	if !LoadSpeedHold(dir).IsZero() || r.SpeedRetryAfterSeconds != 0 {
		t.Errorf("a refused second test must not set a hold: hold %v, retry %d", LoadSpeedHold(dir), r.SpeedRetryAfterSeconds)
	}
}

// ND-234: the second test also runs when download is under one fifth of the last weekly check
// (upload kept under 20 so only this rule can start it); edge at exactly one fifth; no weekly file, no test.
func TestND234WeeklyTrigger(t *testing.T) {
	cases := []struct {
		name   string
		weekly bool
		down   float64
		want   bool
	}{
		{"99 down, last week 500", true, 99, true},
		{"exactly one fifth: 100 down, last week 500", true, 100, false},
		{"99 down, no weekly check yet", false, 99, false},
	}
	for _, c := range cases {
		dir := t.TempDir()
		if c.weekly {
			old := time.Now().Add(-14 * 24 * time.Hour)
			if err := store.AppendWeekly(filepath.Join(dir, "weekly.csv"), store.Row{Time: old, Down: 20, Up: 10, PingMs: 20, Plan: 300}); err != nil {
				t.Fatal(err)
			}
			if err := store.AppendWeekly(filepath.Join(dir, "weekly.csv"), store.Row{Time: old.Add(7 * 24 * time.Hour), Down: 500, Up: 15, PingMs: 20, Plan: 300}); err != nil {
				t.Fatal(err)
			}
		}
		s := ndSpeed(c.down, 10, collect.SpeedPair{Down: 480, Up: 10})
		r := ndRunCheck(t, s, dir)
		ndOnce(t, c.name, s)
		if got := s.cfCalls.Load() == 1 && r.SpeedRecheck != nil; got != c.want {
			t.Errorf("%s: second test=%v, want %v", c.name, got, c.want)
		}
	}
}

// ND-234: a healthy full check never runs the second test.
func TestND234HealthyNeverRechecks(t *testing.T) {
	s := ndSpeed(290, 20, collect.SpeedPair{Down: 290, Up: 20, Country: "NL"})
	s.region = "US"
	r := Check(context.Background(), s, Options{PlanMbps: 300})
	if s.cfCalls.Load() != 0 || r.SpeedRecheck != nil {
		t.Error("a healthy check runs no second test")
	}
	if r.VPNHint != nil {
		t.Error("no second test, no country answer: no VPN hint")
	}
}

// ND-241: Check never sets vpn_hint from the region setting and the connection's country,
// whatever the region and the country say. A computer can be set to one country while
// the line is in another, so comparing them would be false on a normal day. A note comes only
// from a change of place (ND-248), which runs after Check.
func TestND241NeverAVPNHint(t *testing.T) {
	cases := []struct {
		name            string
		country, region string
	}{
		{"NL server, US region", "NL", "US"},
		{"CO line, US region", "CO", "US"},
		{"same country", "US", "US"},
		{"no country in the answer", "", "US"},
		{"region unknown", "NL", ""},
	}
	for _, c := range cases {
		// With a second test (its answer carries a country).
		s := ndSpeed(10, 100, collect.SpeedPair{Down: 280, Up: 95, Country: c.country})
		s.country, s.region = c.country, c.region
		r := Check(context.Background(), s, Options{PlanMbps: 300})
		ndOnce(t, c.name, s)
		if r.VPNHint != nil {
			t.Errorf("%s, second test: vpn_hint must never be set, got %+v", c.name, *r.VPNHint)
		}
		if n := s.countryCalls.Load(); n != 0 {
			t.Errorf("%s, second test: no country request may be made, got %d", c.name, n)
		}
		ndNoLocationAsk(t, c.name, r)

		// Without a second test.
		s = ndSpeed(290, 20, collect.SpeedPair{})
		s.country, s.region = c.country, c.region
		r = Check(context.Background(), s, Options{PlanMbps: 300})
		if s.cfCalls.Load() != 0 || r.SpeedRecheck != nil {
			t.Errorf("%s: a healthy check runs no second test", c.name)
		}
		if r.VPNHint != nil {
			t.Errorf("%s, no second test: vpn_hint must never be set, got %+v", c.name, *r.VPNHint)
		}
		if n := s.countryCalls.Load(); n != 0 {
			t.Errorf("%s, no second test: no country request may be made, got %d", c.name, n)
		}
		ndNoLocationAsk(t, c.name, r)
	}
}

// ND-234: the weekly rule through Options.LastWeeklyDown (0 means no weekly check yet), with
// upload under 20 so only this rule can start the second test.
func TestND234WeeklyTriggerThroughOptions(t *testing.T) {
	cases := []struct {
		name       string
		last, down float64
		want       bool
	}{
		{"99 down, last week 500", 500, 99, true},
		{"exactly one fifth: 100 down, last week 500", 500, 100, false},
		{"99 down, no weekly check (0)", 0, 99, false},
	}
	for _, c := range cases {
		s := ndSpeed(c.down, 10, collect.SpeedPair{Down: 480, Up: 10})
		r := Check(context.Background(), s, Options{PlanMbps: 300, LastWeeklyDown: c.last})
		ndOnce(t, c.name, s)
		if got := s.cfCalls.Load() == 1 && r.SpeedRecheck != nil; got != c.want {
			t.Errorf("%s: second test=%v, want %v", c.name, got, c.want)
		}
	}
	s := ndSpeed(99, 10, collect.SpeedPair{Down: 480, Up: 10})
	if r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true, LastWeeklyDown: 500}); r.SpeedRecheck != nil || s.cfCalls.Load() != 0 {
		t.Error("quick mode: no second test even against the weekly number")
	}
}
