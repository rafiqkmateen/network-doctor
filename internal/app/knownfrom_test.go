package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

const (
	ownMAC   = "AA:BB:CC:00:00:01"
	phoneMAC = "AA:BB:CC:00:00:77" // written with dashes in lower case in the other program's file
	tvMAC    = "11:22:33:44:55:66"
)

// writeTrusted writes a list the way another program might keep it: a byte order mark, then a
// JSON object whose keys are hardware addresses with dashes in lower case.
func writeTrusted(t *testing.T, p string, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte("\xEF\xBB\xBF"+body), 0o644); err != nil {
		t.Fatal(err)
	}
}

// ND-226: the joined list is the tool's own list plus the other program's list. With no
// setting, or no such file, only the tool's own list counts, and no error is shown.
func TestKnownJoined(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "known-devices.txt"), []byte(ownMAC+" laptop\n"), 0o644)
	from := filepath.Join(dir, "trusted.json")
	writeTrusted(t, from, `{"aa-bb-cc-00-00-77": "Phone", "11:22:33:44:55:66": "TV"}`)

	var out bytes.Buffer
	got := newKnownList(dir, from, &out).Load()
	if len(got) != 3 || !got[ownMAC] || !got[phoneMAC] || !got[tvMAC] {
		t.Fatalf("want all three addresses known, got %v", got)
	}
	for _, f := range []string{"", filepath.Join(dir, "missing.json")} {
		got := newKnownList(dir, f, &out).Load()
		if len(got) != 1 || !got[ownMAC] {
			t.Fatalf("setting %q: want only the tool's own address, got %v", f, got)
		}
	}
	if out.Len() != 0 {
		t.Fatalf("no note expected in any of these cases, got %q", out.String())
	}

	// The check uses the same joined list.
	if code := Run(context.Background(), []string{"config", "known-from", from}, healthy(), dir, &out); code != 0 {
		t.Fatal(out.String())
	}
	src := healthy()
	src.devices = append(src.devices, core.Device{Name: "phone", MAC: "AA-BB-CC-00-00-77", IP: "192.168.1.40"})
	out.Reset()
	Run(context.Background(), []string{"check", "--json"}, src, dir, &out)
	var rep Report
	if err := json.Unmarshal(out.Bytes(), &rep); err != nil {
		t.Fatalf("check --json must stay JSON: %v\n%s", err, out.String())
	}
	for _, d := range rep.Unknown {
		if core.NormalizeMAC(d.MAC) == phoneMAC {
			t.Fatal("the check must treat a device on the other program's list as known")
		}
	}
	if len(rep.Unknown) != 1 || rep.Unknown[0].Name != "tv" {
		t.Fatalf("only the tv should be unknown, got %+v", rep.Unknown)
	}
}

// newDeviceAlerts runs one full check and returns the addresses warned about.
func newDeviceAlerts(w *Watcher, now time.Time) []string {
	var macs []string
	for _, a := range w.Tick(context.Background(), now, true) {
		if a.Kind == core.AlertNewDevice {
			macs = append(macs, a.Message)
		}
	}
	return macs
}

// ND-227: the watcher reads the lists again at every full check; a half-written file keeps
// the last good list with one plain note; a file that never read well makes no one known.
func TestWatchReloadsKnown(t *testing.T) {
	dir := t.TempDir()
	from := filepath.Join(dir, "trusted.json")
	writeTrusted(t, from, `{}`)
	src := healthy()
	src.devices = []core.Device{{Name: "phone", MAC: phoneMAC, IP: "192.168.1.40"}}
	var out bytes.Buffer
	kl := newKnownList(dir, from, &out)
	loads := 0
	w := NewWatcher(src, core.NewAlerter(core.DefaultAlertConfig()), Options{LoadKnown: func() map[string]bool { loads++; return kl.Load() }})
	now := time.Date(2026, 11, 17, 12, 0, 0, 0, time.Local)
	fresh := func() { w.alerter = core.NewAlerter(core.DefaultAlertConfig()) } // so a repeat warning can show

	if got := newDeviceAlerts(w, now); len(got) != 1 {
		t.Fatalf("first check should warn about the phone, got %v", got)
	}
	w.Tick(context.Background(), now, false)
	if loads != 1 {
		t.Fatalf("the lists are read at full checks only; reads so far %d", loads)
	}

	writeTrusted(t, from, `{"aa-bb-cc-00-00-77": "Phone"}`)
	fresh()
	if got := newDeviceAlerts(w, now); len(got) != 0 {
		t.Fatalf("after the file marks the phone, no warning; got %v", got)
	}

	os.WriteFile(from, []byte(`{"aa-bb-cc-00-00-77": "Ph`), 0o644)
	for i := 0; i < 2; i++ {
		fresh()
		if got := newDeviceAlerts(w, now); len(got) != 0 {
			t.Fatalf("half file, check %d: the last good list must be kept; got %v", i+1, got)
		}
	}
	if n := strings.Count(out.String(), "Note:"); n != 1 {
		t.Fatalf("want one plain note for the half file, got %d: %q", n, out.String())
	}

	writeTrusted(t, from, `{"aa-bb-cc-00-00-77": "Phone"}`)
	before := out.Len()
	fresh()
	if got := newDeviceAlerts(w, now); len(got) != 0 {
		t.Fatalf("fixed file: no warning, got %v", got)
	}
	if out.Len() != before {
		t.Fatalf("the fixed file must print nothing, got %q", out.String()[before:])
	}

	// A file that never read well makes no one known, and the device is warned about as today.
	bad := filepath.Join(dir, "bad.json")
	os.WriteFile(bad, []byte(`{"aa-bb-cc`), 0o644)
	out.Reset()
	kl2 := newKnownList(dir, bad, &out)
	w2 := NewWatcher(src, core.NewAlerter(core.DefaultAlertConfig()), Options{LoadKnown: kl2.Load})
	if got := newDeviceAlerts(w2, now); len(got) != 1 {
		t.Fatalf("a file that never parsed must make no one known, got %v", got)
	}
	if !strings.Contains(out.String(), "Note:") {
		t.Fatalf("want a plain note for a file that cannot be read, got %q", out.String())
	}
}

// ND-228: the router and this computer are never warned about; another unknown device is.
func TestWatchSkipsRouter(t *testing.T) {
	src := healthy()
	src.devices = []core.Device{
		{Name: "router", MAC: "AA:AA:AA:AA:AA:0A", IP: src.adapter.Gateway},
		{Name: "me", MAC: "AA:AA:AA:AA:AA:0B", IP: src.adapter.IP},
		{Name: "stranger", MAC: "AA:AA:AA:AA:AA:0C", IP: "192.168.1.77"},
	}
	w := NewWatcher(src, core.NewAlerter(core.DefaultAlertConfig()), Options{})
	got := newDeviceAlerts(w, time.Date(2026, 11, 17, 12, 0, 0, 0, time.Local))
	if len(got) != 1 || !strings.Contains(got[0], "AA:AA:AA:AA:AA:0C") {
		t.Fatalf("want one warning, for the stranger only; got %v", got)
	}
}

// ND-229: config known-from saves, show prints, off clears; no built-in person's folder.
func TestConfigKnownFrom(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "trusted.json")
	var out bytes.Buffer
	run := func(args ...string) int {
		out.Reset()
		return Run(context.Background(), args, healthy(), dir, &out)
	}
	if code := run("config", "known-from", path); code != 0 {
		t.Fatal(out.String())
	}
	run("config", "show")
	var shown map[string]any
	if err := json.Unmarshal(out.Bytes(), &shown); err != nil || shown["known_from"] != path {
		t.Fatalf("show should print the saved path %q: %s", path, out.String())
	}
	if code := run("config", "known-from", "off"); code != 0 {
		t.Fatal(out.String())
	}
	run("config", "show")
	if strings.Contains(out.String(), "known_from") {
		t.Fatalf("after off the setting should be gone: %s", out.String())
	}
	if code := run("config", "known-from"); code == 0 || !strings.Contains(out.String(), "known-from PATH|off") {
		t.Fatalf("a missing path should be refused with the usage line: %s", out.String())
	}

	root, _ := filepath.Abs(filepath.Join("..", ".."))
	filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		b, _ := os.ReadFile(p)
		low := strings.ToLower(string(b))
		if strings.Contains(low, `c:\users`) || strings.Contains(low, `c:\\users`) || strings.Contains(low, "c:/users") {
			t.Errorf("%s holds a built-in path to a person's folder", p)
		}
		return nil
	})
}
