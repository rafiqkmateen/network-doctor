package collect

import (
	"strings"
	"testing"
)

// pick the default route Windows really uses. The rows below are
// ones read on a real machine with the cable out and Wi-Fi on.
func TestPickDefaultRoute(t *testing.T) {
	wifiUp := RouteRow{IfIndex: 19, NextHop: "192.168.1.1", RouteMetric: 0, InterfaceMetric: 30, Status: "Up"}
	cableOut := RouteRow{IfIndex: 18, NextHop: "192.168.1.1", RouteMetric: 0, InterfaceMetric: 25, Status: "Disconnected"}
	cableUp := cableOut
	cableUp.Status = "Up"
	cases := []struct {
		name    string
		rows    []RouteRow
		wantOK  bool
		wantIdx int
	}{
		{"cable out, Wi-Fi up: Wi-Fi wins though the cable has the lower metric", []RouteRow{cableOut, wifiUp}, true, 19},
		{"cable out listed last: still Wi-Fi", []RouteRow{wifiUp, cableOut}, true, 19},
		{"both up, same route metric: the cable's lower interface metric wins", []RouteRow{wifiUp, cableUp}, true, 18},
		{"neither up: no pick", []RouteRow{cableOut, {IfIndex: 19, NextHop: "192.168.1.1", InterfaceMetric: 30, Status: "Disconnected"}}, false, 0},
		{"no rows: no pick", nil, false, 0},
		{"no adapter behind the route: dropped", []RouteRow{{IfIndex: 7, NextHop: "10.0.0.1", Status: ""}}, false, 0},
		{"higher route metric loses to a lower sum", []RouteRow{
			{IfIndex: 5, NextHop: "10.0.0.1", RouteMetric: 50, InterfaceMetric: 5, Status: "Up"},
			{IfIndex: 9, NextHop: "10.0.0.2", RouteMetric: 0, InterfaceMetric: 35, Status: "Up"},
		}, true, 9},
		{"exact tie: lowest interface number", []RouteRow{
			{IfIndex: 22, NextHop: "10.0.0.1", RouteMetric: 0, InterfaceMetric: 25, Status: "Up"},
			{IfIndex: 11, NextHop: "10.0.0.2", RouteMetric: 0, InterfaceMetric: 25, Status: "Up"},
		}, true, 11},
	}
	for _, c := range cases {
		got, ok := PickDefaultRoute(c.rows)
		if ok != c.wantOK || (ok && got.IfIndex != c.wantIdx) {
			t.Errorf("%s: got %+v %v, want ifIndex %d %v", c.name, got, ok, c.wantIdx, c.wantOK)
		}
	}
}

// the chosen route goes into the adapter script, and only safe values go in.
func TestAdapterScriptFill(t *testing.T) {
	has := func(name, s string, want ...string) {
		t.Helper()
		for _, w := range want {
			if !strings.Contains(s, w) {
				t.Errorf("%s: missing %q", name, w)
			}
		}
		if strings.Contains(s, "%IFINDEX%") || strings.Contains(s, "%GATEWAY%") {
			t.Errorf("%s: a placeholder was left in", name)
		}
	}
	has("pick", adapterScript(RouteRow{IfIndex: 19, NextHop: "192.168.1.1", Status: "Up"}, true, true),
		"$ix = 19\n", "$gw = '192.168.1.1'")
	has("none up", adapterScript(RouteRow{}, false, true), "$ix = 0\n", "$gw = ''\n")
	has("routes not read", adapterScript(RouteRow{}, false, false), "$ix = -1\n", "$gw = ''\n")

	bad := adapterScript(RouteRow{IfIndex: 19, NextHop: "1.2.3.4'; Remove-Item C:\\x; '"}, true, true)
	has("bad gateway", bad, "$ix = 19\n", "$gw = ''\n")
	if strings.Contains(bad, "Remove-Item") {
		t.Error("bad gateway text got into the script")
	}
	has("IPv6 gateway", adapterScript(RouteRow{IfIndex: 4, NextHop: "fe80::1"}, true, true), "$ix = 4\n", "$gw = ''\n")
	for _, g := range []string{"0.0.0.0", "255.255.255.255", "224.0.0.1", "127.0.0.1"} {
		has("not a real router "+g, adapterScript(RouteRow{IfIndex: 4, NextHop: g}, true, true), "$ix = 4\n", "$gw = ''\n")
	}
}

// Second audit: when no route is on a live adapter, no stale route
// is guessed (the cable's lower metric would always win); index 0 lets the
// script fall back and the diagnosis name both links.
func TestChooseAdapterRoute(t *testing.T) {
	cableOut := RouteRow{IfIndex: 18, NextHop: "192.168.1.1", RouteMetric: 0, InterfaceMetric: 25, Status: "Disconnected"}
	wifiOff := RouteRow{IfIndex: 19, NextHop: "192.168.1.1", RouteMetric: 0, InterfaceMetric: 30, Status: "Disconnected"}
	wifiUp := wifiOff
	wifiUp.Status = "Up"
	cases := []struct {
		name    string
		rows    []RouteRow
		wantIdx string
	}{
		{"one stale cable row: no guess, 0", []RouteRow{cableOut}, "$ix = 0\n"},
		{"two stale rows: no guess, 0", []RouteRow{wifiOff, cableOut}, "$ix = 0\n"},
		{"stale tie: no guess, 0", []RouteRow{{IfIndex: 30, Status: "Disconnected"}, {IfIndex: 12, Status: "Disconnected"}}, "$ix = 0\n"},
		{"a live route still beats a stale one with a lower sum", []RouteRow{cableOut, wifiUp}, "$ix = 19\n"},
		{"empty list: 0", nil, "$ix = 0\n"},
	}
	for _, c := range cases {
		pick, ok := ChooseAdapterRoute(c.rows)
		if s := adapterScript(pick, ok, true); !strings.Contains(s, c.wantIdx) {
			t.Errorf("%s: want %q, got pick %+v %v", c.name, c.wantIdx, pick, ok)
		}
	}
}

// Audit: DNS is read from the same adapter the check reads.
func TestSettingsScriptFill(t *testing.T) {
	for _, c := range []struct {
		idx  int
		want string
	}{{19, "$ix = 19\n"}, {0, "$ix = -1\n"}, {-7, "$ix = -1\n"}} {
		s := settingsScript(c.idx)
		if !strings.Contains(s, c.want) || strings.Contains(s, "%IFINDEX%") {
			t.Errorf("settings %d: want %q in script", c.idx, c.want)
		}
	}
}
