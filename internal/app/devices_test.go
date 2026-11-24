package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/store"
	"github.com/rafiqkmateen/network-doctor/internal/web"
)

// choiceDevices is healthy() with one device of each choice and one nobody decided on.
func choiceDevices() *fake {
	f := healthy()
	f.devices = []core.Device{
		{Name: "tv", MAC: "02:00:00:00:00:31", IP: "192.168.1.31"},
		{Name: "", MAC: "02:00:00:00:00:32", IP: "192.168.1.32"},
		{Name: "printer", MAC: "02:00:00:00:00:33", IP: "192.168.1.33"},
		{Name: "", MAC: "02:00:00:00:00:34", IP: "192.168.1.34"},
	}
	return f
}

var threeStates = map[string]store.KnownEntry{
	"02:00:00:00:00:31": {State: store.Mine, Name: "living tv"},
	"02:00:00:00:00:32": {State: store.NotMine},
	"02:00:00:00:00:33": {State: store.Ignored},
}

// ND-264: each device carries its choice and saved name; ignored devices are not strangers;
// a not-mine device stays flagged.
func TestCheckCarriesDeviceChoices(t *testing.T) {
	r := Check(context.Background(), choiceDevices(), Options{Known: map[string]bool{"02:00:00:00:00:31": true}, States: threeStates})
	by := map[string]core.Device{}
	for _, d := range r.Devices {
		by[d.MAC] = d
	}
	if d := by["02:00:00:00:00:31"]; !d.Known || d.State != "mine" || d.Label != "living tv" {
		t.Errorf("mine: %+v", d)
	}
	if d := by["02:00:00:00:00:32"]; d.Known || d.State != "not-mine" {
		t.Errorf("not mine: %+v", d)
	}
	if d := by["02:00:00:00:00:33"]; d.Known || d.State != "ignored" {
		t.Errorf("ignored: %+v", d)
	}
	if d := by["02:00:00:00:00:34"]; d.Known || d.State != "" {
		t.Errorf("undecided: %+v", d)
	}
	var strangers []string
	for _, d := range r.Unknown {
		strangers = append(strangers, d.MAC)
	}
	if strings.Join(strangers, ",") != "02:00:00:00:00:32,02:00:00:00:00:34" {
		t.Errorf("strangers must be the not-mine and the undecided device, never the ignored one: %v", strangers)
	}
	if r.KnownCount != 2 {
		t.Errorf("known_count %d, want 2 (mine and not mine; ignored does not count)", r.KnownCount)
	}
	b, _ := json.Marshal(r)
	for _, want := range []string{`"state":"not-mine"`, `"label":"living tv"`, `"known_count":2`, `"warn_new_devices":true`} {
		if !strings.Contains(string(b), want) {
			t.Errorf("JSON must carry %s", want)
		}
	}
}

// ND-265: with warnings off the report says so; missing means on.
func TestCheckSaysWhetherWarningsAreOn(t *testing.T) {
	if !Check(context.Background(), healthy(), Options{}).WarnNewDevices {
		t.Error("warnings are on unless turned off")
	}
	if Check(context.Background(), healthy(), Options{NoNewDeviceWarnings: true}).WarnNewDevices {
		t.Error("turned off must show as off")
	}
}

// ND-264: the router's page is offered only for a private router address the check already uses.
func TestRouterPageOnlyForAPrivateAddress(t *testing.T) {
	cases := map[string]string{
		"192.168.1.1": "http://192.168.1.1/",
		"10.0.0.1":    "http://10.0.0.1/",
		"172.16.0.1":  "http://172.16.0.1/",
		"8.8.8.8":     "",
		"":            "",
		"not-an-ip":   "",
		"127.0.0.1":   "",
	}
	for gw, want := range cases {
		f := healthy()
		f.adapter.Gateway = gw
		if got := Check(context.Background(), f, Options{Quick: true}).RouterPage; got != want {
			t.Errorf("router %q: page %q, want %q", gw, got, want)
		}
	}
}

// ND-264 and ND-265: watch never alerts for an ignored device, still alerts for a not-mine one,
// and raises no new-device alert at all while warnings are off.
func TestWatcherHonorsChoicesAndTheSwitch(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.UTC)
	alerted := func(opt Options) []string {
		w := NewWatcher(choiceDevices(), core.NewAlerter(core.DefaultAlertConfig()), opt)
		var macs []string
		for _, a := range w.Tick(context.Background(), now, true) {
			if a.Kind == core.AlertNewDevice {
				for _, m := range []string{"31", "32", "33", "34"} {
					if strings.Contains(a.Message, "02:00:00:00:00:"+m) {
						macs = append(macs, m)
					}
				}
			}
		}
		return macs
	}
	known := map[string]bool{"02:00:00:00:00:31": true}
	if got := strings.Join(alerted(Options{Known: known, LoadStates: func() map[string]store.KnownEntry { return threeStates }}), ","); got != "32,34" {
		t.Errorf("alerts for %s; want the not-mine and the undecided device only", got)
	}
	off := Options{Known: known, States: threeStates, NewDeviceWarningsOff: func() bool { return true }}
	if got := alerted(off); len(got) != 0 {
		t.Errorf("with warnings off there must be no new-device alert, got %v", got)
	}
}

// ND-264 and ND-265: the open page's buttons reach the real files: the known list holds each address
// once with its latest choice, and the switch lands in the settings file and in config show.
func TestOpenPageSavesChoicesAndTheSwitch(t *testing.T) {
	dir := t.TempDir()
	post := func(base, path, body string) int {
		req, _ := http.NewRequest(http.MethodPost, base+path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("X-NetDoctor", "1")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return 0
		}
		resp.Body.Close()
		return resp.StatusCode
	}
	codes := make(chan []int, 1)
	shortOpen(t, func(u string) error {
		go func() {
			codes <- []int{
				post(u, "api/known", `{"devices":[{"mac":"02-00-00-00-00-31","name":"tv","state":"mine"}]}`),
				post(u, "api/known", `{"devices":[{"mac":"02:00:00:00:00:31","name":"<b>den tv</b>","state":"mine"},{"mac":"02:00:00:00:00:32","name":"","state":"ignored"}]}`),
				post(u, "api/known", `{"devices":[{"mac":"02:00:00:00:00:32","name":"","state":"not-mine"}]}`),
				post(u, "api/settings", `{"warn_new_devices":false}`),
			}
		}()
		return nil
	})
	if code := Run(context.Background(), []string{"open"}, healthy(), dir, &bytes.Buffer{}); code != 0 {
		t.Fatalf("exit code %d", code)
	}
	for i, c := range <-codes {
		if c != 200 {
			t.Errorf("request %d: status %d", i, c)
		}
	}
	b, _ := os.ReadFile(filepath.Join(dir, "known-devices.txt"))
	if strings.Count(string(b), "02:00:00:00:00:31") != 1 || strings.Count(string(b), "02:00:00:00:00:32") != 1 {
		t.Errorf("each address must be on the list once:\n%s", b)
	}
	st := store.LoadKnownStates(filepath.Join(dir, "known-devices.txt"))
	if st["02:00:00:00:00:31"] != (store.KnownEntry{State: store.Mine, Name: "<b>den tv</b>"}) || st["02:00:00:00:00:32"].State != store.NotMine {
		t.Errorf("latest choices and names must be kept, as plain text: %v", st)
	}
	var out bytes.Buffer
	Run(context.Background(), []string{"config", "show"}, healthy(), dir, &out)
	if !strings.Contains(out.String(), `"warn_new_devices": false`) {
		t.Errorf("config show must show the switch: %s", out.String())
	}
	out.Reset()
	Run(context.Background(), []string{"config", "show"}, healthy(), t.TempDir(), &out)
	if !strings.Contains(out.String(), `"warn_new_devices": true`) {
		t.Errorf("config show must show the switch on by default: %s", out.String())
	}
	out.Reset()
	Run(context.Background(), []string{"known", "list"}, healthy(), dir, &out)
	if strings.TrimSpace(out.String()) != "02:00:00:00:00:31" {
		t.Errorf("known list shows only the devices that are mine, as before: %q", out.String())
	}
}

// ND-264 and ND-265: a check re-reads the device choices and the switch when it is given the ways to
// read them, so the phone page (share), which runs for a long time, never shows stale choices.
func TestCheckRereadsChoicesAndTheSwitch(t *testing.T) {
	dir := t.TempDir()
	opt := pageOptions(dir, 0)
	r := Check(context.Background(), choiceDevices(), opt)
	if !r.WarnNewDevices || r.KnownCount != 0 {
		t.Fatalf("nothing saved yet: warn %v, count %d", r.WarnNewDevices, r.KnownCount)
	}
	kp := filepath.Join(dir, "known-devices.txt")
	store.SetKnown(kp, "02:00:00:00:00:31", store.Mine, "")
	store.SetKnown(kp, "02:00:00:00:00:33", store.Ignored, "")
	off := false
	cfg, _ := store.LoadConfig(filepath.Join(dir, "config.json"))
	cfg.WarnNewDevices = &off
	store.SaveConfig(filepath.Join(dir, "config.json"), cfg)
	r = Check(context.Background(), choiceDevices(), opt)
	if r.WarnNewDevices || r.KnownCount != 1 {
		t.Errorf("the same options must see the new choices and the switch: warn %v, count %d", r.WarnNewDevices, r.KnownCount)
	}
	for _, d := range r.Devices {
		if d.MAC == "02:00:00:00:00:31" && !d.Known {
			t.Error("a device marked mine since the start must be known")
		}
		if d.MAC == "02:00:00:00:00:33" && d.State != "ignored" {
			t.Error("a device ignored since the start must show as ignored")
		}
	}
}

// ND-264: the terminal agrees with the page: an ignored device is neither counted nor marked as a
// stranger, and a not-mine device says so.
func TestTerminalShowsDeviceChoices(t *testing.T) {
	r := Check(context.Background(), choiceDevices(), Options{Known: map[string]bool{"02:00:00:00:00:31": true}, States: threeStates})
	var out bytes.Buffer
	printReport(&out, r)
	s := out.String()
	if !strings.Contains(s, "(4, 2 not marked known)") {
		t.Errorf("count must leave out the ignored device:\n%s", s)
	}
	for _, l := range strings.Split(s, "\n") {
		switch {
		case strings.Contains(l, "02:00:00:00:00:33") && (!strings.Contains(l, "ignored") || strings.Contains(l, "not marked known")):
			t.Errorf("ignored row: %q", l)
		case strings.Contains(l, "02:00:00:00:00:32") && !strings.Contains(l, "not mine"):
			t.Errorf("not-mine row: %q", l)
		case strings.Contains(l, "02:00:00:00:00:34") && !strings.Contains(l, "not marked known"):
			t.Errorf("undecided row: %q", l)
		}
	}
}

// ND-264: known_count counts only the tool's own list, so a person who chose nothing sees the
// "Tell Network Doctor which of these are yours" wording even with a trusted list from elsewhere.
func TestKnownCountIsOnlyTheOwnList(t *testing.T) {
	r := Check(context.Background(), choiceDevices(), Options{Known: map[string]bool{"02:00:00:00:00:31": true}})
	if r.KnownCount != 0 {
		t.Errorf("a device known only from another program's list is not a choice: count %d", r.KnownCount)
	}
}

// ND-262: the Windows time setting decides 12- or 24-hour: the short time format first (the clock the
// page shows), then the long one, then the older iTime switch. Letters inside quotes are text.
func TestParseWindowsClock(t *testing.T) {
	yes, no := true, false
	cases := []struct {
		short, long, iTime string
		want               *bool
	}{
		{"HH:mm", "h:mm:ss tt", "0", &yes}, // short 24-hour wins over a long 12-hour
		{"h:mm tt", "HH:mm:ss", "1", &no},  // short 12-hour wins over a long 24-hour
		{"", "h:mm:ss tt", "1", &no},       // then the long format
		{"", "HH:mm:ss", "0", &yes},        // the format is what the clock really shows
		{"", "", "1", &yes},                // then iTime
		{"", "", "0", &no},                 //
		{"h:mm 'Hrs'", "", "", &no},        // quoted text is not the hour
		{"'Hrs' mm", "", "", nil},          // nothing outside the quotes
		{"mm:ss", "", "7", nil},            // odd values
		{"", "", "", nil},                  // missing
		{"'it''s' H:mm", "", "", &yes},     // a quote inside quoted text
	}
	for _, c := range cases {
		got := parseWindowsClock(c.short, c.long, c.iTime)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("short %q, long %q, iTime %q: got %v want %v", c.short, c.long, c.iTime, show(got), show(c.want))
		}
	}
}

func show(b *bool) string {
	if b == nil {
		return "unknown"
	}
	if *b {
		return "24-hour"
	}
	return "12-hour"
}

// ND-262: the check sends the clock setting as clock_24h, and leaves it out when unknown.
func TestCheckSendsTheClockSetting(t *testing.T) {
	old := systemClock24h
	t.Cleanup(func() { systemClock24h = old })
	yes := true
	systemClock24h = func() *bool { return &yes }
	b, _ := json.Marshal(Check(context.Background(), healthy(), Options{Quick: true}))
	if !strings.Contains(string(b), `"clock_24h":true`) {
		t.Errorf("clock_24h must be sent: %s", b)
	}
	systemClock24h = func() *bool { return nil }
	b, _ = json.Marshal(Check(context.Background(), healthy(), Options{Quick: true}))
	if strings.Contains(string(b), "clock_24h") {
		t.Error("an unknown clock setting must be left out")
	}
}

// ND-263: the open server waits about 3 minutes without a heartbeat, so a tab left in the
// background (where browsers slow their timers) does not lose it.
func TestOpenWaitsLongEnoughForAHiddenTab(t *testing.T) {
	if openIdle < 170*time.Second || openIdle > 200*time.Second {
		t.Errorf("quiet time %v, want about 3 minutes", openIdle)
	}
}

// ND-263: with no browser, the person is shown the address (a message box on Windows) and the
// program ends when they are done with it; it never waits unseen.
func TestOpenWithNoBrowserEndsWhenThePersonIsDone(t *testing.T) {
	shortOpen(t, func(string) error { return errors.New("no browser") })
	openNoPage = time.Minute // long, so only the person's answer can end it in time
	var shown string
	noBrowser = func(u string, _ io.Writer) bool { shown = u; return true }
	done := make(chan int, 1)
	go func() { done <- Run(context.Background(), []string{"open"}, healthy(), t.TempDir(), &bytes.Buffer{}) }()
	select {
	case code := <-done:
		if code != 0 {
			t.Errorf("exit code %d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("it must end once the person has seen the address")
	}
	if !strings.HasPrefix(shown, "http://127.0.0.1:") {
		t.Errorf("the address shown: %q", shown)
	}
}

// ND-264: "All of these are mine" is saved in one write: all the devices, or none.
func TestPageSaverSavesAllOrNothing(t *testing.T) {
	dir := t.TempDir()
	sv := pageSaver(dir)
	err := sv.Devices([]web.Mark{{MAC: "02:00:00:00:00:31", State: "mine"}, {MAC: "not-an-address", State: "mine"}})
	if err == nil {
		t.Fatal("a bad device must refuse the whole save")
	}
	if _, err := os.Stat(filepath.Join(dir, "known-devices.txt")); err == nil {
		t.Error("a refused save must write nothing")
	}
	if err := sv.Devices([]web.Mark{{MAC: "02:00:00:00:00:31", State: "mine"}, {MAC: "02:00:00:00:00:32", State: "mine"}}); err != nil {
		t.Fatal(err)
	}
	if n := len(store.LoadKnownStates(filepath.Join(dir, "known-devices.txt"))); n != 2 {
		t.Errorf("want both devices saved, got %d", n)
	}
}

// ND-265: the page's switch and a `config` command saved at the same time are both kept.
func TestSwitchAndConfigCommandAtOnceKeepBoth(t *testing.T) {
	dir := t.TempDir()
	sv := pageSaver(dir)
	var wg sync.WaitGroup
	for i := 0; i < 15; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			Run(context.Background(), []string{"config", "plan", "300"}, healthy(), dir, &bytes.Buffer{})
		}()
		go func() {
			defer wg.Done()
			if err := sv.Settings(web.Settings{WarnNewDevices: false}); err != nil {
				t.Error(err)
			}
		}()
	}
	wg.Wait()
	c, err := store.LoadConfig(filepath.Join(dir, "config.json"))
	if err != nil || c.PlanMbps != 300 || c.NewDeviceWarnings() {
		t.Errorf("both changes must be kept: plan %v, warnings on %v (%v)", c.PlanMbps, c.NewDeviceWarnings(), err)
	}
}

// ND-264: ignoring devices alone is not yet a choice of whose devices are whose, so the page keeps
// its neutral wording until one device is marked mine or not mine.
func TestIgnoredAloneIsNotAChoice(t *testing.T) {
	r := Check(context.Background(), choiceDevices(), Options{States: map[string]store.KnownEntry{"02:00:00:00:00:33": {State: store.Ignored}}})
	if r.KnownCount != 0 {
		t.Errorf("only an ignored device: known_count %d, want 0", r.KnownCount)
	}
}

// ND-265: setup saves only what it asked about, so the page's switch changed while setup was
// running is not put back.
func TestSetupKeepsASwitchChangedMeanwhile(t *testing.T) {
	dir := t.TempDir()
	cfgPath := filepath.Join(dir, "config.json")
	opt := SetupOptions{
		Status: func() bool { return false },
		Enable: func(io.Writer) int { // runs in the middle of setup, as a press on the page might
			store.UpdateConfig(cfgPath, func(c *store.Config) error { off := false; c.WarnNewDevices = &off; return nil })
			return 0
		},
		Share: func() int { return 0 },
	}
	in := strings.NewReader("300\nSam\n\ny\ny\nn\n")
	if code := Setup(context.Background(), healthy(), dir, in, &bytes.Buffer{}, opt); code != 0 {
		t.Fatalf("setup code %d", code)
	}
	c, _ := store.LoadConfig(cfgPath)
	if c.NewDeviceWarnings() {
		t.Error("setup must not undo the switch turned off while it ran")
	}
	if c.PlanMbps != 300 || c.Name != "Sam" || strings.Join(c.Sinks, ",") != "console,file" {
		t.Errorf("setup's own answers must be saved: %+v", c)
	}
}
