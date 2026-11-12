package fix

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// pretend is a pretend computer. Nothing here touches the real one.
type pretend struct {
	flushes     int
	proxy       ProxyState
	proxyWrites int
	power       string // "on" or "off"
	powerWrites int
	powerDrops  bool // true if turning it off would drop the link in use
	elevations  int
}

func newPretend() *pretend {
	return &pretend{proxy: ProxyState{Enabled: true, Server: "10.0.0.9:8080"}, power: "on"}
}

func (p *pretend) FlushDNS() error              { p.flushes++; return nil }
func (p *pretend) Proxy() (ProxyState, error)   { return p.proxy, nil }
func (p *pretend) SetProxy(s ProxyState) error  { p.proxyWrites++; p.proxy = s; return nil }
func (p *pretend) PowerSaving() (string, error) { return p.power, nil }
func (p *pretend) SetPowerSaving(v string) error {
	p.powerWrites++
	p.power = v
	return nil
}
func (p *pretend) PowerSavingDropsLink() bool { return p.powerDrops }
func (p *pretend) Elevate() error             { p.elevations++; return nil }

func newManager(t *testing.T, p *pretend, o Options) (*Manager, string) {
	t.Helper()
	dir := t.TempDir()
	return NewManager(p, dir, o), dir
}

// ND-90: nothing changes without approval; only the approved fix changes.
func TestNothingChangesWithoutApproval(t *testing.T) {
	p := newPretend()
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
	p := newPretend()
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
	p := newPretend()
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
	p := newPretend()
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
	p := newPretend()
	m, _ := newManager(t, p, Options{})
	m.Apply("name-cache", true)
	if err := m.Undo("name-cache"); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("want ErrNothingToUndo, got %v", err)
	}
}

// ND-93: every change and every undo is logged with time, setting, old and new value.
func TestEveryChangeAndUndoIsLogged(t *testing.T) {
	p := newPretend()
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
	p := newPretend()
	m, dir := newManager(t, p, Options{})
	m.Apply("proxy", false)
	m.Apply("driver", true)
	if _, err := os.Stat(filepath.Join(dir, "fix-log.txt")); err == nil {
		t.Error("nothing was changed, so nothing may be logged as a change")
	}
}

// ND-94: administrator rights are asked for only at the approval, once, and never to look.
func TestAdministratorOnlyAtApproval(t *testing.T) {
	p := newPretend()
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
	p := newPretend()
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
	p := newPretend()
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
	p := newPretend()
	m, _ := newManager(t, p, Options{})
	if _, err := m.Apply("name-cache", true); err != nil {
		t.Errorf("%v", err)
	}
}

// Text shown to people is plain English (hard rule 4): no leftover codes or jargon in refusals.
func TestRefusalsUsePlainWords(t *testing.T) {
	p := newPretend()
	m, _ := newManager(t, p, Options{})
	_, err := m.Apply("driver", true)
	if err == nil || strings.Contains(strings.ToLower(err.Error()), "errno") || strings.Contains(err.Error(), "0x") {
		t.Errorf("refusal should be plain words: %v", err)
	}
}
