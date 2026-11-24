package fix

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"
)

// ND-316: UndoAll puts back every recorded fix with Undo itself, reads nothing when nothing is
// recorded, settles a setting changed by hand since the fix, and counts a failed change, a fix it
// is told to skip and unreadable notes as not put back.
func TestUndoAllUsesUndo(t *testing.T) {
	// Nothing recorded: no notes file, or an empty one. Nothing is read or changed.
	for _, notes := range []string{"", "{}"} {
		p := newPretend()
		m, dir := newManager(t, p, Options{})
		if notes != "" {
			os.WriteFile(filepath.Join(dir, "fix-saved.json"), []byte(notes), 0o644)
		}
		if f, s := m.UndoAll(nil); len(f) != 0 || len(s) != 0 || p.proxyWrites != 0 || p.powerWrites != 0 || p.elevations != 0 {
			t.Errorf("notes %q: failures %v, settled %v, pretend %+v", notes, f, s, p)
		}
	}

	// Two fixes applied, both put back.
	p := newPretend()
	m, _ := newManager(t, p, Options{})
	if _, err := m.Apply(proxy, true); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Apply(power, true); err != nil {
		t.Fatal(err)
	}
	if f, s := m.UndoAll(nil); len(f) != 0 || len(s) != 0 {
		t.Fatalf("all should be put back: %v %v", f, s)
	}
	if !p.proxy.Enabled || p.power != "on" || len(m.loadSaved()) != 0 {
		t.Errorf("not put back: %+v, notes %v", p, m.loadSaved())
	}

	// A fix the caller skips is not tried.
	p = newPretend()
	m, _ = newManager(t, p, Options{})
	m.Apply(proxy, true)
	writes := p.proxyWrites
	f, _ := m.UndoAll(func(name string) bool { return name == proxy })
	if len(f) != 1 || f[0].Reason != ReasonAdmin || p.proxyWrites != writes {
		t.Errorf("skipped: %+v, writes %d then %d", f, writes, p.proxyWrites)
	}

	// A change the computer refuses.
	p = newPretend()
	m = NewManager(&failingProxy{p}, t.TempDir(), Options{})
	m.Apply(proxy, true)
	f, _ = m.UndoAll(nil)
	if len(f) != 1 || f[0].Reason != ReasonFailed || f[0].Err == nil {
		t.Errorf("refused by the computer: %+v", f)
	}

	// Notes that cannot be read count as not put back, and are never treated as empty.
	p = newPretend()
	m, dir := newManager(t, p, Options{})
	os.WriteFile(filepath.Join(dir, "fix-saved.json"), []byte("not notes"), 0o644)
	f, _ = m.UndoAll(nil)
	if len(f) != 1 || f[0].Reason != ReasonNotes || p.proxyWrites != 0 {
		t.Errorf("damaged notes: %+v", f)
	}
	if UndoTitle(proxy) != "Turn the web proxy back on" {
		t.Errorf("undo title: %q", UndoTitle(proxy))
	}
}

// ND-316: at removal a setting the person changed by hand since the fix is settled: left as they
// set it, its note dropped, and not a failure. Mixed with a real failure only that one's note stays.
// Undo itself still refuses such a fix.
func TestUndoAllSettlesAChangedSetting(t *testing.T) {
	p := newPretend()
	m, _ := newManager(t, p, Options{})
	m.Apply(proxy, true)
	p.proxy = ProxyState{Enabled: true, Server: "changed.example:1"}
	var r *RefusedError
	if err := m.Undo(proxy); !errors.As(err, &r) {
		t.Fatalf("fix undo must still refuse a changed setting: %v", err)
	}
	writes := p.proxyWrites
	f, s := m.UndoAll(nil)
	if len(f) != 0 || len(s) != 1 || s[0] != proxy {
		t.Errorf("changed by hand: failures %+v, settled %v", f, s)
	}
	if p.proxy.Server != "changed.example:1" || p.proxyWrites != writes {
		t.Errorf("the person's own setting must win: %+v", p.proxy)
	}
	if _, ok := m.loadSaved()[proxy]; ok {
		t.Error("a settled fix's note must be dropped")
	}

	// Mixed: a changed proxy and power saving that will not go back.
	p = newPretend()
	m = NewManager(&stuckPower{p}, t.TempDir(), Options{})
	m.Apply(proxy, true)
	m.Apply(power, true)
	p.proxy = ProxyState{Enabled: true, Server: "changed.example:1"}
	f, s = m.UndoAll(nil)
	if len(f) != 1 || f[0].Name != power || len(s) != 1 || s[0] != proxy {
		t.Errorf("mixed: failures %+v, settled %v", f, s)
	}
	notes := m.loadSaved()
	if _, ok := notes[proxy]; ok {
		t.Error("mixed: the settled proxy note must be dropped")
	}
	if _, ok := notes[power]; !ok {
		t.Error("mixed: the failed power note must stay")
	}
}

// ND-316: the name servers count as changed by hand only when every adapter the fix changed is
// still there and its servers differ from what the fix set. An adapter that is gone (offline, or
// another adapter in use) or a setting that cannot be read stays a real failure, its note kept.
func TestUndoAllPublicDNSSettledOnlyWhenTheAdapterIsThere(t *testing.T) {
	setup := func(t *testing.T, sys func(*ndExtra) System) (*ndExtra, *Manager) {
		p := newNDExtra()
		m := NewManager(sys(p), t.TempDir(), Options{})
		if _, err := m.Apply(ndPublicDNS, true); err != nil {
			t.Fatal(err)
		}
		return p, m
	}
	plain := func(p *ndExtra) System { return p }

	// Present, and its servers changed by hand: settled.
	p, m := setup(t, plain)
	for i := range p.dns {
		p.dns[i].Servers, p.dns[i].Automatic = []string{"9.9.9.9"}, false
	}
	calls := p.setCalls
	if f, s := m.UndoAll(nil); len(f) != 0 || len(s) != 1 || s[0] != ndPublicDNS || p.setCalls != calls {
		t.Errorf("present and changed: failures %+v settled %v", f, s)
	}

	// The adapter is gone from the list: a failure, nothing changed, the note kept.
	p, m = setup(t, plain)
	p.dns = []DNSState{{Adapter: "Other", IfIndex: 99, Servers: []string{"10.9.9.9"}, InUse: true}}
	calls = p.setCalls
	f, s := m.UndoAll(nil)
	if len(f) != 1 || f[0].Name != ndPublicDNS || f[0].Reason != ReasonFailed || len(s) != 0 || p.setCalls != calls {
		t.Errorf("adapter gone: failures %+v settled %v", f, s)
	}
	if _, ok := m.loadSaved()[ndPublicDNS]; !ok {
		t.Error("adapter gone: the note must stay")
	}

	// No adapter listed at all (offline): a failure too.
	p, m = setup(t, plain)
	p.dns = nil
	if f, s := m.UndoAll(nil); len(f) != 1 || len(s) != 0 {
		t.Errorf("offline: failures %+v settled %v", f, s)
	}

	// The servers cannot be read: a failure.
	_, m = setup(t, func(p *ndExtra) System { return &dnsUnreadable{ndExtra: p} })
	m.sys.(*dnsUnreadable).broken = true
	if f, s := m.UndoAll(nil); len(f) != 1 || len(s) != 0 {
		t.Errorf("unreadable: failures %+v settled %v", f, s)
	}
}

// ND-316: a proxy that cannot be read stays a real failure, never settled.
func TestUndoAllProxyReadErrorIsAFailure(t *testing.T) {
	p := newPretend()
	sys := &proxyUnreadable{pretend: p}
	m := NewManager(sys, t.TempDir(), Options{})
	m.Apply(proxy, true)
	sys.broken = true
	f, s := m.UndoAll(nil)
	if len(f) != 1 || f[0].Reason != ReasonFailed || len(s) != 0 {
		t.Errorf("unreadable proxy: failures %+v settled %v", f, s)
	}
	if _, ok := m.loadSaved()[proxy]; !ok {
		t.Error("the note must stay")
	}
}

// ND-316: whether a refusal means "changed by hand" is a mark on the refusal, never its words.
func TestSettledIsAMarkNotWords(t *testing.T) {
	if changedByHand(&RefusedError{Name: proxy, Reason: "The setting was changed since the fix."}) {
		t.Error("words alone must not make a refusal settled")
	}
	if !changedByHand(fmt.Errorf("wrapped: %w", &RefusedError{Name: proxy, Reason: "Reworded.", ChangedByHand: true})) {
		t.Error("the mark must make it settled, whatever the words")
	}
	if changedByHand(errors.New("changed since the fix")) {
		t.Error("a plain error is never settled")
	}
}

// ND-316: dropping one note never wipes the others when the notes cannot be read again.
func TestDropSavedWritesNothingOnAReadFault(t *testing.T) {
	m, dir := newManager(t, newPretend(), Options{})
	path := filepath.Join(dir, "fix-saved.json")
	os.WriteFile(path, []byte("not notes"), 0o644)
	if err := m.dropSaved(proxy); err == nil {
		t.Error("a notes file that cannot be read must be reported")
	}
	if b, _ := os.ReadFile(path); string(b) != "not notes" {
		t.Errorf("the notes were written over: %q", b)
	}
}

// dnsUnreadable is a computer whose name servers cannot be read once broken.
type dnsUnreadable struct {
	*ndExtra
	broken bool
}

func (d *dnsUnreadable) DNS() ([]DNSState, error) {
	if d.broken {
		return nil, errNoChange
	}
	return d.ndExtra.DNS()
}

// proxyUnreadable is a computer whose proxy cannot be read once broken.
type proxyUnreadable struct {
	*pretend
	broken bool
}

func (p *proxyUnreadable) Proxy() (ProxyState, error) {
	if p.broken {
		return ProxyState{}, errNoChange
	}
	return p.pretend.Proxy()
}

// failingProxy is a computer that will not change the proxy back.
type failingProxy struct{ *pretend }

var errNoChange = errors.New("the computer said no")

func (f *failingProxy) SetProxy(s ProxyState) error {
	if !s.Enabled {
		return f.pretend.SetProxy(s) // the fix itself
	}
	return errNoChange
}

// stuckPower is a computer that turns power saving off but never back on.
type stuckPower struct{ *pretend }

func (s *stuckPower) SetPowerSaving(v string) error {
	if v == "off" {
		return s.pretend.SetPowerSaving(v)
	}
	return errNoChange
}
