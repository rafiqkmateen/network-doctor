package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// fake is a pretend machine.
type fake struct {
	adapter  collect.AdapterInfo
	routerP  collect.PingResult
	netP     collect.PingResult
	down, up float64
	speedErr error
	devices  []core.Device
	busy     int
	events   []core.Event
	calls    []string
}

func healthy() *fake {
	return &fake{
		adapter: collect.AdapterInfo{Kind: core.Wired, Up: true, LinkMbps: 1000, Duplex: "full", Gateway: "192.168.1.1", IP: "192.168.1.20", PrefixLength: 24},
		routerP: collect.PingResult{AvgMs: 1}, netP: collect.PingResult{AvgMs: 20},
		down: 290, up: 20,
		devices: []core.Device{{Name: "tv", MAC: "AA:AA:AA:AA:AA:01", IP: "192.168.1.30"}},
	}
}

func (f *fake) Adapter(context.Context) (collect.AdapterInfo, error) {
	f.calls = append(f.calls, "adapter")
	return f.adapter, nil
}
func (f *fake) WiFi(context.Context) (collect.WiFiInfo, error) {
	return collect.WiFiInfo{}, collect.ErrUnsupported
}
func (f *fake) Ping(_ context.Context, host string, _ int) (collect.PingResult, error) {
	f.calls = append(f.calls, "ping "+host)
	if host == f.adapter.Gateway {
		return f.routerP, nil
	}
	return f.netP, nil
}
func (f *fake) Throughput(context.Context, int) (collect.Throughput, error) {
	return collect.Throughput{BusyPct: f.busy, Procs: []core.Proc{{Name: "chrome", Conns: 12}}}, nil
}
func (f *fake) Events(context.Context, int) ([]core.Event, error) { return f.events, nil }
func (f *fake) Devices(context.Context, string, int) ([]core.Device, error) {
	return f.devices, nil
}
func (f *fake) Settings(context.Context) (core.Settings, error) {
	return core.Settings{DNS: []string{"192.168.1.1"}}, nil
}
func (f *fake) Speed(context.Context) (float64, float64, error) {
	f.calls = append(f.calls, "speed")
	return f.down, f.up, f.speedErr
}
func (f *fake) DNSWorks(context.Context) bool { return f.speedErr == nil }

// ND-70: one command gives a full check.
func TestFullCheck(t *testing.T) {
	r := Check(context.Background(), healthy(), Options{PlanMbps: 300})
	if r.Finding.Cause != core.CauseNone || r.Reading.Kind != core.Wired || r.Reading.DownMbps != 290 {
		t.Fatalf("%+v", r.Reading)
	}
	if len(r.Settings) != 6 || len(r.Devices) != 1 || len(r.Procs) == 0 {
		t.Fatal("full check should fill every section")
	}
}

// Found on the live laptop: it flagged itself as an unknown device.
func TestOwnComputerIsNeverFlagged(t *testing.T) {
	f := healthy()
	f.devices = append(f.devices, core.Device{Name: "me", MAC: "CC:CC:CC:CC:CC:09", IP: f.adapter.IP})
	r := Check(context.Background(), f, Options{})
	for _, d := range r.Unknown {
		if d.IP == f.adapter.IP {
			t.Fatal("this computer must not be flagged")
		}
	}
	w := NewWatcher(f, core.NewAlerter(core.DefaultAlertConfig()), Options{Known: map[string]bool{"AA:AA:AA:AA:AA:01": true}})
	for _, a := range w.Tick(context.Background(), time.Now(), true) {
		if strings.Contains(a.Message, f.adapter.IP) {
			t.Fatal("watch must not flag this computer")
		}
	}
}

// ND-03: offline, the online checks say so and nothing crashes.
func TestOfflineCheck(t *testing.T) {
	f := healthy()
	f.speedErr = errors.New("no route")
	f.netP = collect.PingResult{LossPct: 100}
	r := Check(context.Background(), f, Options{PlanMbps: 300})
	if r.Reading.Online {
		t.Fatal("should be offline")
	}
	joined := strings.Join(r.Notes, " ")
	if !strings.Contains(joined, "offline") {
		t.Fatalf("notes should say offline: %v", r.Notes)
	}
	if len(r.Settings) != 6 {
		t.Fatal("local checks must still run")
	}
}

// ND-80: command-line mode gives valid JSON and the right exit code.
func TestCLIJSONAndExitCodes(t *testing.T) {
	f := healthy()
	var out bytes.Buffer
	code := Run(context.Background(), []string{"check", "--json", "--plan", "300"}, f, t.TempDir(), &out)
	if code != 0 {
		t.Fatalf("healthy should exit 0, got %d", code)
	}
	var rep Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("not valid JSON: %v\n%s", err, out.String())
	}
	f.routerP = collect.PingResult{AvgMs: 90, LossPct: 4}
	out.Reset()
	code = Run(context.Background(), []string{"check", "--json"}, f, t.TempDir(), &out)
	if code != core.ExitCode(core.Finding{Cause: core.CauseRouter}) {
		t.Fatalf("router fault should map to its exit code, got %d", code)
	}
}

func TestQuickCheckSkipsOnlineAndSweep(t *testing.T) {
	f := healthy()
	Check(context.Background(), f, Options{Quick: true})
	for _, c := range f.calls {
		if c == "speed" {
			t.Fatal("quick must not run the speed test")
		}
	}
}

// ND-71 and ND-72: the weekly command adds a row and drafts, never sends.
func TestWeeklyRunWritesRowAndDraft(t *testing.T) {
	dir := t.TempDir()
	f := healthy()
	f.down = 90
	for i := 0; i < 3; i++ {
		var out bytes.Buffer
		if code := Run(context.Background(), []string{"weekly", "--plan", "300"}, f, dir, &out); code > 20 {
			t.Fatalf("code %d", code)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "weekly.csv"))
	if err != nil || strings.Count(string(b), "\n") != 4 {
		t.Fatalf("csv should have header and three rows: %q %v", b, err)
	}
	d, err := os.ReadFile(filepath.Join(dir, "complaint-draft.txt"))
	if err != nil || !strings.Contains(string(d), "DRAFT") {
		t.Fatal("a draft should exist after three low weeks")
	}
}

// ND-73 and ND-74: one alert for a drop, a slow stretch and a new device.
func TestWatcherAlerts(t *testing.T) {
	f := healthy()
	known := map[string]bool{"AA:AA:AA:AA:AA:01": true}
	w := NewWatcher(f, core.NewAlerter(core.DefaultAlertConfig()), Options{PlanMbps: 300, Known: known})
	now := time.Date(2026, 11, 11, 12, 0, 0, 0, time.UTC)
	if a := w.Tick(context.Background(), now, true); len(a) != 0 {
		t.Fatalf("healthy should be silent: %v", a)
	}
	f.adapter.Up = false
	a := w.Tick(context.Background(), now.Add(time.Minute), false)
	if len(a) != 1 || a[0].Kind != core.AlertDrop {
		t.Fatalf("drop: %v", a)
	}
	if a := w.Tick(context.Background(), now.Add(2*time.Minute), false); len(a) != 0 {
		t.Fatal("one alert per outage")
	}
	f.adapter.Up = true
	w.Tick(context.Background(), now.Add(3*time.Minute), false)
	f.down = 50
	var slow []Alert
	for i := 0; i < 3; i++ {
		slow = append(slow, w.Tick(context.Background(), now.Add(time.Duration(10+i)*time.Minute), true)...)
	}
	if len(slow) != 1 || slow[0].Kind != core.AlertSlow {
		t.Fatalf("slow stretch should alert once after three: %v", slow)
	}
	f.down = 290
	f.devices = append(f.devices, core.Device{Name: "?", MAC: "BB:BB:BB:BB:BB:02", IP: "192.168.1.99"})
	nd := w.Tick(context.Background(), now.Add(time.Hour), true)
	if len(nd) != 1 || nd[0].Kind != core.AlertNewDevice {
		t.Fatalf("new device: %v", nd)
	}
}

func TestVersionAndHelp(t *testing.T) {
	var out bytes.Buffer
	if Run(context.Background(), []string{"version"}, healthy(), t.TempDir(), &out) != 0 || !strings.Contains(out.String(), Version) {
		t.Fatal("version")
	}
	out.Reset()
	if Run(context.Background(), []string{"nonsense"}, healthy(), t.TempDir(), &out) == 0 {
		t.Fatal("unknown command should fail")
	}
}
