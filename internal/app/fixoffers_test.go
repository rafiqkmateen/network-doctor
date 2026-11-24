package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/fix"
	"github.com/rafiqkmateen/network-doctor/internal/store"
)

type jsonOut struct {
	OK         bool   `json:"ok"`
	Name       string `json:"name"`
	Title      string `json:"title"`
	Before     string `json:"before"`
	After      string `json:"after"`
	NeedsAdmin bool   `json:"needs_admin"`
	Applied    bool   `json:"applied"`
	Undone     bool   `json:"undone"`
	Error      string `json:"error"`
}

func fixJSON(t *testing.T, p *cliPretend, dir string, args ...string) (jsonOut, int) {
	t.Helper()
	out, code := runFix(t, p, dir, args...)
	var j jsonOut
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &j); err != nil {
		t.Fatalf("not one JSON line: %v\n%s", err, out)
	}
	return j, code
}

// ND-214: with --json each fix command prints one JSON line and keeps its exit codes.
func TestFixJSON(t *testing.T) {
	p := &cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	dir := t.TempDir()
	j, code := fixJSON(t, p, dir, "apply", "proxy", "--json")
	if code != 0 || !j.OK || j.Applied || p.writes != 0 || !strings.Contains(j.Before, "10.0.0.9:8080") || j.After != "off" || j.Title == "" {
		t.Fatalf("preview must show and change nothing: %+v code %d writes %d", j, code, p.writes)
	}
	j, code = fixJSON(t, p, dir, "apply", "proxy", "--yes", "--json")
	if code != 0 || !j.OK || !j.Applied || p.proxy.Enabled {
		t.Fatalf("yes must apply: %+v code %d", j, code)
	}
	j, code = fixJSON(t, p, dir, "undo", "proxy", "--json")
	if code != 0 || !j.OK || !j.Undone || !p.proxy.Enabled {
		t.Fatalf("undo must put it back: %+v code %d", j, code)
	}
	j, code = fixJSON(t, p, dir, "apply", "driver", "--yes", "--json")
	if code == 0 || j.OK || !strings.Contains(j.Error, "never changes drivers") {
		t.Fatalf("a refusal has ok false, a plain error and a non-zero code: %+v code %d", j, code)
	}
	j, _ = fixJSON(t, p, dir, "apply", "power-saving", "--json")
	if !j.NeedsAdmin {
		t.Errorf("power saving must say Windows asks for administrator rights: %+v", j)
	}
	out, code := runFix(t, p, dir, "list", "--json")
	if code != 0 || !strings.Contains(out, "name-cache") || !strings.Contains(out, "power-saving") {
		t.Errorf("list --json must name every fix: %d\n%s", code, out)
	}
}

// ND-212, ND-213: the check names the fixes that fit and gives an offer for each, and changes nothing.
func TestCheckNamesFixesAndOffers(t *testing.T) {
	p := &cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	f := healthy()
	f.settings = core.Settings{DNS: []string{"192.168.1.1"}, ProxyOn: true}
	old := newFixSystem
	newFixSystem = func() fix.System { return p }
	defer func() { newFixSystem = old }()
	var out strings.Builder
	if code := Run(context.Background(), []string{"check", "--json"}, f, t.TempDir(), &out); code != 0 {
		t.Fatalf("code %d", code)
	}
	var rep struct {
		Fixes  []string `json:"fixes"`
		Offers []struct {
			Name       string `json:"name"`
			Title      string `json:"title"`
			Now        string `json:"now"`
			After      string `json:"after"`
			NeedsAdmin bool   `json:"needs_admin"`
		} `json:"fix_offers"`
	}
	if err := json.Unmarshal([]byte(out.String()), &rep); err != nil {
		t.Fatal(err)
	}
	if strings.Join(rep.Fixes, ",") != "proxy" || len(rep.Offers) != 1 {
		t.Fatalf("only the proxy fits: %+v", rep)
	}
	o := rep.Offers[0]
	if o.Name != "proxy" || o.Title == "" || !strings.Contains(o.Now, "10.0.0.9:8080") || o.After != "off" || o.NeedsAdmin {
		t.Errorf("offer: %+v", o)
	}
	if p.writes != 0 || !p.proxy.Enabled {
		t.Error("the check must change nothing")
	}
	// A healthy machine names none, and the fields are empty lists, not missing.
	out.Reset()
	Run(context.Background(), []string{"check", "--json"}, healthy(), t.TempDir(), &out)
	if !strings.Contains(out.String(), `"fixes": []`) || !strings.Contains(out.String(), `"fix_offers": []`) {
		t.Errorf("a healthy check lists no fixes:\n%s", out.String())
	}
}

func withProgramDir(t *testing.T, c *Choices) {
	t.Helper()
	d := t.TempDir()
	if c != nil {
		if err := SaveChoices(d, *c); err != nil {
			t.Fatal(err)
		}
	}
	old := programDir
	programDir = func() string { return d }
	t.Cleanup(func() { programDir = old })
}

func sinkList(s []core.Sink) string {
	var parts []string
	for _, x := range s {
		parts = append(parts, string(x))
	}
	return strings.Join(parts, ",")
}

// ND-215, ND-216: the alerts part, when chosen, turns the file and the screen on until the person says otherwise.
func TestAlertSinksFollowTheAlertsChoice(t *testing.T) {
	fresh := func() store.Config {
		c, _ := store.LoadConfig(filepath.Join(t.TempDir(), "config.json"))
		return c
	}
	withProgramDir(t, &Choices{Core: true, Proactive: true})
	if got := sinkList(alertSinks(fresh())); got != "console,file,screen" {
		t.Errorf("alerts chosen, nothing saved: %s", got)
	}
	saved := fresh()
	saved.Sinks, saved.SinksSaved = []string{"console"}, true
	if got := sinkList(alertSinks(saved)); got != "console" {
		t.Errorf("the person's saved choice wins: %s", got)
	}
	withProgramDir(t, &Choices{Core: true})
	if got := sinkList(alertSinks(fresh())); got != "console" {
		t.Errorf("alerts not chosen: %s", got)
	}
	withProgramDir(t, nil)
	if got := sinkList(alertSinks(fresh())); got != "console" {
		t.Errorf("no choices file: %s", got)
	}
}

// ND-215: a settings file written without a choice of places does not count as a saved choice.
func TestSavingOtherSettingsKeepsPlacesUnsaved(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.json")
	c, _ := store.LoadConfig(path)
	c.PlanMbps = 300
	if err := store.SaveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(path)
	if strings.Contains(string(b), "alert_sinks") {
		t.Errorf("no places were chosen, so none are written:\n%s", b)
	}
	back, _ := store.LoadConfig(path)
	if back.SinksSaved || back.PlanMbps != 300 {
		t.Errorf("%+v", back)
	}
	back.Sinks, back.SinksSaved = []string{"console", "file"}, true
	if err := store.SaveConfig(path, back); err != nil {
		t.Fatal(err)
	}
	again, _ := store.LoadConfig(path)
	if !again.SinksSaved || strings.Join(again.Sinks, ",") != "console,file" {
		t.Errorf("a chosen place is saved: %+v", again)
	}
}

// ND-217: setup asks the alerts question only when the alerts part was chosen; Enter means yes.
func TestSetupAlertsQuestion(t *testing.T) {
	run := func(c *Choices, input string) (store.Config, string) {
		withProgramDir(t, c)
		dir := t.TempDir()
		var out strings.Builder
		if code := Setup(context.Background(), threeDevices(), dir, strings.NewReader(input), &out, newRecorder(false).opt); code != 0 {
			t.Fatalf("code %d\n%s", code, out.String())
		}
		cfg, _ := store.LoadConfig(filepath.Join(dir, "config.json"))
		return cfg, out.String()
	}
	const keep, save = "Keep alerts on the screen and in a file", "Save alerts in a file"
	// The answers, in order: plan, name, devices, weekly, alerts, phone.
	cfg, out := run(&Choices{Core: true, Proactive: true}, "300\nSam\n1\nn\n\nn\n")
	if !strings.Contains(out, keep) || strings.Contains(out, save) || strings.Join(cfg.Sinks, ",") != "console,file,screen" || !cfg.SinksSaved {
		t.Errorf("Enter means yes: %v\n%s", cfg.Sinks, out)
	}
	cfg, _ = run(&Choices{Core: true, Proactive: true}, "300\nSam\n1\nn\nn\nn\n")
	if strings.Join(cfg.Sinks, ",") != "console" || !cfg.SinksSaved {
		t.Errorf("n turns them off, and that is remembered: %v saved %v", cfg.Sinks, cfg.SinksSaved)
	}
	cfg, out = run(&Choices{Core: true}, "300\nSam\n1\nn\nn\n")
	if strings.Contains(out, keep) || strings.Contains(out, save) || cfg.SinksSaved {
		t.Errorf("no alerts part, no question: %v\n%s", cfg.Sinks, out)
	}
	cfg, out = run(nil, "300\nSam\n1\nn\ny\nn\n")
	if !strings.Contains(out, save) || strings.Join(cfg.Sinks, ",") != "console,file" {
		t.Errorf("no choices file keeps the earlier question: %v\n%s", cfg.Sinks, out)
	}
}

// refusingPower is a computer whose real system would refuse the power saving fix.
type refusingPower struct{ *cliPretend }

func (refusingPower) PowerSavingDropsLink() bool { return true }

// ND-213: a fix the program would refuse today is not offered, so the card never shows a button that cannot work.
func TestAFixTheProgramWouldRefuseIsNotOffered(t *testing.T) {
	old := newFixSystem
	defer func() { newFixSystem = old }()
	p := &cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	newFixSystem = func() fix.System { return refusingPower{p} }
	got := fixOffers([]string{"power-saving", "proxy"}, t.TempDir())
	if len(got) != 1 || got[0].Name != "proxy" {
		t.Errorf("only the proxy may be offered: %+v", got)
	}
	newFixSystem = func() fix.System { return p }
	if got := fixOffers([]string{"power-saving", "proxy"}, t.TempDir()); len(got) != 2 {
		t.Errorf("when the program can make the fix it is offered: %+v", got)
	}
}

// ND-215: a settings file from an earlier version that lists only the console is not a choice;
// anything else in it is, and a "no" the person says now is remembered.
func TestAnOldSettingsFileWithOnlyTheConsoleIsNotAChoice(t *testing.T) {
	load := func(text string) store.Config {
		path := filepath.Join(t.TempDir(), "config.json")
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
		c, err := store.LoadConfig(path)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	if c := load(`{"plan_mbps":300,"alert_sinks":["console"]}`); c.SinksSaved {
		t.Error("the old default is not a choice")
	}
	if c := load(`{"alert_sinks":["console","file"]}`); !c.SinksSaved {
		t.Error("a list with more than the console is a choice")
	}
	path := filepath.Join(t.TempDir(), "config.json")
	c, _ := store.LoadConfig(path)
	c.Sinks, c.SinksSaved = []string{"console"}, true
	if err := store.SaveConfig(path, c); err != nil {
		t.Fatal(err)
	}
	again, _ := store.LoadConfig(path)
	if !again.SinksSaved || strings.Join(again.Sinks, ",") != "console" {
		t.Errorf("a chosen console-only list stays a choice: %+v", again)
	}
}

// ND-215: a choice of no places at all is still a choice, so turning the last place off sticks.
func TestAnEmptyChosenListStaysAChoice(t *testing.T) {
	withProgramDir(t, &Choices{Core: true, Proactive: true})
	dir := t.TempDir()
	run := func(args ...string) {
		t.Helper()
		var out strings.Builder
		if code := Run(context.Background(), append([]string{"config"}, args...), healthy(), dir, &out); code != 0 {
			t.Fatalf("config %v: %s", args, out.String())
		}
	}
	run("sinks", "file")
	run("alerts-file", "off")
	cfg, _ := store.LoadConfig(filepath.Join(dir, "config.json"))
	if !cfg.SinksSaved {
		t.Fatalf("the person turned the file off: that is a choice: %+v", cfg)
	}
	if got := sinkList(alertSinks(cfg)); got != "console" {
		t.Errorf("an empty chosen list falls back to the console only: %s", got)
	}
}

// ND-215, ND-219: when the alerts part sends alerts to the console, file and screen and nothing is
// saved, switching the file off or on (each in a fresh folder) starts from where alerts really go,
// so the screen is not dropped silently.
func TestAlertsFileSwitchStartsFromWhereAlertsGo(t *testing.T) {
	withProgramDir(t, &Choices{Core: true, Proactive: true})
	dir := t.TempDir()
	var out strings.Builder
	if code := Run(context.Background(), []string{"config", "alerts-file", "off"}, healthy(), dir, &out); code != 0 {
		t.Fatal(out.String())
	}
	cfg, _ := store.LoadConfig(filepath.Join(dir, "config.json"))
	if got := sinkList(alertSinks(cfg)); got != "console,screen" {
		t.Errorf("file off keeps the console and the screen: %s", got)
	}
	dir2 := t.TempDir()
	if code := Run(context.Background(), []string{"config", "alerts-file", "on"}, healthy(), dir2, &out); code != 0 {
		t.Fatal(out.String())
	}
	cfg, _ = store.LoadConfig(filepath.Join(dir2, "config.json"))
	if got := sinkList(alertSinks(cfg)); got != "console,file,screen" {
		t.Errorf("file on keeps everything: %s", got)
	}
}

// ND-213: every way of running the check, the phone page included, leaves refused fixes out of both lists.
func TestCheckItselfLeavesOutAFixTheProgramWouldRefuse(t *testing.T) {
	old := newFixSystem
	defer func() { newFixSystem = old }()
	p := &cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	newFixSystem = func() fix.System { return refusingPower{p} }
	f := healthy()
	f.settings = core.Settings{DNS: []string{"192.168.1.1"}, ProxyOn: true, PowerKnown: true, PowerSaving: true}
	now := time.Now()
	f.events = []core.Event{{Time: now.Add(-2 * time.Hour), Up: false}, {Time: now.Add(-110 * time.Minute), Up: true}}
	rep := Check(context.Background(), f, Options{})
	if strings.Join(rep.Fixes, ",") != "proxy" || len(rep.FixOffers) != 1 || rep.FixOffers[0].Name != "proxy" {
		t.Errorf("Check alone must leave power saving out of both: %v %+v", rep.Fixes, rep.FixOffers)
	}
}

type undoOut struct {
	Fixes []string `json:"fixes"`
	Undos []struct {
		Name       string `json:"name"`
		Title      string `json:"title"`
		Now        string `json:"now"`
		After      string `json:"after"`
		NeedsAdmin bool   `json:"needs_admin"`
	} `json:"undos"`
}

func checkJSON(t *testing.T, f *fake, dir string) undoOut {
	t.Helper()
	var out strings.Builder
	Run(context.Background(), []string{"check", "--json"}, f, dir, &out)
	var rep undoOut
	if err := json.Unmarshal([]byte(out.String()), &rep); err != nil {
		t.Fatalf("%v\n%s", err, out.String())
	}
	if rep.Undos == nil || !strings.Contains(out.String(), `"undos": [`) {
		t.Fatalf("undos must always be a list:\n%s", out.String())
	}
	return rep
}

// ND-220, ND-223: the check lists the undo from its own results folder, and fix undo in the
// same folder puts it back; after that, the next check lists none.
func TestCheckListsUndosFromTheFolderFixUndoUses(t *testing.T) {
	p := &cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	dir := t.TempDir()
	if _, code := runFix(t, p, dir, "apply", "proxy", "--yes"); code != 0 {
		t.Fatal("apply")
	}
	old := newFixSystem
	newFixSystem = func() fix.System { return p }
	defer func() { newFixSystem = old }()
	rep := checkJSON(t, healthy(), dir)
	if len(rep.Undos) != 1 || rep.Undos[0].Name != "proxy" || rep.Undos[0].Now != "off" || !strings.Contains(rep.Undos[0].After, "10.0.0.9:8080") || rep.Undos[0].Title == "" {
		t.Fatalf("the check lists the proxy undo: %+v", rep)
	}
	if out, code := runFix(t, p, dir, "undo", "proxy"); code != 0 || !p.proxy.Enabled {
		t.Fatalf("fix undo in the same folder puts it back: %d %s", code, out)
	}
	if rep := checkJSON(t, healthy(), dir); len(rep.Undos) != 0 {
		t.Errorf("nothing left to put back: %+v", rep.Undos)
	}
	if rep := checkJSON(t, healthy(), t.TempDir()); len(rep.Undos) != 0 {
		t.Errorf("another folder holds no saved value: %+v", rep.Undos)
	}
}

// ND-222: a fix offered to do is never also offered to undo.
func TestAFixIsNeverOfferedToDoAndToUndo(t *testing.T) {
	p := &cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	dir := t.TempDir()
	runFix(t, p, dir, "apply", "proxy", "--yes")
	old := newFixSystem
	newFixSystem = func() fix.System { return p }
	defer func() { newFixSystem = old }()
	if got := fixUndos(dir, []string{"proxy"}); len(got) != 0 {
		t.Errorf("offered as a fix, so no undo: %+v", got)
	}
	p.proxy = fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}
	f := healthy()
	f.settings = core.Settings{DNS: []string{"192.168.1.1"}, ProxyOn: true}
	rep := checkJSON(t, f, dir)
	if strings.Join(rep.Fixes, ",") != "proxy" || len(rep.Undos) != 0 {
		t.Errorf("turned on again by hand: the fix shows and no undo: %+v", rep)
	}
}

// powerOff is a computer whose adapter power saving reads off, which is what the fix leaves.
type powerOff struct{ *cliPretend }

func (powerOff) PowerSaving() (string, error) { return "off", nil }

// ND-220, ND-223, ND-225: a check run without a results folder lists no undos, and a saved power
// saving value is never listed as an undo, even while the setting is what that fix left.
func TestNoFolderNoUndosAndNeverPowerSaving(t *testing.T) {
	old := newFixSystem
	defer func() { newFixSystem = old }()
	p := &cliPretend{proxy: fix.ProxyState{Enabled: false, Server: "10.0.0.9:8080"}}
	newFixSystem = func() fix.System { return powerOff{p} }
	f := healthy()
	f.settings = core.Settings{PowerKnown: true, PowerSaving: false}
	rep := Check(context.Background(), f, Options{})
	if rep.Undos == nil || len(rep.Undos) != 0 {
		t.Errorf("no folder, an empty list: %+v", rep.Undos)
	}
	dir := t.TempDir()
	saved := `{"power-saving": "on", "proxy": "{\"enabled\":true,\"server\":\"10.0.0.9:8080\"}"}`
	if err := os.WriteFile(filepath.Join(dir, "fix-saved.json"), []byte(saved), 0o644); err != nil {
		t.Fatal(err)
	}
	r := checkJSON(t, f, dir)
	if len(r.Undos) != 1 || r.Undos[0].Name != "proxy" {
		t.Errorf("only the proxy is listed; power saving never is: %+v", r.Undos)
	}
	for _, u := range fixUndos(dir, nil) {
		if u.Name == core.FixPowerSaving {
			t.Errorf("power saving is never listed: %+v", u)
		}
	}
}

// ND-224: a second proxy apply by command is refused in plain words with a non-zero code.
func TestProxyApplyRefusedWhenAlreadyOffByCommand(t *testing.T) {
	p := &cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	dir := t.TempDir()
	if _, code := runFix(t, p, dir, "apply", "proxy", "--yes"); code != 0 {
		t.Fatal("first apply")
	}
	writes := p.writes
	j, code := fixJSON(t, p, dir, "apply", "proxy", "--yes", "--json")
	if code == 0 || j.OK || j.Applied || !strings.Contains(j.Error, "already off") || strings.HasPrefix(j.Error, "{") {
		t.Fatalf("a second apply is refused in plain words: %+v code %d", j, code)
	}
	if p.writes != writes {
		t.Errorf("nothing changed: %d -> %d", writes, p.writes)
	}
}

// ND-221: fix undo by command refuses a stale value with a plain reason and a non-zero code.
func TestFixUndoCommandRefusesAStaleValue(t *testing.T) {
	p := &cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	dir := t.TempDir()
	runFix(t, p, dir, "apply", "proxy", "--yes")
	p.proxy = fix.ProxyState{Enabled: true, Server: "new:8080"}
	writes := p.writes
	j, code := fixJSON(t, p, dir, "undo", "proxy", "--json")
	if code == 0 || j.OK || j.Undone || !strings.Contains(j.Error, "changed") || strings.HasPrefix(j.Error, "{") {
		t.Fatalf("a stale undo is refused in plain words: %+v code %d", j, code)
	}
	out, code := runFix(t, p, dir, "undo", "proxy")
	if code == 0 || !strings.Contains(out, "changed") {
		t.Errorf("plain text too: %d %s", code, out)
	}
	if p.writes != writes || p.proxy.Server != "new:8080" {
		t.Errorf("nothing changed: %+v", p)
	}
}
