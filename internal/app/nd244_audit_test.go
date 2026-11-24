package app

// Check for ND-244 through the command path, written before the
// build: `fix apply NAME --yes --json` reads the situation at the press and refuses a fix whose
// trouble is gone.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/fix"
)

// condSimulated is a simulated Windows computer with the round-two abilities and the situation read.
type condSimulated struct {
	cliSimulated
	cond           fix.Conditions
	renews, resets int
}

func (p *condSimulated) DNS() ([]fix.DNSState, error) {
	return []fix.DNSState{{Adapter: "Wi-Fi", IfIndex: 12, Automatic: true, InUse: true}}, nil
}
func (p *condSimulated) SetDNS([]fix.DNSState) error         { return nil }
func (p *condSimulated) Renew() error                        { p.renews++; return nil }
func (p *condSimulated) ResetAdapter() error                 { p.resets++; return nil }
func (p *condSimulated) Conditions() (fix.Conditions, error) { return p.cond, nil }

var (
	_ fix.ExtraSystem     = (*condSimulated)(nil)
	_ fix.ConditionSystem = (*condSimulated)(nil)
)

func runCondFix(t *testing.T, p *condSimulated, args ...string) (fixReply, string) {
	t.Helper()
	old := newFixSystem
	newFixSystem = func() fix.System { return p }
	defer func() { newFixSystem = old }()
	var out strings.Builder
	var buf = &out
	_ = Run(t.Context(), append([]string{"fix"}, args...), healthy(), t.TempDir(), writerFunc(func(b []byte) (int, error) { return buf.Write(b) }))
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	var j fixReply
	_ = json.Unmarshal([]byte(lines[len(lines)-1]), &j)
	return j, out.String()
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(b []byte) (int, error) { return f(b) }

func TestND244CommandRefusesWhenTheTroubleIsGone(t *testing.T) {
	p := &condSimulated{cond: fix.Conditions{AdapterUp: true, HasAddress: true, RouterAnswers: true, InternetAnswers: true}}
	for _, name := range []string{"reset-adapter", "renew-address"} {
		j, raw := runCondFix(t, p, "apply", name, "--yes", "--json")
		if j.Applied || j.OK || strings.TrimSpace(j.Error) == "" {
			t.Errorf("%s on a working line: must be refused with a plain note:\n%s", name, raw)
		}
	}
	if p.renews != 0 || p.resets != 0 {
		t.Errorf("nothing may run on a working line: renews %d, resets %d", p.renews, p.resets)
	}
	p = &condSimulated{cond: fix.Conditions{AdapterUp: true}}
	for _, name := range []string{"reset-adapter", "renew-address"} {
		if j, raw := runCondFix(t, p, "apply", name, "--yes", "--json"); !j.Applied {
			t.Errorf("%s on a dead line with no address: must run:\n%s", name, raw)
		}
	}
	if p.renews != 1 || p.resets != 1 {
		t.Errorf("each must run once on a dead line: renews %d, resets %d", p.renews, p.resets)
	}
}
