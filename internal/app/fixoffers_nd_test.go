package app

// Checks for when the check offers the round-two fixes (ND-235 to ND-238) and how the fix command
// shows them, written before the build. The fix system is a pretend
// one; nothing on this computer changes.

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/fix"
)

// ndFixPC is a pretend Windows computer with the round-two abilities.
type ndFixPC struct {
	*cliPretend
	dns            []fix.DNSState
	sets, renews   int
	resets, elevns int
}

func (p *ndFixPC) DNS() ([]fix.DNSState, error) {
	out := make([]fix.DNSState, len(p.dns))
	for i, s := range p.dns {
		s.Servers = append([]string(nil), s.Servers...)
		out[i] = s
	}
	return out, nil
}
func (p *ndFixPC) SetDNS(in []fix.DNSState) error {
	p.sets++
	for _, s := range in {
		for i := range p.dns {
			if p.dns[i].IfIndex == s.IfIndex {
				s.Servers = append([]string(nil), s.Servers...)
				p.dns[i] = s
			}
		}
	}
	return nil
}
func (p *ndFixPC) Renew() error        { p.renews++; return nil }
func (p *ndFixPC) ResetAdapter() error { p.resets++; return nil }
func (p *ndFixPC) Elevate() error      { p.elevns++; return nil }

var _ fix.ExtraSystem = (*ndFixPC)(nil)

func newNDFixPC() *ndFixPC {
	return &ndFixPC{
		cliPretend: &cliPretend{proxy: fix.ProxyState{}},
		dns:        []fix.DNSState{{Adapter: "Wi-Fi", IfIndex: 12, Servers: []string{"192.168.1.1"}, Automatic: true}},
	}
}

func ndWithFixSystem(t *testing.T, sys fix.System) {
	t.Helper()
	old := newFixSystem
	newFixSystem = func() fix.System { return sys }
	t.Cleanup(func() { newFixSystem = old })
}

func ndOffer(r Report, name string) (FixOffer, bool) {
	for _, o := range r.FixOffers {
		if o.Name == name {
			return o, true
		}
	}
	return FixOffer{}, false
}

func ndChangedNothing(t *testing.T, where string, p *ndFixPC) {
	t.Helper()
	if p.sets != 0 || p.renews != 0 || p.resets != 0 || p.elevns != 0 || p.writes != 0 {
		t.Errorf("%s: the check must change nothing and ask for nothing: %+v", where, *p)
	}
}

// ND-235: public-dns is offered when names fail while the internet answers, with administrator
// permission and the third-party words; never when the internet is silent.
func TestND235PublicDNSOffered(t *testing.T) {
	p := newNDFixPC()
	ndWithFixSystem(t, p)
	s := ndWiFi()
	s.dnsFails = true
	r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	o, ok := ndOffer(r, "public-dns")
	if !ok {
		t.Fatalf("names fail, internet answers: public-dns must be offered: %v", r.Fixes)
	}
	if !o.NeedsAdmin || !strings.Contains(o.After, "1.1.1.1") || !strings.Contains(strings.ToLower(o.Title+" "+o.Now+" "+o.After), "third") {
		t.Errorf("offer: %+v", o)
	}
	ndChangedNothing(t, "public-dns offer", p)
	ndNoLocationAsk(t, "public-dns offer", r)
}

// ND-238: no round-two fix is offered on a system without the abilities (Mac and Linux).
func TestND238NotOfferedWithoutAbilities(t *testing.T) {
	ndWithFixSystem(t, &cliPretend{})
	s := ndWiFi()
	s.dnsFails = true
	r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	if _, ok := ndOffer(r, "public-dns"); ok {
		t.Error("a system without the ability must not be offered public-dns")
	}
	s = ndWiFi()
	s.adapter.NoAddress, s.adapter.IP, s.adapter.Gateway = true, "169.254.1.2", ""
	s.netP = collect.PingResult{LossPct: 100}
	s.probeErr = context.DeadlineExceeded
	r = Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	if _, ok := ndOffer(r, "renew-address"); ok {
		t.Error("a system without the ability must not be offered renew-address")
	}
}

// ND-236: renew-address is offered when the adapter is up with no usable address, and says it has no undo.
func TestND236RenewOffered(t *testing.T) {
	p := newNDFixPC()
	ndWithFixSystem(t, p)
	s := ndWiFi()
	s.adapter.NoAddress, s.adapter.IP, s.adapter.Gateway = true, "169.254.1.2", ""
	s.netP = collect.PingResult{LossPct: 100}
	s.probeErr = context.DeadlineExceeded
	r := Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	o, ok := ndOffer(r, "renew-address")
	if !ok {
		t.Fatalf("up with no address: renew-address must be offered: %v", r.Fixes)
	}
	if !o.NeedsAdmin || !strings.Contains(strings.ToLower(o.Title+" "+o.Now+" "+o.After), "undo") {
		t.Errorf("offer must need administrator permission and say it has no undo: %+v", o)
	}
	ndChangedNothing(t, "renew offer", p)
}

// ND-237: reset-adapter is offered only when neither the internet nor the router answers.
func TestND237ResetOfferedOnlyWhenDead(t *testing.T) {
	p := newNDFixPC()
	ndWithFixSystem(t, p)
	dead := func(routerLoss float64) Report {
		s := ndWiFi()
		s.routerP = collect.PingResult{LossPct: routerLoss}
		s.netP = collect.PingResult{LossPct: 100}
		s.probeErr = context.DeadlineExceeded
		return Check(context.Background(), s, Options{PlanMbps: 300, Quick: true})
	}
	r := dead(100)
	o, ok := ndOffer(r, "reset-adapter")
	if !ok {
		t.Fatalf("no internet and no router answer: reset-adapter must be offered: %v", r.Fixes)
	}
	if !o.NeedsAdmin || !strings.Contains(strings.ToLower(o.Title+" "+o.Now+" "+o.After), "undo") {
		t.Errorf("offer must need administrator permission and say it has no undo: %+v", o)
	}
	if _, ok := ndOffer(dead(0), "reset-adapter"); ok {
		t.Error("the router answers: reset-adapter must not be offered")
	}
	ndChangedNothing(t, "reset offer", p)
}

// ND-238: a working connection is never offered any round-two fix.
func TestND238NeverOfferedOnAWorkingConnection(t *testing.T) {
	p := newNDFixPC()
	ndWithFixSystem(t, p)
	r := Check(context.Background(), ndWiFi(), Options{PlanMbps: 300})
	for _, n := range []string{"public-dns", "renew-address", "reset-adapter"} {
		if _, ok := ndOffer(r, n); ok {
			t.Errorf("working connection: %s must not be offered", n)
		}
	}
	ndChangedNothing(t, "healthy check", p)
}

func ndFix(t *testing.T, sys fix.System, dir string, args ...string) (string, int) {
	t.Helper()
	ndWithFixSystem(t, sys)
	var out bytes.Buffer
	code := Run(context.Background(), append([]string{"fix"}, args...), healthy(), dir, &out)
	return out.String(), code
}

// ND-238: the three fixes appear in fix list, fix apply NAME --yes --json and fix undo NAME --json,
// with needs_admin true; renew-address and reset-adapter have no undo and the list says so.
func TestND238FixCommand(t *testing.T) {
	p := newNDFixPC()
	dir := t.TempDir()
	out, code := ndFix(t, p, dir, "list", "--json")
	var list struct {
		Fixes []struct {
			Name       string `json:"name"`
			NeedsAdmin bool   `json:"needs_admin"`
		} `json:"fixes"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &list); err != nil || code != 0 {
		t.Fatalf("list --json: %v code %d\n%s", err, code, out)
	}
	seen := map[string]bool{}
	for _, f := range list.Fixes {
		seen[f.Name] = f.NeedsAdmin
	}
	for _, n := range []string{"public-dns", "renew-address", "reset-adapter"} {
		admin, ok := seen[n]
		if !ok || !admin {
			t.Errorf("list --json must name %s with needs_admin true: %s", n, out)
		}
	}

	out, _ = ndFix(t, p, dir, "list")
	lines := strings.Split(out, "\n")
	for _, n := range []string{"renew-address", "reset-adapter"} {
		found := false
		for i, l := range lines {
			if strings.HasPrefix(l, n+" ") {
				found = true
				block := l
				if i+1 < len(lines) {
					block += " " + lines[i+1]
				}
				if !strings.Contains(strings.ToLower(block), "undo") {
					t.Errorf("fix list must say %s has no undo: %q", n, block)
				}
			}
		}
		if !found {
			t.Errorf("fix list must name %s:\n%s", n, out)
		}
	}

	var j jsonOut
	out, code = ndFix(t, p, dir, "apply", "public-dns", "--yes", "--json")
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &j); err != nil || code != 0 || !j.OK || !j.Applied || !j.NeedsAdmin {
		t.Errorf("apply public-dns --yes --json: %+v code %d\n%s", j, code, out)
	}
	j = jsonOut{}
	out, code = ndFix(t, p, dir, "undo", "public-dns", "--json")
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &j); err != nil || code != 0 || !j.OK || !j.Undone {
		t.Errorf("undo public-dns --json: %+v code %d\n%s", j, code, out)
	}
	if len(p.dns) != 1 || !p.dns[0].Automatic || strings.Join(p.dns[0].Servers, ",") != "192.168.1.1" {
		t.Errorf("undo must put back exactly what was there: %+v", p.dns)
	}
	for _, n := range []string{"renew-address", "reset-adapter"} {
		j = jsonOut{}
		out, code = ndFix(t, p, dir, "apply", n, "--yes", "--json")
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &j); err != nil || code != 0 || !j.OK || !j.Applied || !j.NeedsAdmin {
			t.Errorf("apply %s --yes --json: %+v code %d\n%s", n, j, code, out)
		}
		j = jsonOut{}
		out, _ = ndFix(t, p, dir, "undo", n, "--json")
		if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &j); err != nil || j.OK || j.Undone || j.Error == "" {
			t.Errorf("undo %s --json must say there is no undo: %+v\n%s", n, j, out)
		}
	}
	if p.renews != 1 || p.resets != 1 {
		t.Errorf("each ran once: renews %d, resets %d", p.renews, p.resets)
	}
}
