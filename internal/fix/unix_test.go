package fix

import (
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// pc is a simulated Mac or Linux computer. Every command the fixes run lands here; none reaches
// the real system (ND-307). It keeps the settings the commands change, so a fix can be applied,
// read back and undone.
type pc struct {
	goos    string
	calls   []string
	missing map[string]bool // programs that are not there
	// Mac
	route        bool // a default route exists
	proxyOn      bool
	proxyServer  string
	proxyPort    string
	needAdmin    bool // the set commands fail without administrator rights
	adminPrompts int
	adminRefused bool
	svcEnabled   bool
	// Linux
	mode, host, port, pac string
	ignoreAuto            bool
	nmState               string
	// both
	dns       []string // nil: set by the router
	address   string
	renewGood bool   // the router hands out a usable address
	comesBack bool   // the adapter comes back on after a reset
	setIgnore bool   // a set command silently changes nothing
	zeroError bool   // a refused set prints an error but exits 0, as networksetup can
	offFails  bool   // turning the adapter off is refused
	forceMode string // the desktop keeps this mode whatever is set
	vpn       bool   // default routes through tunnel adapters come first
	noUUID    bool   // an older nmcli that gives no unique id
}

func newMac() *pc {
	return &pc{goos: "darwin", route: true, proxyOn: true, proxyServer: "proxy.local", proxyPort: "8080",
		address: "169.254.10.2", renewGood: true, comesBack: true, svcEnabled: true, missing: map[string]bool{}}
}

func newLinux() *pc {
	return &pc{goos: "linux", route: true, mode: "manual", host: "proxy.local", port: "8080",
		address: "169.254.10.2", renewGood: true, comesBack: true, nmState: "100 (connected)", missing: map[string]bool{}}
}

var fail = errors.New("exit status 1")

func (p *pc) look(name string) (string, error) {
	if p.missing[name] {
		return "", errors.New("not found")
	}
	return "/usr/bin/" + name, nil
}

func (p *pc) sys() UnixSystem {
	return UnixSystem{GOOS: p.goos, Run: p.run, Look: p.look, Index: func(string) int { return 4 },
		Sleep: func(time.Duration) {}, Conds: func() (Conditions, error) { return lineDead, nil }}
}

func (p *pc) run(name string, args ...string) (string, error) {
	return p.runAs(false, name, args...)
}

func (p *pc) runAs(admin bool, name string, args ...string) (string, error) {
	cmd := name + " " + strings.Join(args, " ")
	p.calls = append(p.calls, cmd)
	if p.missing[name] {
		return "", errors.New("executable file not found")
	}
	set := func(apply func()) (string, error) {
		if p.needAdmin && !admin {
			if p.zeroError {
				return "** Error: Command requires admin privileges.", nil
			}
			return "** Error: Command requires admin privileges.", fail
		}
		if !p.setIgnore {
			apply()
		}
		return "", nil
	}
	switch {
	// Mac
	case name == "osascript" && strings.Contains(strings.Join(args, " "), "with administrator privileges"):
		p.adminPrompts++
		if p.adminRefused {
			return "execution error: User canceled. (-128)", fail
		}
		i := 0
		for i < len(args) && args[i] == "-e" {
			i += 2
		}
		return p.runAs(true, args[i], args[i+1:]...)
	case cmd == "route -n get default":
		if !p.route {
			return "route: writing to routing socket: not in table", fail
		}
		return "   route to: default\ndestination: default\n    gateway: 192.168.1.1\n  interface: en0\n", nil
	case cmd == "networksetup -listnetworkserviceorder":
		return "An asterisk (*) denotes that a network service is disabled.\n(1) Wi-Fi\n(Hardware Port: Wi-Fi, Device: en0)\n\n(*) Thunderbolt Bridge\n(Hardware Port: Thunderbolt Bridge, Device: bridge0)\n", nil
	case cmd == "ifconfig en0":
		if p.svcEnabled {
			return "en0: flags=8863<UP>\n\tstatus: active", nil
		}
		return "en0: flags=8822\n\tstatus: inactive", nil
	case cmd == "networksetup -getwebproxy Wi-Fi":
		on := "No"
		if p.proxyOn {
			on = "Yes"
		}
		return "Enabled: " + on + "\nServer: " + p.proxyServer + "\nPort: " + p.proxyPort + "\nAuthenticated Proxy Enabled: 0", nil
	case strings.HasPrefix(cmd, "networksetup -setwebproxystate Wi-Fi "):
		return set(func() { p.proxyOn = args[2] == "on" })
	case cmd == "networksetup -getdnsservers Wi-Fi":
		if p.dns == nil {
			return "There aren't any DNS Servers set on Wi-Fi.", nil
		}
		return strings.Join(p.dns, "\n"), nil
	case strings.HasPrefix(cmd, "networksetup -setdnsservers Wi-Fi "):
		return set(func() {
			if args[2] == "Empty" {
				p.dns = nil
			} else {
				p.dns = append([]string(nil), args[2:]...)
			}
		})
	case cmd == "ipconfig set en0 DHCP":
		return set(func() {
			if p.renewGood {
				p.address = "192.168.1.20"
			}
		})
	case cmd == "ipconfig getifaddr en0" || cmd == "ip -4 -o addr show dev wlp2s0":
		if p.goos == "linux" {
			return "3: wlp2s0    inet " + p.address + "/24 brd 192.168.1.255 scope global dynamic wlp2s0", nil
		}
		return p.address, nil
	case name == "sh" && len(args) == 4 && args[3] == "Wi-Fi":
		return set(func() { p.svcEnabled = p.comesBack })
	case cmd == "dscacheutil -flushcache", cmd == "resolvectl flush-caches":
		return "", nil
	// Linux
	case cmd == "gsettings get org.gnome.system.proxy mode":
		return "'" + p.mode + "'", nil
	case cmd == "gsettings get org.gnome.system.proxy.http host":
		return "'" + p.host + "'", nil
	case cmd == "gsettings get org.gnome.system.proxy.http port":
		return p.port, nil
	case cmd == "gsettings get org.gnome.system.proxy autoconfig-url":
		return "'" + p.pac + "'", nil
	case strings.HasPrefix(cmd, "gsettings set org.gnome.system.proxy mode "):
		return set(func() {
			p.mode = args[3]
			if p.forceMode != "" {
				p.mode = p.forceMode
			}
		})
	case cmd == "ip route show default":
		if !p.route {
			return "", nil
		}
		routes := "default via 10.0.0.1 dev eth9 proto dhcp metric 900\ndefault via 192.168.1.1 dev wlp2s0 proto dhcp metric 600"
		if p.vpn {
			routes = "default dev wg0 scope link metric 50\ndefault via 10.8.0.1 dev myvpn proto static metric 40\n" +
				"default dev tun0 scope link metric 30\n" + routes
		}
		return routes, nil
	case cmd == "nmcli -t -f DEVICE,TYPE,STATE device":
		return "lo:loopback:unmanaged\nwlp2s0:wifi:connecting (getting IP configuration)\neth9:ethernet:unavailable\n" +
			"myvpn:wireguard:connected (externally)\nwg0:wireguard:connected\ntun0:tun:connected (externally)", nil
	case cmd == "nmcli -t -g GENERAL.CONNECTION,GENERAL.CON-UUID device show wlp2s0":
		if p.noUUID {
			return `Home\: WiFi`, nil
		}
		return "Home\\: WiFi\n" + testUUID, nil
	case cmd == "nmcli -t -g GENERAL.DEVICES connection show "+p.ref():
		return "wlp2s0", nil
	case cmd == "nmcli -t -g ipv4.dns,ipv4.ignore-auto-dns connection show "+p.ref():
		ig := "no"
		if p.ignoreAuto {
			ig = "yes"
		}
		return strings.TrimSpace(strings.Join(p.dns, ",") + "\n" + ig), nil
	case strings.HasPrefix(cmd, "nmcli connection modify "+p.ref()+" ipv4.dns "):
		return set(func() {
			p.dns = nil
			if f := strings.Fields(args[5]); len(f) > 0 {
				p.dns = f
			}
			p.ignoreAuto = args[7] == "yes"
		})
	case cmd == "nmcli device reapply wlp2s0":
		return set(func() {
			if p.renewGood {
				p.address = "192.168.1.20"
			}
		})
	case cmd == "nmcli device disconnect wlp2s0":
		if p.offFails {
			return "Error: not authorized", fail
		}
		return set(func() { p.nmState = "30 (disconnected)" })
	case cmd == "nmcli device connect wlp2s0":
		return set(func() {
			if p.comesBack {
				p.nmState = "100 (connected)"
			}
		})
	case cmd == "nmcli -t -g GENERAL.STATE device show wlp2s0":
		return p.nmState, nil
	}
	return "unexpected command: " + cmd, fail
}

const testUUID = "5d1c0e2a-7b1f-4c55-9a1e-0f3b2d6c8e11"

// ref is how nmcli is told which connection: by its unique id when it has one, else by its name.
func (p *pc) ref() string {
	if p.noUUID {
		return "id Home: WiFi"
	}
	return "uuid " + testUUID
}

func (p *pc) ran(sub string) bool {
	for _, c := range p.calls {
		if strings.Contains(c, sub) {
			return true
		}
	}
	return false
}

func logLines(t *testing.T, dir string) int {
	b, _ := os.ReadFile(filepath.Join(dir, "fix-log.txt"))
	return strings.Count(string(b), "\n")
}

// ND-303: the proxy fix on a simulated Mac (networksetup) and a simulated Linux (gsettings): no change
// without a yes, before and after shown, a second apply refused, undo puts back the saved value, a
// hand change since makes undo refuse, and every change is logged.
func TestProxyFixMacAndLinux(t *testing.T) {
	for _, p := range []*pc{newMac(), newLinux()} {
		dir := t.TempDir()
		m := NewManager(p.sys(), dir, Options{})
		pl, err := m.Preview(proxy)
		if err != nil || pl.Before != "on, using proxy.local:8080" || pl.After != "off" {
			t.Fatalf("%s preview: %+v %v", p.goos, pl, err)
		}
		if _, err := m.Apply(proxy, false); err != ErrNotApproved || p.ran("-set") || p.ran(" set ") {
			t.Fatalf("%s: nothing may change without a yes: %v %v", p.goos, err, p.calls)
		}
		r, err := m.Apply(proxy, true)
		if err != nil || r.Before != "on, using proxy.local:8080" || r.After != "off" {
			t.Fatalf("%s apply: %+v %v", p.goos, r, err)
		}
		if p.goos == "darwin" && (p.proxyOn || !p.ran("networksetup -setwebproxystate Wi-Fi off")) {
			t.Errorf("mac: the web proxy must be off through networksetup: %v", p.calls)
		}
		if p.goos == "linux" && (p.mode != "none" || !p.ran("gsettings set org.gnome.system.proxy mode none")) {
			t.Errorf("linux: the proxy mode must be none through gsettings: %v", p.calls)
		}
		var re *RefusedError
		if _, err := m.Apply(proxy, true); !errors.As(err, &re) {
			t.Errorf("%s: a second apply must be refused: %v", p.goos, err)
		}
		if err := m.Undo(proxy); err != nil {
			t.Fatalf("%s undo: %v", p.goos, err)
		}
		if (p.goos == "darwin" && !p.proxyOn) || (p.goos == "linux" && p.mode != "manual") {
			t.Errorf("%s: undo must put the proxy back on", p.goos)
		}
		if n := logLines(t, dir); n != 2 {
			t.Errorf("%s: apply and undo must each be logged, found %d lines", p.goos, n)
		}
		// A hand change after the fix: undo refuses and changes nothing.
		if _, err := m.Apply(proxy, true); err != nil {
			t.Fatal(err)
		}
		if p.goos == "darwin" {
			p.proxyServer = "other.local"
		} else {
			p.host = "other.local"
		}
		before := len(p.calls)
		if err := m.Undo(proxy); !errors.As(err, &re) || !strings.Contains(err.Error(), "other.local") {
			t.Errorf("%s: a stale undo must be refused naming the server now: %v", p.goos, err)
		}
		for _, c := range p.calls[before:] {
			if strings.Contains(c, " set") || strings.Contains(c, "-set") {
				t.Errorf("%s: a refused undo changed something: %s", p.goos, c)
			}
		}
	}
	// Linux automatic setup is put back as automatic setup.
	p := newLinux()
	p.mode, p.host, p.pac = "auto", "", "http://wpad.local/proxy.pac"
	m := NewManager(p.sys(), t.TempDir(), Options{})
	if r, err := m.Apply(proxy, true); err != nil || !strings.Contains(r.Before, "http://wpad.local/proxy.pac") || p.mode != "none" {
		t.Fatalf("auto apply: %+v %v mode=%s", r, err, p.mode)
	}
	if err := m.Undo(proxy); err != nil || p.mode != "auto" {
		t.Errorf("auto undo: %v mode=%s", err, p.mode)
	}
	// A set that does not take is a failure, not a success (ND-243).
	p = newLinux()
	p.setIgnore = true
	if _, err := NewManager(p.sys(), t.TempDir(), Options{}).Apply(proxy, true); err == nil {
		t.Error("a proxy that is still on when read back must be reported as failed")
	}
}

// ND-304: public-dns on a simulated Mac and Linux, as ND-235 and ND-244: only the adapter in use is
// saved and set, read back, names cleared, exact undo ("Empty" on a Mac means automatic), a stale
// undo refused, and a second apply refused.
func TestPublicDNSMacAndLinux(t *testing.T) {
	for _, p := range []*pc{newMac(), newLinux()} {
		for _, start := range [][]string{nil, {"9.9.9.9"}} {
			p.dns = start
			p.calls = nil
			dir := t.TempDir()
			m := NewManager(p.sys(), dir, Options{})
			pl, err := m.Preview(publicDNS)
			if err != nil || !strings.Contains(pl.After, "third party") {
				t.Fatalf("%s preview: %+v %v", p.goos, pl, err)
			}
			if start == nil && !strings.Contains(pl.Before, "set by the router") {
				t.Errorf("%s: automatic must read as set by the router: %q", p.goos, pl.Before)
			}
			if _, err := m.Apply(publicDNS, true); err != nil {
				t.Fatalf("%s %v apply: %v", p.goos, start, err)
			}
			if strings.Join(p.dns, " ") != "1.1.1.1 1.0.0.1" {
				t.Errorf("%s: servers after %v", p.goos, p.dns)
			}
			if p.goos == "darwin" && !p.ran("networksetup -setdnsservers Wi-Fi 1.1.1.1 1.0.0.1") {
				t.Errorf("mac: %v", p.calls)
			}
			if p.goos == "linux" && (!p.ran("nmcli connection modify uuid "+testUUID+" ipv4.dns 1.1.1.1 1.0.0.1 ipv4.ignore-auto-dns yes") || !p.ran("nmcli device reapply wlp2s0")) {
				t.Errorf("linux: %v", p.calls)
			}
			if !p.ran("flush") {
				t.Errorf("%s: the saved names must be cleared", p.goos)
			}
			var re *RefusedError
			if _, err := m.Apply(publicDNS, true); !errors.As(err, &re) {
				t.Errorf("%s: a second apply must be refused: %v", p.goos, err)
			}
			if err := m.Undo(publicDNS); err != nil {
				t.Fatalf("%s undo: %v", p.goos, err)
			}
			if strings.Join(p.dns, " ") != strings.Join(start, " ") {
				t.Errorf("%s: undo must put back exactly %v, got %v", p.goos, start, p.dns)
			}
			if p.goos == "darwin" && start == nil && !p.ran("networksetup -setdnsservers Wi-Fi Empty") {
				t.Errorf("mac: automatic is put back with Empty: %v", p.calls)
			}
			if p.goos == "linux" && start == nil && (p.ignoreAuto || !p.ran(`ipv4.dns  ipv4.ignore-auto-dns no`)) {
				t.Errorf("linux: automatic is put back with no servers and the router's servers used: %v", p.calls)
			}
			// Stale undo: changed by hand since the fix.
			if _, err := m.Apply(publicDNS, true); err != nil {
				t.Fatal(err)
			}
			p.dns = []string{"8.8.8.8"}
			if err := m.Undo(publicDNS); !errors.As(err, &re) {
				t.Errorf("%s: a stale undo must be refused: %v", p.goos, err)
			}
		}
	}
	// No adapter in use: refused, nothing changed.
	p := newMac()
	p.route = false
	if _, err := NewManager(p.sys(), t.TempDir(), Options{}).Apply(publicDNS, true); err == nil || p.ran("-setdnsservers") {
		t.Errorf("no adapter in use must be refused: %v", err)
	}
}

// ND-305: renew-address on a simulated Mac (ipconfig set DHCP) and Linux (nmcli device reapply):
// refused when a usable address is there (ND-244), the address read back, a failure reported.
func TestRenewAddressMacAndLinux(t *testing.T) {
	for _, p := range []*pc{newMac(), newLinux()} {
		p.route = false // no address, so no default route: the adapter that is up is used
		sys := p.sys()
		sys.Conds = func() (Conditions, error) { return Conditions{AdapterUp: true}, nil }
		m := NewManager(sys, t.TempDir(), Options{})
		if _, err := m.Apply(renewAddress, true); err != nil {
			t.Fatalf("%s: %v %v", p.goos, err, p.calls)
		}
		want := map[string]string{"darwin": "ipconfig set en0 DHCP", "linux": "nmcli device reapply wlp2s0"}[p.goos]
		if !p.ran(want) || p.address != "192.168.1.20" {
			t.Errorf("%s: %v", p.goos, p.calls)
		}
		if err := m.Undo(renewAddress); err == nil {
			t.Errorf("%s: renew-address has no undo", p.goos)
		}
		// The router gives nothing usable: failed, not done.
		p.address, p.renewGood = "169.254.1.1", false
		if _, err := m.Apply(renewAddress, true); err == nil {
			t.Errorf("%s: no usable address after the renew must be a failure", p.goos)
		}
		// A usable address now: refused, nothing run.
		sys.Conds = func() (Conditions, error) { return Conditions{AdapterUp: true, HasAddress: true}, nil }
		p.calls = nil
		var re *RefusedError
		if _, err := NewManager(sys, t.TempDir(), Options{}).Apply(renewAddress, true); !errors.As(err, &re) || p.ran(want) {
			t.Errorf("%s: a usable address now must be refused: %v", p.goos, err)
		}
	}
}

// ND-306: reset-adapter on a simulated Mac (service off then on, in one step) and Linux (nmcli
// disconnect then connect): refused while the line works, checked to come back, a failure reported.
func TestResetAdapterMacAndLinux(t *testing.T) {
	for _, p := range []*pc{newMac(), newLinux()} {
		m := NewManager(p.sys(), t.TempDir(), Options{})
		pl, err := m.Preview(resetAdapter)
		if err != nil || !strings.Contains(pl.After, "only runs when the line is down") {
			t.Fatalf("%s preview: %+v %v", p.goos, pl, err)
		}
		if _, err := m.Apply(resetAdapter, true); err != nil {
			t.Fatalf("%s: %v %v", p.goos, err, p.calls)
		}
		switch p.goos {
		case "darwin":
			var sh string
			for _, c := range p.calls {
				if strings.HasPrefix(c, "sh -c") {
					sh = c
				}
			}
			if !strings.Contains(sh, `networksetup -setnetworkserviceenabled "$1" off`) || !strings.Contains(sh, `networksetup -setnetworkserviceenabled "$1" on`) ||
				!strings.HasSuffix(sh, " sh Wi-Fi") {
				t.Errorf("mac: off then on in one step, the service name as its own argument: %q", sh)
			}
		case "linux":
			if !p.ran("nmcli device disconnect wlp2s0") || !p.ran("nmcli device connect wlp2s0") {
				t.Errorf("linux: %v", p.calls)
			}
		}
		p.comesBack = false
		if _, err := m.Apply(resetAdapter, true); err == nil {
			t.Errorf("%s: an adapter that does not come back must be a failure", p.goos)
		}
		// The line works now: refused, nothing run.
		sys := p.sys()
		sys.Conds = func() (Conditions, error) {
			return Conditions{AdapterUp: true, HasAddress: true, RouterAnswers: true}, nil
		}
		p.calls = nil
		var re *RefusedError
		if _, err := NewManager(sys, t.TempDir(), Options{}).Apply(resetAdapter, true); !errors.As(err, &re) || len(p.calls) > 0 && p.ran("connect") {
			t.Errorf("%s: a working line must be refused: %v", p.goos, err)
		}
	}
	// Mac administrator rights are asked only at the change, and a no is a plain failure.
	p := newMac()
	p.needAdmin = true
	m := NewManager(p.sys(), t.TempDir(), Options{})
	if _, err := m.Preview(resetAdapter); err != nil || p.adminPrompts != 0 {
		t.Errorf("a preview must not ask for administrator rights: %v %d", err, p.adminPrompts)
	}
	if _, err := m.Apply(resetAdapter, true); err != nil || p.adminPrompts != 1 {
		t.Errorf("the change asks once: %v %d", err, p.adminPrompts)
	}
	p.adminRefused, p.svcEnabled = true, false
	if _, err := m.Apply(resetAdapter, true); err == nil || !strings.Contains(err.Error(), "administrator") {
		t.Errorf("a no at the prompt must say administrator rights were not given: %v", err)
	}
	// networksetup printing an error with exit code 0 still counts as refused, so the person is asked.
	p = newMac()
	p.needAdmin, p.zeroError = true, true
	if _, err := NewManager(p.sys(), t.TempDir(), Options{}).Apply(resetAdapter, true); err != nil || p.adminPrompts != 1 {
		t.Errorf("an error printed with exit 0 must lead to the permission box: %v %d", err, p.adminPrompts)
	}
	// The Mac step fails when either half fails.
	if !strings.Contains(macResetScript, "[ $a -eq 0 ] && [ $b -eq 0 ]") {
		t.Errorf("the reset step must fail when turning off fails: %s", macResetScript)
	}
	// Linux: a refused "off" is a failure, and the adapter is still turned back on.
	l := newLinux()
	l.offFails = true
	if _, err := NewManager(l.sys(), t.TempDir(), Options{}).Apply(resetAdapter, true); err == nil || !l.ran("nmcli device connect wlp2s0") {
		t.Errorf("a refused off must be a failure after turning it back on: %v %v", err, l.calls)
	}
}

// ND-307: on a simulated Mac and Linux with each program missing in turn, the fix that needs it is
// refused in plain words, changes nothing and is not offered; power saving is refused everywhere.
func TestFixesRefusedWhenTheToolIsMissing(t *testing.T) {
	need := map[string]map[string][]string{
		"darwin": {nameCache: {"dscacheutil"}, proxy: {"networksetup", "route"}, publicDNS: {"networksetup", "route"},
			renewAddress: {"ipconfig", "networksetup", "route"}, resetAdapter: {"networksetup", "ifconfig", "route"}},
		"linux": {nameCache: {"resolvectl"}, proxy: {"gsettings"}, publicDNS: {"nmcli", "ip"},
			renewAddress: {"nmcli", "ip"}, resetAdapter: {"nmcli", "ip"}},
	}
	for goos, fixes := range need {
		for name, tools := range fixes {
			for _, tool := range tools {
				p := newMac()
				if goos == "linux" {
					p = newLinux()
				}
				p.missing[tool] = true
				m := NewManager(p.sys(), t.TempDir(), Options{})
				var re *RefusedError
				if _, err := m.Preview(name); !errors.As(err, &re) {
					t.Errorf("%s %s without %s: preview must be refused, got %v", goos, name, tool, err)
				}
				_, err := m.Apply(name, true)
				if !errors.As(err, &re) || !strings.Contains(err.Error(), tool) {
					t.Errorf("%s %s without %s: apply must be refused naming it: %v", goos, name, tool, err)
				}
				if len(p.calls) != 0 {
					t.Errorf("%s %s without %s: nothing may run: %v", goos, name, tool, p.calls)
				}
				if p.adminPrompts != 0 {
					t.Errorf("%s: no permission may be asked", goos)
				}
			}
		}
		p := newMac()
		if goos == "linux" {
			p = newLinux()
		}
		var re *RefusedError
		m := NewManager(p.sys(), t.TempDir(), Options{})
		if _, err := m.Apply(power, true); !errors.As(err, &re) {
			t.Errorf("%s: power saving must always be refused: %v", goos, err)
		}
		if !p.sys().PowerSavingDropsLink() {
			t.Errorf("%s: power saving is never offered", goos)
		}
	}
	for _, s := range []string{missingToolReason("networksetup"), powerNeverReason, otherSystemReason, noExtraReason} {
		if g := core.GradeLevel(s); g > 8.0 {
			t.Errorf("grade %.1f too high: %q", g, s)
		}
	}
}

// ND-307: no fix test reaches a real command. The test files of this package may not start a
// program, ask for the real computer, or use a real command runner.
func TestFixTestsRunOnlySimulatedCommands(t *testing.T) {
	files, _ := filepath.Glob("*_test.go")
	bad := regexp.MustCompile(`exec\.Command|exec\.LookPath|\bReal\(\)|ExecRunner|execRun\b|os/exec"`)
	for _, f := range files {
		b, err := os.ReadFile(f)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(b), "\n") {
			if strings.Contains(line, "regexp.MustCompile(`exec") {
				continue // this check's own pattern
			}
			if bad.MatchString(line) {
				t.Errorf("%s:%d reaches a real command: %s", f, i+1, strings.TrimSpace(line))
			}
		}
	}
}

// Honest needs_admin on Mac and Linux: a fix whose tool may ask for a password says so, in the fix
// list and in its undo; one that never asks says it does not.
func TestNeedsAdminIsHonestOnMacAndLinux(t *testing.T) {
	want := map[string]map[string]bool{
		// networksetup and ipconfig may bring up the Mac's permission box; dscacheutil does not.
		"darwin": {nameCache: false, proxy: true, publicDNS: true, renewAddress: true, resetAdapter: true, power: false},
		// gsettings is the person's own setting; resolvectl and nmcli may ask through the system.
		"linux": {nameCache: true, proxy: false, publicDNS: true, renewAddress: true, resetAdapter: true, power: false},
	}
	for goos, fixes := range want {
		p := newMac()
		if goos == "linux" {
			p = newLinux()
		}
		m := NewManager(p.sys(), t.TempDir(), Options{})
		for name, w := range fixes {
			if got := m.NeedsAdmin(name); got != w {
				t.Errorf("%s %s: needs admin %v, want %v", goos, name, got, w)
			}
		}
		for _, in := range m.List() {
			if in.NeedsAdmin != fixes[in.Name] {
				t.Errorf("%s list %s: needs admin %v", goos, in.Name, in.NeedsAdmin)
			}
		}
		// The proxy undo says the same as the proxy fix.
		if _, err := m.Apply(proxy, true); err != nil {
			t.Fatal(err)
		}
		for _, u := range m.Undos() {
			if u.Name == proxy && u.NeedsAdmin != fixes[proxy] {
				t.Errorf("%s: proxy undo needs admin %v", goos, u.NeedsAdmin)
			}
		}
	}
	// A system that does not say keeps the old answer (Windows).
	if !NewManager(&ndExtra{}, t.TempDir(), Options{}).NeedsAdmin(power) {
		t.Error("without the system's own answer, the old list is used")
	}
}

// ND-304: on Linux, undoing public-dns puts back exactly what was saved, servers and whether the
// router's servers were used too, and changes no other setting.
func TestLinuxPublicDNSUndoIsExact(t *testing.T) {
	for _, c := range []struct {
		dns    []string
		ignore bool
	}{{[]string{"9.9.9.9"}, false}, {[]string{"9.9.9.9", "8.8.4.4"}, true}, {nil, false}, {nil, true}} {
		p := newLinux()
		p.dns, p.ignoreAuto = c.dns, c.ignore
		m := NewManager(p.sys(), t.TempDir(), Options{})
		if _, err := m.Apply(publicDNS, true); err != nil {
			t.Fatalf("%+v apply: %v", c, err)
		}
		if !p.ignoreAuto {
			t.Errorf("%+v: Cloudflare alone must be used after the fix", c)
		}
		if err := m.Undo(publicDNS); err != nil {
			t.Fatalf("%+v undo: %v", c, err)
		}
		if strings.Join(p.dns, " ") != strings.Join(c.dns, " ") || p.ignoreAuto != c.ignore {
			t.Errorf("%+v: after undo dns=%v ignore=%v", c, p.dns, p.ignoreAuto)
		}
		for _, call := range p.calls {
			if strings.HasPrefix(call, "nmcli connection modify") {
				f := strings.Fields(strings.TrimPrefix(call, "nmcli connection modify "+p.ref()+" "))
				for _, w := range f {
					if strings.HasPrefix(w, "ipv") && w != "ipv4.dns" && w != "ipv4.ignore-auto-dns" {
						t.Errorf("%+v: another setting was changed: %s", c, call)
					}
				}
			}
		}
	}
}

// ND-303: on Linux the proxy mode itself is saved and put back exactly, also when the automatic
// setup has no address or the manual setting has no server; the read-back compares the mode.
func TestLinuxProxyUndoPutsBackTheMode(t *testing.T) {
	for _, c := range []struct{ name, mode, host, pac string }{
		{"auto with no address", "auto", "", ""},
		{"manual with an empty server", "manual", "", ""},
		{"manual", "manual", "proxy.local", ""},
		{"auto with an address", "auto", "", "http://wpad.local/proxy.pac"},
		{"manual with an automatic address left over", "manual", "", "http://wpad.local/proxy.pac"},
	} {
		p := newLinux()
		p.mode, p.host, p.pac = c.mode, c.host, c.pac
		m := NewManager(p.sys(), t.TempDir(), Options{})
		if _, err := m.Apply(proxy, true); err != nil || p.mode != "none" {
			t.Fatalf("%s apply: %v mode=%s", c.name, err, p.mode)
		}
		if err := m.Undo(proxy); err != nil || p.mode != c.mode {
			t.Errorf("%s undo: %v mode=%s, want %s", c.name, err, p.mode, c.mode)
		}
	}
	// A mode that comes back different from the one asked for is a failure, not a success.
	p := newLinux()
	p.mode, p.host = "auto", ""
	m := NewManager(p.sys(), t.TempDir(), Options{})
	if _, err := m.Apply(proxy, true); err != nil {
		t.Fatal(err)
	}
	p.forceMode = "manual"
	if err := m.Undo(proxy); err == nil {
		t.Error("a read-back with the wrong mode must be reported as failed")
	}
}

// The adapter in use on Linux is never a tunnel: a default route through a VPN (by name, or by
// the kind NetworkManager gives it) is skipped, and nmcli is told the connection by its unique id,
// or by its name when nmcli gives none.
func TestLinuxInUseSkipsTunnelsAndUsesTheUniqueID(t *testing.T) {
	for _, noUUID := range []bool{false, true} {
		p := newLinux()
		p.vpn, p.noUUID = true, noUUID
		m := NewManager(p.sys(), t.TempDir(), Options{})
		pl, err := m.Preview(publicDNS)
		if err != nil || !strings.HasPrefix(pl.Before, "Home: WiFi") {
			t.Fatalf("noUUID=%v: the Wi-Fi under the VPN is the adapter in use: %+v %v", noUUID, pl, err)
		}
		if _, err := m.Apply(publicDNS, true); err != nil {
			t.Fatal(err)
		}
		if !p.ran("nmcli connection modify "+p.ref()+" ipv4.dns 1.1.1.1 1.0.0.1") || !p.ran("nmcli device reapply wlp2s0") {
			t.Errorf("noUUID=%v: %v", noUUID, p.calls)
		}
		for _, c := range p.calls {
			if strings.Contains(c, "wg0") || strings.Contains(c, "myvpn") || strings.Contains(c, "tun0") {
				t.Errorf("a tunnel was touched: %s", c)
			}
		}
	}
	for _, name := range []string{"tun0", "tap1", "wg0", "ppp0", "utun3", "ipsec0"} {
		if !tunnelName(name) {
			t.Errorf("%s is a tunnel", name)
		}
	}
	if tunnelName("wlp2s0") || tunnelName("eth0") {
		t.Error("real adapters are not tunnels")
	}
}
