package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/fix"
)

type cliPretend struct {
	proxy  fix.ProxyState
	writes int
}

func (p *cliPretend) FlushDNS() error                { return nil }
func (p *cliPretend) Proxy() (fix.ProxyState, error) { return p.proxy, nil }
func (p *cliPretend) SetProxy(s fix.ProxyState) error {
	p.writes++
	p.proxy = s
	return nil
}
func (p *cliPretend) PowerSaving() (string, error) { return "on", nil }
func (p *cliPretend) SetPowerSaving(string) error  { return nil }
func (p *cliPretend) PowerSavingDropsLink() bool   { return false }
func (p *cliPretend) Elevate() error               { return nil }

func runFix(t *testing.T, p *cliPretend, dir string, args ...string) (string, int) {
	t.Helper()
	old := newFixSystem
	newFixSystem = func() fix.System { return p }
	defer func() { newFixSystem = old }()
	var out bytes.Buffer
	code := Run(context.Background(), append([]string{"fix"}, args...), healthy(), dir, &out)
	return out.String(), code
}

// ND-90, ND-91: apply without --yes shows before and after and changes nothing; with --yes it changes that one fix.
func TestFixApplyShowsThenNeedsYes(t *testing.T) {
	p := &cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	dir := t.TempDir()
	out, code := runFix(t, p, dir, "apply", "proxy")
	if code != 0 || p.writes != 0 {
		t.Fatalf("no yes means no change: code %d, writes %d", code, p.writes)
	}
	if !strings.Contains(out, "10.0.0.9:8080") || !strings.Contains(out, "--yes") {
		t.Errorf("must show the setting now and how to say yes:\n%s", out)
	}
	out, code = runFix(t, p, dir, "apply", "proxy", "--yes")
	if code != 0 || p.proxy.Enabled {
		t.Fatalf("with yes the proxy goes off: code %d\n%s", code, out)
	}
	if !strings.Contains(out, "fix undo proxy") {
		t.Errorf("must say how to put it back:\n%s", out)
	}
	out, code = runFix(t, p, dir, "undo", "proxy")
	if code != 0 || !p.proxy.Enabled {
		t.Errorf("undo must restore: code %d\n%s", code, out)
	}
}

// ND-95: a refused fix exits with a plain reason and a non-zero code.
func TestFixRefusalExitsWithReason(t *testing.T) {
	out, code := runFix(t, &cliPretend{}, t.TempDir(), "apply", "driver", "--yes")
	if code == 0 || !strings.Contains(out, "never changes drivers") {
		t.Errorf("code %d\n%s", code, out)
	}
}

// ND-96: with NETDOCTOR_UNATTENDED set, fixes stay off until enable-here.
func TestFixServerSwitch(t *testing.T) {
	t.Setenv("NETDOCTOR_UNATTENDED", "1")
	p := &cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "x:1"}}
	dir := t.TempDir()
	if _, code := runFix(t, p, dir, "apply", "proxy", "--yes"); code == 0 || p.writes != 0 {
		t.Fatalf("server must refuse by default: code %d writes %d", code, p.writes)
	}
	runFix(t, p, dir, "enable-here")
	if _, code := runFix(t, p, dir, "apply", "proxy", "--yes"); code != 0 || p.writes != 1 {
		t.Errorf("after enable-here it must work: code %d writes %d", code, p.writes)
	}
}

// The plain list names each fix and its setting now, and changes nothing.
func TestFixList(t *testing.T) {
	p := &cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "x:1"}}
	out, code := runFix(t, p, t.TempDir(), "list")
	if code != 0 || p.writes != 0 {
		t.Fatalf("code %d writes %d", code, p.writes)
	}
	for _, want := range []string{"name-cache", "proxy", "power-saving"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s:\n%s", want, out)
		}
	}
}
