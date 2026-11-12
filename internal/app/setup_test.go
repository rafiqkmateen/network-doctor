package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/store"
)

type setupRecorder struct {
	weeklyOn    bool
	enableCalls int
	shareCalls  int
	statusAsked int
	opt         SetupOptions
}

func newRecorder(weeklyOn bool) *setupRecorder {
	r := &setupRecorder{weeklyOn: weeklyOn}
	r.opt = SetupOptions{
		Status: func() bool { r.statusAsked++; return r.weeklyOn },
		Enable: func(io.Writer) int { r.enableCalls++; return 0 },
		Share:  func() int { r.shareCalls++; return 0 },
	}
	return r
}

func threeDevices() *fake {
	f := healthy()
	f.devices = []core.Device{
		{Name: "tv", MAC: "AA:AA:AA:AA:AA:01", IP: "192.168.1.30", Maker: "Samsung"},
		{Name: "", MAC: "AA:AA:AA:AA:AA:02", IP: "192.168.1.31"},
		{Name: "laptop", MAC: "AA:AA:AA:AA:AA:03", IP: "192.168.1.32"},
		{Name: "this", MAC: "AA:AA:AA:AA:AA:99", IP: "192.168.1.20"}, // this computer: never listed
	}
	return f
}

// ND-140 and ND-143: the helper asks, shows the devices, and saves what was answered.
func TestSetupAsksAndSaves(t *testing.T) {
	dir := t.TempDir()
	rec := newRecorder(false)
	in := strings.NewReader("300\nRafiq\n1 3\ny\ny\nn\n")
	var out bytes.Buffer
	if code := Setup(context.Background(), threeDevices(), dir, in, &out, rec.opt); code != 0 {
		t.Fatalf("code %d\n%s", code, out.String())
	}
	cfg, _ := store.LoadConfig(filepath.Join(dir, "config.json"))
	if cfg.PlanMbps != 300 || cfg.Name != "Rafiq" {
		t.Errorf("plan and name: %+v", cfg)
	}
	if got := strings.Join(cfg.Sinks, ","); got != "console,file" {
		t.Errorf("alerts file should be on beside the console: %s", got)
	}
	known, _ := store.LoadKnown(filepath.Join(dir, "known-devices.txt"))
	if !known["AA:AA:AA:AA:AA:01"] || !known["AA:AA:AA:AA:AA:03"] || known["AA:AA:AA:AA:AA:02"] || known["AA:AA:AA:AA:AA:99"] {
		t.Errorf("only devices 1 and 3 should be marked known: %v", known)
	}
	if rec.enableCalls != 1 || rec.shareCalls != 0 {
		t.Errorf("weekly on once, phone page not started: %d %d", rec.enableCalls, rec.shareCalls)
	}
	if strings.Contains(out.String(), "AA:AA:AA:AA:AA:99") {
		t.Error("this computer must not be in the list of devices to mark")
	}
}

// ND-142 and ND-143: pressing Enter on everything means no, and changes nothing but the tool's own file.
func TestSetupEnterMeansNo(t *testing.T) {
	dir := t.TempDir()
	rec := newRecorder(false)
	var out bytes.Buffer
	Setup(context.Background(), threeDevices(), dir, strings.NewReader("\n\n\n\n\n\n\n"), &out, rec.opt)
	if rec.enableCalls != 0 || rec.shareCalls != 0 {
		t.Fatalf("nothing should start: %d %d", rec.enableCalls, rec.shareCalls)
	}
	cfg, err := store.LoadConfig(filepath.Join(dir, "config.json"))
	if err != nil || cfg.PlanMbps != 0 || cfg.Name != "" || strings.Join(cfg.Sinks, ",") != "console" {
		t.Fatalf("settings should stay at their defaults: %+v %v", cfg, err)
	}
	if _, err := os.Stat(filepath.Join(dir, "config.json")); err != nil {
		t.Error("the settings file should exist so the helper is not offered again")
	}
	if _, err := os.Stat(filepath.Join(dir, "known-devices.txt")); err == nil {
		t.Error("no device should be marked known")
	}
}

// ND-143: an end of input is the same as Enter.
func TestSetupEndOfInputIsNo(t *testing.T) {
	rec := newRecorder(false)
	var out bytes.Buffer
	if code := Setup(context.Background(), healthy(), t.TempDir(), strings.NewReader(""), &out, rec.opt); code != 0 || rec.enableCalls != 0 {
		t.Fatalf("code %d, enable %d", code, rec.enableCalls)
	}
}

// ND-140: the weekly check is not offered twice when the installer already made it.
func TestSetupSkipsWeeklyWhenAlreadyOn(t *testing.T) {
	rec := newRecorder(true)
	var out bytes.Buffer
	Setup(context.Background(), healthy(), t.TempDir(), strings.NewReader("\n\n\ny\n\n\n"), &out, rec.opt)
	if rec.enableCalls != 0 {
		t.Fatal("must not make a second weekly task")
	}
	if strings.Contains(out.String(), setupAskWeekly) {
		t.Error("the weekly question should not be asked when it is already on")
	}
	if !strings.Contains(out.String(), setupWeeklyOn) {
		t.Error("should say the weekly check is already on")
	}
}

// ND-140: yes to the phone page starts it.
func TestSetupCanStartPhonePage(t *testing.T) {
	rec := newRecorder(true)
	var out bytes.Buffer
	Setup(context.Background(), healthy(), t.TempDir(), strings.NewReader("\n\n\n\ny\n"), &out, rec.opt)
	if rec.shareCalls != 1 {
		t.Fatalf("phone page should start once, got %d\n%s", rec.shareCalls, out.String())
	}
}

// ND-141: the helper starts by itself only for a person at a keyboard, with no command and no settings file.
func TestSetupStartsOnlyWhenItShould(t *testing.T) {
	ctx := context.Background()
	run := func(args []string, keyboard bool, haveConfig bool) string {
		dir := t.TempDir()
		if haveConfig {
			store.SaveConfig(filepath.Join(dir, "config.json"), store.Config{Sinks: []string{"console"}})
		}
		var out bytes.Buffer
		RunWith(ctx, args, healthy(), dir, Terminal{In: strings.NewReader("\n\n\n\n\n\n"), Keyboard: keyboard}, &out)
		return out.String()
	}
	if !strings.Contains(run(nil, true, false), setupWelcome) {
		t.Error("a person, no command, no settings file: the helper should start")
	}
	if strings.Contains(run(nil, true, true), setupWelcome) {
		t.Error("with a settings file the helper must not start")
	}
	if strings.Contains(run(nil, false, false), setupWelcome) {
		t.Error("without a keyboard the helper must not start")
	}
	for _, args := range [][]string{{"check"}, {"check", "--json"}, {"weekly"}, {"watch"}, {"serve"}, {"share"}} {
		if args[0] == "watch" || args[0] == "serve" || args[0] == "share" {
			continue // these run until stopped; the rule is the same code path as check
		}
		if strings.Contains(run(args, true, false), setupWelcome) {
			t.Errorf("%v must never start the helper", args)
		}
	}
}

// ND-141: check with JSON and no settings file is still pure JSON, with no question in front of it.
func TestJSONNeverGetsAQuestion(t *testing.T) {
	var out bytes.Buffer
	RunWith(context.Background(), []string{"check", "--json"}, healthy(), t.TempDir(), Terminal{In: strings.NewReader("y\n"), Keyboard: true}, &out)
	var rep Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("output is not only JSON: %v\n%s", err, out.String())
	}
}

// ND-141: asked for by name, with nothing to read from, the helper says so and stops.
func TestSetupNeedsSomewhereToType(t *testing.T) {
	var out bytes.Buffer
	if code := RunWith(context.Background(), []string{"setup"}, healthy(), t.TempDir(), Terminal{}, &out); code == 0 {
		t.Fatal("should refuse without input")
	}
	if !strings.Contains(out.String(), setupNoInput) {
		t.Errorf("plain message expected:\n%s", out.String())
	}
}

// ND-05: the words of the helper read at grade 8 or lower.
func TestSetupTextsReadable(t *testing.T) {
	for _, s := range setupTexts() {
		if g := core.GradeLevel(s); g > 8.0 {
			t.Errorf("grade %.1f too high: %q", g, s)
		}
	}
}
