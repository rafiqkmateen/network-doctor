package fix

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// simulated is a simulated computer. Nothing here touches the real one.
type simulated struct {
	flushes     int
	proxy       ProxyState
	proxyWrites int
	power       string // "on" or "off"
	powerWrites int
	powerDrops  bool  // true if turning it off would drop the link in use
	powerErr    error // the driver will not tell us the setting
	elevations  int
}

func newSimulated() *simulated {
	return &simulated{proxy: ProxyState{Enabled: true, Server: "10.0.0.9:8080"}, power: "on"}
}

func (p *simulated) FlushDNS() error              { p.flushes++; return nil }
func (p *simulated) Proxy() (ProxyState, error)   { return p.proxy, nil }
func (p *simulated) SetProxy(s ProxyState) error  { p.proxyWrites++; p.proxy = s; return nil }
func (p *simulated) PowerSaving() (string, error) { return p.power, p.powerErr }
func (p *simulated) SetPowerSaving(v string) error {
	p.powerWrites++
	p.power = v
	return nil
}
func (p *simulated) PowerSavingDropsLink() bool { return p.powerDrops }
func (p *simulated) Elevate() error             { p.elevations++; return nil }

func newManager(t *testing.T, p *simulated, o Options) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	return NewManager(p, dir, o), dir
}

// ND-90: nothing changes without approval; only the approved fix changes.
func TestNothingChangesWithoutApproval(t *testing.T) {
	p := newSimulated()
	m, _ := newManager(t, p, Options{})
	if _, err := m.Apply("proxy", false); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("want ErrNotApproved, got %v", err)
	}
	if p.proxyWrites != 0 || p.flushes != 0 || p.powerWrites != 0 {
		t.Fatalf("nothing may change without approval: %+v", p)
	}
	if _, err := m.Apply("proxy", true); err != nil {
		t.Fatal(err)
	}
	if p.proxy.Enabled || p.proxyWrites != 1 {
		t.Errorf("the approved fix must change the proxy: %+v", p.proxy)
	}
	if p.flushes != 0 || p.powerWrites != 0 {
		t.Errorf("other fixes must stay untouched: %+v", p)
	}
}

// ND-90 (fix 1): the name cache is cleared only on approval.
func TestFlushNameCache(t *testing.T) {
	p := newSimulated()
	m, _ := newManager(t, p, Options{})
	m.Apply("name-cache", false)
	if p.flushes != 0 {
		t.Fatal("cleared without approval")
	}
	if _, err := m.Apply("name-cache", true); err != nil || p.flushes != 1 {
		t.Fatalf("err %v, flushes %d", err, p.flushes)
	}
}

// ND-91: the before and after values are shown before anything is applied.
func TestPreviewShowsBeforeAndAfter(t *testing.T) {
	p := newSimulated()
	m, _ := newManager(t, p, Options{})
	pl, err := m.Preview("proxy")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pl.Before, "10.0.0.9:8080") || pl.After == "" || pl.Before == pl.After {
		t.Errorf("preview must show a different before and after: %+v", pl)
	}
	pw, _ := m.Preview("power-saving")
	if pw.Before == "" || pw.After == "" || pw.Before == pw.After {
		t.Errorf("power saving preview: %+v", pw)
	}
	if p.proxyWrites != 0 || p.powerWrites != 0 {
		t.Error("a preview must change nothing")
	}
	for _, in := range m.List() {
		if in.Name == "" || in.Title == "" || in.Now == "" {
			t.Errorf("list entry is incomplete: %+v", in)
		}
	}
}

// ND-92: apply, then undo with one action; the setting returns to the original value.
func TestUndoPutsBackTheOldValue(t *testing.T) {
	p := newSimulated()
	m, _ := newManager(t, p, Options{})
	if _, err := m.Apply("proxy", true); err != nil {
		t.Fatal(err)
	}
	// A new manager proves the old value was saved to disk, not kept in memory.
	m2 := NewManager(p, m.Dir(), Options{})
	if err := m2.Undo("proxy"); err != nil {
		t.Fatal(err)
	}
	if !p.proxy.Enabled || p.proxy.Server != "10.0.0.9:8080" {
		t.Errorf("proxy not restored: %+v", p.proxy)
	}
	if err := m2.Undo("proxy"); err == nil {
		t.Error("a second undo has nothing to put back and must say so")
	}

	if _, err := m.Apply("power-saving", true); err != nil {
		t.Fatal(err)
	}
	if p.power != "off" {
		t.Fatalf("power saving not turned off: %s", p.power)
	}
	if err := m.Undo("power-saving"); err != nil || p.power != "on" {
		t.Errorf("err %v, power %s", err, p.power)
	}
}

// ND-92: the name cache holds nothing of the user's, so undo says there is nothing to put back.
func TestNameCacheHasNothingToUndo(t *testing.T) {
	p := newSimulated()
	m, _ := newManager(t, p, Options{})
	m.Apply("name-cache", true)
	if err := m.Undo("name-cache"); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("want ErrNothingToUndo, got %v", err)
	}
}

// ND-93: every change and every undo is logged with time, setting, old and new value.
func TestEveryChangeAndUndoIsLogged(t *testing.T) {
	p := newSimulated()
	m, dir := newManager(t, p, Options{})
	m.Apply("proxy", true)
	m.Undo("proxy")
	b, err := os.ReadFile(filepath.Join(dir, "fix-log.txt"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 log lines, got %d: %q", len(lines), lines)
	}
	for i, l := range lines {
		f := strings.Split(l, "\t")
		if len(f) != 5 {
			t.Fatalf("line %d needs time, action, setting, old, new: %q", i, l)
		}
		if f[2] != "proxy" {
			t.Errorf("setting column: %q", f[2])
		}
	}
	if !strings.Contains(lines[0], "apply") || !strings.Contains(lines[1], "undo") {
		t.Errorf("actions in order: %q", lines)
	}
	if !strings.Contains(lines[0], "10.0.0.9:8080") {
		t.Errorf("the old value belongs in the log: %q", lines[0])
	}
}

// ND-93: a refused or unapproved request writes nothing to the log.
func TestRefusedRequestsAreNotLoggedAsChanges(t *testing.T) {
	p := newSimulated()
	m, dir := newManager(t, p, Options{})
	m.Apply("proxy", false)
	m.Apply("driver", true)
	if _, err := os.Stat(filepath.Join(dir, "fix-log.txt")); err == nil {
		t.Error("nothing was changed, so nothing may be logged as a change")
	}
}

// ND-94: administrator rights are asked for only at the approval, once, and never to look.
func TestAdministratorOnlyAtApproval(t *testing.T) {
	p := newSimulated()
	m, _ := newManager(t, p, Options{})
	m.List()
	m.Preview("power-saving")
	m.Apply("power-saving", false)
	if p.elevations != 0 {
		t.Fatalf("asked for administrator rights %d times before any yes", p.elevations)
	}
	m.Apply("power-saving", true)
	if p.elevations != 1 {
		t.Errorf("want one request at the yes, got %d", p.elevations)
	}
	m.Apply("proxy", true)
	m.Apply("name-cache", true)
	if p.elevations != 1 {
		t.Errorf("fixes that need no administrator rights must not ask: %d", p.elevations)
	}
}

// ND-95: drivers, security protections and changes that cut the link in use are refused with a reason.
func TestUnsafeFixesAreRefused(t *testing.T) {
	p := newSimulated()
	m, _ := newManager(t, p, Options{})
	for _, name := range []string{"driver", "firewall", "security", "dns-servers"} {
		_, err := m.Apply(name, true)
		var r *RefusedError
		if !errors.As(err, &r) || r.Reason == "" {
			t.Errorf("%s: want a refusal with a reason, got %v", name, err)
		}
	}
	// A real fix that would cut the link in use is refused too.
	p.powerDrops = true
	_, err := m.Apply("power-saving", true)
	var r *RefusedError
	if !errors.As(err, &r) || !strings.Contains(r.Reason, "by hand") {
		t.Errorf("want a refusal that says what to change by hand, got %v", err)
	}
	if p.powerWrites != 0 || p.elevations != 0 {
		t.Errorf("a refused fix must not change anything or ask for rights: %+v", p)
	}
}

// ND-96: on an unattended server, apply is off until the owner switches it on for that machine.
func TestServerModeIsOffByDefault(t *testing.T) {
	p := newSimulated()
	m, dir := newManager(t, p, Options{Unattended: true})
	_, err := m.Apply("proxy", true)
	if !errors.Is(err, ErrServerOff) {
		t.Fatalf("want ErrServerOff, got %v", err)
	}
	if p.proxyWrites != 0 {
		t.Fatal("changed while off")
	}
	if err := m.EnableHere(); err != nil {
		t.Fatal(err)
	}
	// The switch lives in this machine's results folder, so a new manager sees it.
	m2 := NewManager(p, dir, Options{Unattended: true})
	if _, err := m2.Apply("proxy", true); err != nil {
		t.Errorf("after the switch, apply should work: %v", err)
	}
	other := NewManager(p, t.TempDir(), Options{Unattended: true})
	if _, err := other.Apply("name-cache", true); !errors.Is(err, ErrServerOff) {
		t.Errorf("the switch is per machine, not global: %v", err)
	}
}

// A person at the keyboard needs no switch.
func TestAttendedComputerNeedsNoSwitch(t *testing.T) {
	p := newSimulated()
	m, _ := newManager(t, p, Options{})
	if _, err := m.Apply("name-cache", true); err != nil {
		t.Errorf("%v", err)
	}
}

// Found on a real machine: a wired adapter has the setting, but its driver will not
// let Windows read it. The tool must say so and give the steps, not claim there is no setting.
func TestUnreadablePowerSettingGivesSteps(t *testing.T) {
	p := newSimulated()
	p.powerErr = errors.New("a device attached to the system is not functioning")
	m, _ := newManager(t, p, Options{})
	pl, err := m.Preview("power-saving")
	if err != nil {
		t.Fatalf("an unreadable setting is not an error to show a person: %v", err)
	}
	all := pl.Before + " " + pl.After
	for _, want := range []string{"cannot read", "Device Manager", "Power Management", "turn off this device to save power"} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %q in %q", want, all)
		}
	}
	if strings.Contains(strings.ToLower(all), "does not report") || strings.Contains(strings.ToLower(all), "no such setting") {
		t.Errorf("must not claim the adapter has no setting: %q", all)
	}
	_, err = m.Apply("power-saving", true)
	var r *RefusedError
	if !errors.As(err, &r) || !strings.Contains(r.Reason, "Device Manager") {
		t.Errorf("apply must refuse with the steps, got %v", err)
	}
	if p.powerWrites != 0 || p.elevations != 0 {
		t.Errorf("nothing may change or ask for rights: %+v", p)
	}
	for _, in := range m.List() {
		if in.Name == "power-saving" && !strings.Contains(in.Now, "cannot read") {
			t.Errorf("the list should say it cannot read the setting: %q", in.Now)
		}
	}
}

// Text shown to people is plain English (hard rule 4): no leftover codes or jargon in refusals.
func TestRefusalsUsePlainWords(t *testing.T) {
	p := newSimulated()
	m, _ := newManager(t, p, Options{})
	_, err := m.Apply("driver", true)
	if err == nil || strings.Contains(strings.ToLower(err.Error()), "errno") || strings.Contains(err.Error(), "0x") {
		t.Errorf("refusal should be plain words: %v", err)
	}
}

// ND-220: the undo list names a fix only while the setting still is what the fix left,
// and making the list changes nothing, not even the saved values file.
func TestUndosListOnlyWhatTheFixLeft(t *testing.T) {
	p := newSimulated()
	m, dir := newManager(t, p, Options{})
	if got := m.Undos(); len(got) != 0 {
		t.Fatalf("no fix made, nothing to put back: %+v", got)
	}
	if _, err := m.Apply("name-cache", true); err != nil {
		t.Fatal(err)
	}
	if got := m.Undos(); len(got) != 0 {
		t.Fatalf("the name cache has nothing to put back: %+v", got)
	}
	if _, err := m.Apply("proxy", true); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(filepath.Join(dir, "fix-saved.json"))
	writes := p.proxyWrites
	got := m.Undos()
	if len(got) != 1 || got[0].Name != "proxy" || got[0].Now != "off" || got[0].After != "on, using 10.0.0.9:8080" || got[0].Title == "" || got[0].NeedsAdmin {
		t.Fatalf("one undo for the proxy, with now and after: %+v", got)
	}
	if got[0].Title == Title("proxy") {
		t.Errorf("the undo needs its own title, not the fix's: %q", got[0].Title)
	}
	p.proxy = ProxyState{Enabled: true, Server: "10.0.0.9:8080"}
	if got := m.Undos(); len(got) != 0 {
		t.Errorf("turned on again by hand: no undo: %+v", got)
	}
	p.proxy = ProxyState{Enabled: false, Server: "other:3128"}
	if got := m.Undos(); len(got) != 0 {
		t.Errorf("another server since the fix: no undo: %+v", got)
	}
	p.proxy = ProxyState{Enabled: false, Server: "10.0.0.9:8080"}
	if got := m.Undos(); len(got) != 1 {
		t.Errorf("back to what the fix left: listed again: %+v", got)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "fix-saved.json"))
	if p.proxyWrites != writes || string(after) != string(saved) {
		t.Errorf("listing must change nothing: writes %d->%d, saved %q -> %q", writes, p.proxyWrites, saved, after)
	}
	if got := NewManager(p, "", Options{}).Undos(); len(got) != 0 {
		t.Errorf("with no folder known the list is empty: %+v", got)
	}
}

// ND-221: undo refuses a setting changed since the fix, and then changes, asks and logs nothing.
func TestUndoRefusesAStaleValue(t *testing.T) {
	p := newSimulated()
	m, dir := newManager(t, p, Options{})
	if _, err := m.Apply("proxy", true); err != nil {
		t.Fatal(err)
	}
	p.proxy = ProxyState{Enabled: true, Server: "new:8080"}
	writes, elev := p.proxyWrites, p.elevations
	err := m.Undo("proxy")
	var r *RefusedError
	if !errors.As(err, &r) || !strings.Contains(err.Error(), "changed") {
		t.Fatalf("want a plain refusal, got %v", err)
	}
	if p.proxyWrites != writes || p.elevations != elev || p.proxy.Server != "new:8080" || !p.proxy.Enabled {
		t.Errorf("a refused undo changes nothing: %+v", p)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "fix-log.txt"))
	if strings.Contains(string(b), "undo") {
		t.Errorf("a refused undo is not logged: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "fix-saved.json")); err != nil {
		t.Errorf("the saved value stays on file: %v", err)
	}
}

// ND-224: the proxy fix is refused when the proxy is already off, so a second apply never
// replaces the only saved old value; the refusal saves, changes and logs nothing.
func TestProxyFixIsRefusedWhenAlreadyOff(t *testing.T) {
	p := newSimulated()
	m, dir := newManager(t, p, Options{})
	if _, err := m.Apply("proxy", true); err != nil {
		t.Fatal(err)
	}
	saved, _ := os.ReadFile(filepath.Join(dir, "fix-saved.json"))
	logged, _ := os.ReadFile(filepath.Join(dir, "fix-log.txt"))
	writes := p.proxyWrites
	_, err := m.Apply("proxy", true)
	var r *RefusedError
	if !errors.As(err, &r) || !strings.Contains(err.Error(), "already off") {
		t.Fatalf("a second apply must be refused in plain words, got %v", err)
	}
	if strings.Contains(err.Error(), "0x") || strings.Contains(strings.ToLower(err.Error()), "errno") {
		t.Errorf("plain words only: %v", err)
	}
	after, _ := os.ReadFile(filepath.Join(dir, "fix-saved.json"))
	logAfter, _ := os.ReadFile(filepath.Join(dir, "fix-log.txt"))
	if string(after) != string(saved) || p.proxyWrites != writes || string(logAfter) != string(logged) {
		t.Errorf("a refused apply saves, changes and logs nothing: saved %q -> %q, writes %d -> %d", saved, after, writes, p.proxyWrites)
	}
	if n := strings.Count(string(logAfter), "\tapply\t"); n != 1 {
		t.Errorf("one apply line, got %d: %q", n, logAfter)
	}
	if err := m.Undo("proxy"); err != nil || !p.proxy.Enabled || p.proxy.Server != "10.0.0.9:8080" {
		t.Errorf("the first saved value still puts back: %v %+v", err, p.proxy)
	}

	q := newSimulated()
	q.proxy = ProxyState{Enabled: false, Server: "old:3128"}
	m2, dir2 := newManager(t, q, Options{})
	if _, err := m2.Apply("proxy", true); !errors.As(err, &r) {
		t.Errorf("off from the start: refused, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir2, "fix-saved.json")); err == nil || q.proxyWrites != 0 {
		t.Errorf("off from the start: nothing saved, nothing changed (writes %d)", q.proxyWrites)
	}
	if _, err := os.Stat(filepath.Join(dir2, "fix-log.txt")); err == nil {
		t.Errorf("off from the start: nothing logged")
	}
}

// ND-225: a saved power saving value is never listed as an undo; power saving is never offered.
func TestUndosNeverListPowerSaving(t *testing.T) {
	p := newSimulated()
	m, dir := newManager(t, p, Options{})
	if _, err := m.Apply("power-saving", true); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "fix-saved.json"))
	if !strings.Contains(string(b), `"power-saving": "on"`) || p.power != "off" {
		t.Fatalf("the simulated computer must hold a saved power value and power off now: %s %s", b, p.power)
	}
	if got := m.Undos(); len(got) != 0 {
		t.Errorf("power saving is never listed: %+v", got)
	}
	if _, err := m.Apply("proxy", true); err != nil {
		t.Fatal(err)
	}
	if got := m.Undos(); len(got) != 1 || got[0].Name != "proxy" {
		t.Errorf("only the proxy is listed beside a saved power value: %+v", got)
	}
}

// ND-221: undo's refusal says what the setting is now and what the fix left, the server too.
func TestUndoRefusalSaysNowAndWhatTheFixLeft(t *testing.T) {
	p := newSimulated()
	m, _ := newManager(t, p, Options{})
	if _, err := m.Apply("proxy", true); err != nil {
		t.Fatal(err)
	}
	p.proxy = ProxyState{Enabled: false, Server: "other:3128"}
	err := m.Undo("proxy")
	if err == nil || !strings.Contains(err.Error(), "changed") || !strings.Contains(err.Error(), "other:3128") || !strings.Contains(err.Error(), "10.0.0.9:8080") {
		t.Errorf("off both times, but the server differs: the words must name both servers: %v", err)
	}
	p.proxy = ProxyState{Enabled: true, Server: "new:8080"}
	err = m.Undo("proxy")
	if err == nil || !strings.Contains(err.Error(), "on, using new:8080") || !strings.Contains(err.Error(), "10.0.0.9:8080") {
		t.Errorf("turned on with a new server: the words must say so and what the fix left: %v", err)
	}
}
