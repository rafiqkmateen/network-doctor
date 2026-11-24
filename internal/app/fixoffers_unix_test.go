package app

import (
	"bytes"
	"context"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/fix"
)

// ND-303 with ND-224: when the check sees a proxy the fix cannot turn off (on Linux one set only in
// the environment, while the desktop's own setting is off), the proxy fix would be refused, so it is
// never offered.
func TestProxyFixNotOfferedWhenItsSettingIsOff(t *testing.T) {
	old := newFixSystem
	t.Cleanup(func() { newFixSystem = old })
	p := &cliPretend{proxy: fix.ProxyState{Enabled: false}}
	newFixSystem = func() fix.System { return p }
	if got := fixOffers([]string{core.FixProxy}, ""); len(got) != 0 {
		t.Errorf("a proxy that is already off must not be offered: %+v", got)
	}
	p.proxy = fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}
	if got := fixOffers([]string{core.FixProxy}, ""); len(got) != 1 {
		t.Errorf("a proxy that is on is offered: %+v", got)
	}
}

// adminPretend is a pretend computer that says itself which fixes may ask for administrator
// rights, as a Mac does for the proxy fix.
type adminPretend struct{ *cliPretend }

func (adminPretend) MayAskAdmin(name string) bool { return name == core.FixProxy }

// The proxy fix on a computer whose tool may ask for administrator rights says so everywhere: in
// the check's offer, in `fix list` (words and JSON) and in `fix apply`.
func TestNeedsAdminFollowsTheComputer(t *testing.T) {
	old := newFixSystem
	t.Cleanup(func() { newFixSystem = old })
	p := adminPretend{&cliPretend{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}}
	newFixSystem = func() fix.System { return p }
	if got := fixOffers([]string{core.FixProxy}, ""); len(got) != 1 || !got[0].NeedsAdmin {
		t.Errorf("offer: %+v", got)
	}
	run := func(args ...string) string {
		var out bytes.Buffer
		Run(context.Background(), append([]string{"fix"}, args...), healthy(), t.TempDir(), &out)
		return out.String()
	}
	if s := run("list"); !strings.Contains(s, "proxy - Turn off the web proxy (asks for administrator rights when you say yes)") {
		t.Errorf("list words:\n%s", s)
	}
	if s := run("list", "--json"); !strings.Contains(s, `"name":"proxy","title":"Turn off the web proxy","now":"on, using 10.0.0.9:8080","needs_admin":true`) {
		t.Errorf("list json:\n%s", s)
	}
	if s := run("apply", "proxy", "--json"); !strings.Contains(s, `"needs_admin":true`) {
		t.Errorf("apply json:\n%s", s)
	}
	if s := run("apply", "power-saving", "--json"); strings.Contains(s, `"needs_admin":true`) {
		t.Errorf("this computer says power saving never asks:\n%s", s)
	}
}
