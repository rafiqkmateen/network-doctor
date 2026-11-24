package fix

// Checks for the round-two fixes (ND-235 to ND-238) and the read-back rule (ND-243), written
// before the build. Everything runs on a simulated computer.

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// ndExtra is a simulated Windows computer that has the round-two abilities.
type ndExtra struct {
	*simulated
	dns      []DNSState
	setCalls int
	setErr   error // Windows refused the change
	stuck    bool  // SetDNS says yes but nothing changes (the read-back must catch it)
	renews   int
	renewErr error
	resets   int
	resetErr error
	elevErr  error
}

func ndCopy(in []DNSState) []DNSState {
	out := make([]DNSState, len(in))
	for i, s := range in {
		s.Servers = append([]string(nil), s.Servers...)
		out[i] = s
	}
	return out
}

func (p *ndExtra) DNS() ([]DNSState, error) { return ndCopy(p.dns), nil }
func (p *ndExtra) SetDNS(in []DNSState) error {
	p.setCalls++
	if p.setErr != nil {
		return p.setErr
	}
	if p.stuck {
		return nil
	}
	for _, s := range ndCopy(in) {
		for i := range p.dns {
			if p.dns[i].IfIndex == s.IfIndex {
				s.InUse = p.dns[i].InUse // which adapter carries the connection does not change with its servers
				p.dns[i] = s
			}
		}
	}
	return nil
}
func (p *ndExtra) Renew() error        { p.renews++; return p.renewErr }
func (p *ndExtra) ResetAdapter() error { p.resets++; return p.resetErr }
func (p *ndExtra) Elevate() error {
	if p.elevErr != nil {
		return p.elevErr
	}
	p.simulated.elevations++
	return nil
}

func ndOriginalDNS() []DNSState {
	return []DNSState{
		{Adapter: "Wi-Fi", IfIndex: 12, Servers: []string{"192.168.1.1"}, Automatic: true, InUse: true},
		{Adapter: "Ethernet", IfIndex: 7, Servers: []string{"10.0.0.53", "10.0.0.54"}, Automatic: false},
	}
}

func newNDExtra() *ndExtra { return &ndExtra{simulated: newSimulated(), dns: ndOriginalDNS()} }

func ndByIndex(in []DNSState) map[int]DNSState {
	m := map[int]DNSState{}
	for _, s := range in {
		m[s.IfIndex] = s
	}
	return m
}

func ndOnCloudflare(in []DNSState) int {
	n := 0
	for _, s := range in {
		if reflect.DeepEqual(s.Servers, []string{"1.1.1.1", "1.0.0.1"}) && !s.Automatic {
			n++
		}
	}
	return n
}

func ndListed(m *Manager, name string) bool {
	for _, u := range m.Undos() {
		if u.Name == name {
			return true
		}
	}
	return false
}

// ND-235: the before and after view says where lookups go.
func TestND235PreviewSaysThirdParty(t *testing.T) {
	p := newNDExtra()
	m2 := NewManager(p, t.TempDir(), Options{})
	pl, err := m2.Preview(ndPublicDNS)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(Title(ndPublicDNS) + " " + pl.Before + " " + pl.After)
	if !strings.Contains(pl.After, "1.1.1.1") || !strings.Contains(text, "third") {
		t.Errorf("the offer must name 1.1.1.1 and say lookups go to a third party: %q / %+v", Title(ndPublicDNS), pl)
	}
	if !strings.Contains(pl.Before, "192.168.1.1") && !strings.Contains(strings.ToLower(pl.Before), "automatic") {
		t.Errorf("before must show the servers now: %+v", pl)
	}
	if p.setCalls != 0 || p.elevations != 0 {
		t.Error("a preview changes nothing and asks for nothing")
	}
}

// ND-235, ND-243: yes, administrator, save, set Cloudflare, clear the list, log; undo puts back exactly.
func TestND235ApplyAndUndoRestoreExactly(t *testing.T) {
	p := newNDExtra()
	dir := t.TempDir()
	m := NewManager(p, dir, Options{})
	if _, err := m.Apply(ndPublicDNS, false); !errors.Is(err, ErrNotApproved) {
		t.Fatalf("no yes: want ErrNotApproved, got %v", err)
	}
	if p.setCalls != 0 || p.elevations != 0 || p.flushes != 0 {
		t.Fatal("nothing may happen without a yes")
	}
	if _, err := m.Apply(ndPublicDNS, true); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if p.elevations < 1 {
		t.Error("public-dns must ask Windows for administrator permission")
	}
	if ndOnCloudflare(p.dns) < 1 {
		t.Errorf("the adapter in use must be set to 1.1.1.1 and 1.0.0.1: %+v", p.dns)
	}
	if p.flushes != 1 {
		t.Errorf("the saved web addresses must be cleared once, got %d", p.flushes)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "fix-log.txt")); !strings.Contains(string(b), ndPublicDNS) {
		t.Error("the change must be logged")
	}
	if !ndListed(m, ndPublicDNS) {
		t.Error("the undo must be listed, as for the proxy fix")
	}
	before := p.simulated.elevations
	if err := m.Undo(ndPublicDNS); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if !reflect.DeepEqual(ndByIndex(p.dns), ndByIndex(ndOriginalDNS())) {
		t.Errorf("undo must put back exactly the saved servers:\n got %+v\nwant %+v", p.dns, ndOriginalDNS())
	}
	if p.simulated.elevations <= before {
		t.Error("undo also needs administrator permission")
	}
	if ndListed(m, ndPublicDNS) {
		t.Error("after the undo nothing is left to put back")
	}
}

// ND-235 data loss: applying twice must not replace the first saved servers.
func TestND235ApplyTwiceKeepsTheFirstSave(t *testing.T) {
	p := newNDExtra()
	m := NewManager(p, t.TempDir(), Options{})
	if _, err := m.Apply(ndPublicDNS, true); err != nil {
		t.Fatal(err)
	}
	_, _ = m.Apply(ndPublicDNS, true) // may be refused; either way the first save must stay
	if err := m.Undo(ndPublicDNS); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if !reflect.DeepEqual(ndByIndex(p.dns), ndByIndex(ndOriginalDNS())) {
		t.Errorf("after two applies the undo must still restore the first servers: %+v", p.dns)
	}
}

// ND-235: undo refuses when the servers were changed since the fix, and changes nothing.
func TestND235UndoRefusesWhenChangedSince(t *testing.T) {
	p := newNDExtra()
	m := NewManager(p, t.TempDir(), Options{})
	if _, err := m.Apply(ndPublicDNS, true); err != nil {
		t.Fatal(err)
	}
	changed := false
	for i := range p.dns {
		if reflect.DeepEqual(p.dns[i].Servers, []string{"1.1.1.1", "1.0.0.1"}) {
			p.dns[i].Servers = []string{"9.9.9.9"}
			changed = true
		}
	}
	if !changed {
		t.Fatal("no adapter was on 1.1.1.1 after the fix")
	}
	want := ndCopy(p.dns)
	calls := p.setCalls
	err := m.Undo(ndPublicDNS)
	var ref *RefusedError
	if !errors.As(err, &ref) {
		t.Fatalf("want a plain refusal, got %v", err)
	}
	if p.setCalls != calls || !reflect.DeepEqual(p.dns, want) {
		t.Errorf("a refused undo must change nothing: %+v", p.dns)
	}
	if ndListed(m, ndPublicDNS) {
		t.Error("a stale saved value must not be listed as an undo")
	}
}

// ND-243: a refusal by Windows, or a change that is not there when read back, is a failure.
func TestND243PublicDNSReadBack(t *testing.T) {
	p := newNDExtra()
	p.setErr = errors.New("access denied")
	m := NewManager(p, t.TempDir(), Options{})
	if r, err := m.Apply(ndPublicDNS, true); err == nil || r.Name != "" {
		t.Errorf("Windows refused: must report failure, got %+v %v", r, err)
	}
	p = newNDExtra()
	p.stuck = true
	m = NewManager(p, t.TempDir(), Options{})
	if r, err := m.Apply(ndPublicDNS, true); err == nil || r.Name != "" {
		t.Errorf("servers not there when read back: must report failure, got %+v %v", r, err)
	}
	p = newNDExtra()
	p.elevErr = errors.New("the person said no")
	m = NewManager(p, t.TempDir(), Options{})
	if _, err := m.Apply(ndPublicDNS, true); err == nil {
		t.Error("no administrator permission: must not apply")
	}
	if p.setCalls != 0 || !reflect.DeepEqual(p.dns, ndOriginalDNS()) {
		t.Error("no administrator permission: nothing may change")
	}
	if err := m.Undo(ndPublicDNS); !errors.Is(err, ErrNothingToUndo) {
		t.Errorf("no administrator permission: nothing saved, want ErrNothingToUndo, got %v", err)
	}
}

// ND-243: the existing power saving fix also reads back: a yes from Windows with the value
// unchanged is a failure, for apply and for undo.
type ndStuckPower struct{ *simulated }

func (ndStuckPower) SetPowerSaving(string) error { return nil }

func TestND243PowerSavingReadBack(t *testing.T) {
	p := newSimulated()
	m := NewManager(ndStuckPower{p}, t.TempDir(), Options{})
	if _, err := m.Apply("power-saving", true); err == nil {
		t.Error("power saving still on after the change: apply must report failure")
	}
	p = newSimulated()
	dir := t.TempDir()
	if _, err := NewManager(p, dir, Options{}).Apply("power-saving", true); err != nil {
		t.Fatal(err)
	}
	if err := NewManager(ndStuckPower{p}, dir, Options{}).Undo("power-saving"); err == nil {
		t.Error("power saving still off after the undo: undo must report failure")
	}
}

// ND-236, ND-237, ND-243: renew-address and reset-adapter need a yes and administrator
// permission, report a failure as failed, and have no undo.
func TestND236And237NoUndoAndFailures(t *testing.T) {
	for _, name := range []string{ndRenewAddress, ndResetAdapter} {
		p := newNDExtra()
		m := NewManager(p, t.TempDir(), Options{})
		count := func() int {
			if name == ndRenewAddress {
				return p.renews
			}
			return p.resets
		}
		if _, err := m.Apply(name, false); !errors.Is(err, ErrNotApproved) || count() != 0 {
			t.Errorf("%s: no yes means nothing happens: %v", name, err)
		}
		if _, err := m.Apply(name, true); err != nil || count() != 1 || p.elevations < 1 {
			t.Errorf("%s: yes must run it once with administrator permission: err %v, runs %d, elevations %d", name, err, count(), p.elevations)
		}
		if err := m.Undo(name); err == nil {
			t.Errorf("%s: there is no undo, the command must say so", name)
		}
		if count() != 1 {
			t.Errorf("%s: undo must not run anything", name)
		}
		if ndListed(m, name) {
			t.Errorf("%s: must never be listed as an undo", name)
		}
		pl, err := m.Preview(name)
		if err != nil {
			t.Errorf("%s: preview: %v", name, err)
		} else if !strings.Contains(strings.ToLower(Title(name)+" "+pl.Before+" "+pl.After), "undo") {
			t.Errorf("%s: the offer must say it has no undo: %q %+v", name, Title(name), pl)
		}

		p = newNDExtra()
		p.renewErr = errors.New("no answer from the router")
		p.resetErr = errors.New("the adapter did not come back")
		m = NewManager(p, t.TempDir(), Options{})
		if r, err := m.Apply(name, true); err == nil || r.Name != "" {
			t.Errorf("%s: a failure must be reported as failed: %+v %v", name, r, err)
		}
		p = newNDExtra()
		p.elevErr = errors.New("the person said no")
		m = NewManager(p, t.TempDir(), Options{})
		if _, err := m.Apply(name, true); err == nil || count() != 0 {
			t.Errorf("%s: no administrator permission: must not run", name)
		}
	}
}

// ND-238: a system without the round-two abilities (Mac and Linux) refuses all three and changes nothing.
func TestND238RefusedWithoutExtraSystem(t *testing.T) {
	for _, name := range []string{ndPublicDNS, ndRenewAddress, ndResetAdapter} {
		p := newSimulated()
		m := NewManager(p, t.TempDir(), Options{})
		_, err := m.Apply(name, true)
		var ref *RefusedError
		if !errors.As(err, &ref) {
			t.Errorf("%s: want a plain refusal on a system without the ability, got %v", name, err)
		}
		if p.elevations != 0 || p.flushes != 0 || p.proxyWrites != 0 || p.powerWrites != 0 {
			t.Errorf("%s: a refusal changes nothing and asks for nothing: %+v", name, p)
		}
	}
}

// ND-238: the list names every fix with needs_admin true for the new three, and says which can be undone.
func TestND238ListAdminAndUndoable(t *testing.T) {
	m := NewManager(newNDExtra(), t.TempDir(), Options{})
	got := map[string]Info{}
	for _, in := range m.List() {
		got[in.Name] = in
	}
	wantUndo := map[string]bool{
		"proxy": true, "power-saving": true, ndPublicDNS: true,
		ndRenewAddress: false, ndResetAdapter: false,
	}
	for name, undo := range wantUndo {
		in, ok := got[name]
		if !ok {
			t.Errorf("fix list must name %s", name)
			continue
		}
		if in.Undoable != undo {
			t.Errorf("%s: undoable %v, want %v", name, in.Undoable, undo)
		}
	}
	for _, name := range []string{ndPublicDNS, ndRenewAddress, ndResetAdapter, "power-saving"} {
		if !NeedsAdmin(name) || !got[name].NeedsAdmin {
			t.Errorf("%s: needs_admin must be true", name)
		}
	}
	for _, name := range []string{"name-cache", "proxy"} {
		if NeedsAdmin(name) {
			t.Errorf("%s: needs no administrator permission", name)
		}
	}
	if _, ok := got["name-cache"]; !ok {
		t.Error("the old fixes stay in the list")
	}
}

// The fix names, the same strings core uses (core.FixPublicDNS and the others).
const (
	ndPublicDNS    = "public-dns"
	ndRenewAddress = "renew-address"
	ndResetAdapter = "reset-adapter"
)
