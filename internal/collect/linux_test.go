package collect

import (
	"context"
	"reflect"
	"regexp"
	"strings"
	"testing"
	"time"
)

// ND-01, ND-163: every command the Linux reader may run only reads.
func TestLinuxCommandsOnlyRead(t *testing.T) {
	badWord := map[string]bool{}
	for _, w := range strings.Fields(`set add del delete flush up down restart start stop enable disable reload modify write
		connect disconnect connection radio networking edit clone import export monitor replace change append remove kill reset
		trigger reboot poweroff flush-caches revert`) {
		badWord[w] = true
	}
	badFlag := regexp.MustCompile(`^--(rotate|vacuum|flush|sync|relinquish|setup-keys|update-catalog|verify|kill)`)
	// Each tool may use only these words, so a new word has to be looked at here.
	allowed := map[string][]string{
		"ip":         {"-j", "route", "addr", "show"},
		"ethtool":    {"-i", "{dev}"},
		"iw":         {"dev", "{dev}", "link", "get", "power_save", "scan", "dump"},
		"nmcli":      {"-t", "-f", "IN-USE,CHAN,FREQ,SIGNAL", "GENERAL.STATE", "device", "wifi", "list", "show", "--rescan", "no", "{dev}"},
		"resolvectl": {"status"},
		"journalctl": {"--no-pager", "-k", "-u", "NetworkManager", "-o", "short-iso", "{since}"},
		"gsettings":  {"get", "org.gnome.system.proxy", "mode"},
		"ss":         {"-H", "-tnp", "state", "established"},
		"ping":       {"-c", "{n}", "-W", "1", "{ip}"},
	}
	if len(linuxCommands) == 0 {
		t.Fatal("no commands listed")
	}
	for _, c := range linuxCommands {
		tool := c[0]
		ok, known := allowed[tool]
		if !known {
			t.Errorf("tool %q is not expected", tool)
			continue
		}
		for _, a := range c[1:] {
			if badWord[strings.ToLower(a)] || badFlag.MatchString(a) {
				t.Errorf("%v uses change word %q", c, a)
			}
			found := false
			for _, w := range ok {
				found = found || w == a
			}
			if !found {
				t.Errorf("%v: word %q is not allowed for %s", c, a, tool)
			}
		}
	}
	// A few tools need one specific word.
	for _, c := range linuxCommands {
		switch c[0] {
		case "gsettings":
			if c[1] != "get" {
				t.Errorf("gsettings must only get: %v", c)
			}
		case "resolvectl":
			if c[1] != "status" {
				t.Errorf("resolvectl must only show status: %v", c)
			}
		case "iw":
			for i, a := range c {
				if a == "scan" && (i+1 >= len(c) || c[i+1] != "dump") {
					t.Errorf("iw may only read the scan the system already has: %v", c)
				}
			}
		case "ip":
			if c[len(c)-1] != "show" {
				t.Errorf("ip must only show: %v", c)
			}
		}
	}
}

func TestLinuxRunRefusesUnlisted(t *testing.T) {
	ctx := context.Background()
	for _, c := range [][]string{
		{"ip", "link", "set", "eth0", "down"},
		{"ip", "-j", "route", "show", "extra"},
		{"ethtool", "-s", "eth0", "speed", "10"},
		{"ethtool", "--", "eth0"},
		{"ethtool", "-eth0"},
		{"nmcli", "radio", "wifi", "off"},
		{"iw", "dev", "wlan0", "scan"},
		{"journalctl", "--rotate"},
		{"journalctl", "--no-pager", "-k", "-o", "short-iso", "--since=$(reboot)"},
		{"ping", "-c", "1", "-W", "1", "example.com"},
		{"rm", "-rf", "/"},
	} {
		out, err := linuxRun(ctx, c[0], c[1:]...)
		if err == nil || out != "" {
			t.Errorf("%v should be refused, got %q %v", c, out, err)
		}
	}
	if !linuxAllowed([]string{"ethtool", "-i", "enp3s0"}) || !linuxAllowed([]string{"ping", "-c", "1", "-W", "1", "192.168.1.5"}) {
		t.Fatal("listed commands must pass")
	}
}

func TestLinuxFilesGate(t *testing.T) {
	for _, p := range []string{
		"/etc/shadow", "/etc/passwd", "/sys/class/net/../../etc/shadow", "/sys/class/net/-x/speed",
		"/sys/class/net/eth0/speed/x", "/proc/net/dev/../../self/environ", "sys/class/net/eth0/speed", "/sys/class/net/eth0/carrier_changes",
	} {
		if linuxFileAllowed(p) {
			t.Errorf("%s should be refused", p)
		}
		if _, err := linuxReadFile(p); err == nil {
			t.Errorf("%s read should fail", p)
		}
	}
	for _, p := range []string{"/proc/net/dev", "/proc/net/arp", "/sys/class/net/wlp2s0/speed", "/sys/class/net/enx001122334455/statistics/rx_errors", "/etc/resolv.conf"} {
		if !linuxFileAllowed(p) {
			t.Errorf("%s should be allowed", p)
		}
	}
}

const ipRouteSample = `[{"dst":"default","gateway":"10.8.0.1","dev":"tun0","metric":50,"flags":[]},
{"dst":"default","gateway":"192.168.1.1","dev":"wlp2s0","protocol":"dhcp","prefsrc":"192.168.1.23","metric":600,"flags":[]},
{"dst":"default","gateway":"192.168.1.1","dev":"enp3s0","protocol":"dhcp","prefsrc":"192.168.1.20","metric":100,"flags":[]},
{"dst":"192.168.1.0/24","dev":"enp3s0","protocol":"kernel","scope":"link","prefsrc":"192.168.1.20","metric":100,"flags":[]}]`

func TestParseLinuxRoutes(t *testing.T) {
	r, err := ParseLinuxRoutes(ipRouteSample)
	if err != nil || r.Dev != "enp3s0" || r.Gateway != "192.168.1.1" || r.Src != "192.168.1.20" {
		t.Fatalf("lowest metric real adapter should win over the VPN: %+v %v", r, err)
	}
	only := `[{"dst":"default","dev":"ppp0","metric":0}]`
	if r, err := ParseLinuxRoutes(only); err != nil || r.Dev != "ppp0" || r.Gateway != "" {
		t.Fatalf("a VPN alone is used: %+v %v", r, err)
	}
	if _, err := ParseLinuxRoutes(`[{"dst":"10.0.0.0/8","dev":"eth0"}]`); err == nil {
		t.Fatal("no default route must be an error")
	}
	if _, err := ParseLinuxRoutes(`Cannot open netlink socket`); err == nil {
		t.Fatal("garbage must be an error")
	}
}

const ipAddrSample = `[
{"ifindex":1,"ifname":"lo","flags":["LOOPBACK","UP","LOWER_UP"],"operstate":"UNKNOWN","addr_info":[{"family":"inet","local":"127.0.0.1","prefixlen":8,"scope":"host"}]},
{"ifindex":2,"ifname":"enp3s0","flags":["NO-CARRIER","BROADCAST","MULTICAST","UP"],"operstate":"DOWN","addr_info":[]},
{"ifindex":3,"ifname":"wlp2s0","flags":["BROADCAST","MULTICAST","UP","LOWER_UP"],"operstate":"UP","addr_info":[{"family":"inet","local":"192.168.1.23","prefixlen":24,"scope":"global"},{"family":"inet6","local":"fe80::1","prefixlen":64,"scope":"link"}]},
{"ifindex":4,"ifname":"wg0","flags":["POINTOPOINT","NOARP","UP","LOWER_UP"],"operstate":"UNKNOWN","addr_info":[{"family":"inet","local":"10.2.0.2","prefixlen":32,"scope":"global"}]},
{"ifindex":5,"ifname":"docker0","flags":["NO-CARRIER","BROADCAST","MULTICAST","UP"],"operstate":"DOWN","addr_info":[{"family":"inet","local":"172.17.0.1","prefixlen":16,"scope":"global"}]}]`

func TestParseLinuxAddrs(t *testing.T) {
	a, err := ParseLinuxAddrs(ipAddrSample)
	if err != nil || len(a) != 5 {
		t.Fatalf("%v %v", a, err)
	}
	w, _ := LinuxFindAddr(a, "wlp2s0")
	if !w.Up || w.IP != "192.168.1.23" || w.Prefix != 24 {
		t.Fatalf("%+v", w)
	}
	e, _ := LinuxFindAddr(a, "enp3s0")
	if e.Up || e.IP != "" {
		t.Fatalf("a cable that is out is down: %+v", e)
	}
	if !LinuxVPNActive(a) {
		t.Fatal("wg0 is up, so a VPN is active")
	}
	if LinuxVPNActive(a[:3]) {
		t.Fatal("no VPN adapter in this list")
	}
	if _, ok := LinuxFindAddr(a, "nope"); ok {
		t.Fatal("unknown name")
	}
	if _, err := ParseLinuxAddrs("not json"); err == nil {
		t.Fatal("garbage must be an error")
	}
}

func TestLinuxPickPhysical(t *testing.T) {
	a, _ := ParseLinuxAddrs(ipAddrSample)
	p, ok := LinuxPickPhysical(a)
	if !ok || p.Name != "wlp2s0" {
		t.Fatalf("an adapter that is up with an address is picked: %+v", p)
	}
	p, ok = LinuxPickPhysical(a[:2])
	if !ok || p.Name != "enp3s0" {
		t.Fatalf("with nothing up, the first real adapter: %+v", p)
	}
	if _, ok := LinuxPickPhysical(a[:1]); ok {
		t.Fatal("loopback is not a real adapter")
	}
}

func TestLinuxSysFiles(t *testing.T) {
	if v, ok := ParseLinuxInt("1000\n"); !ok || v != 1000 {
		t.Fatal("speed")
	}
	if v, ok := ParseLinuxInt("-1\n"); !ok || v != -1 {
		t.Fatal("unknown speed is -1")
	}
	if _, ok := ParseLinuxInt("Invalid argument"); ok {
		t.Fatal("not a number")
	}
	if LinuxDuplex("full\n") != "full" || LinuxDuplex("Half") != "half" || LinuxDuplex("unknown\n") != "" || LinuxDuplex("") != "" {
		t.Fatal("duplex")
	}
}

const ethtoolSample = `Settings for enp3s0:
	Supported ports: [ TP ]
	Supported link modes:   10baseT/Half 10baseT/Full
	                        100baseT/Half 100baseT/Full
	                        1000baseT/Full
	Supported pause frame use: Symmetric Receive-only
	Supports auto-negotiation: Yes
	Advertised auto-negotiation: Yes
	Speed: 100Mb/s
	Duplex: Half
	Auto-negotiation: on
	Port: Twisted Pair
	Link detected: yes
`

const ethtoolUnknown = `Settings for wlp2s0:
	Supports auto-negotiation: No
	Speed: Unknown!
	Duplex: Unknown! (255)
	Port: Twisted Pair
	Link detected: no
`

func TestParseLinuxEthtool(t *testing.T) {
	if m, d := ParseLinuxEthtool(ethtoolSample); m != 100 || d != "half" {
		t.Fatalf("%d %q", m, d)
	}
	if m, d := ParseLinuxEthtool(ethtoolUnknown); m != 0 || d != "" {
		t.Fatalf("unknown must stay unknown: %d %q", m, d)
	}
	if m, d := ParseLinuxEthtool("Cannot get device settings: Operation not permitted"); m != 0 || d != "" {
		t.Fatal("not readable")
	}
	drv, ver := ParseLinuxEthtoolDriver("driver: iwlwifi\nversion: 6.8.0-45-generic\nfirmware-version: 86.fb5c9aeb.0 so-a0-gf-a0-86.ucode\nbus-info: 0000:00:14.3\n")
	if drv != "iwlwifi" || ver != "6.8.0-45-generic" {
		t.Fatalf("%q %q", drv, ver)
	}
	if drv, ver := ParseLinuxEthtoolDriver("driver: virtio_net\nversion: \n"); drv != "virtio_net" || ver != "" {
		t.Fatalf("missing version: %q %q", drv, ver)
	}
}

const iwLinkSample = `Connected to 3c:84:6a:aa:bb:cc (on wlp2s0)
	SSID: HomeNet
	freq: 5180
	RX: 123456789 bytes (98765 packets)
	TX: 23456789 bytes (45678 packets)
	signal: -58 dBm
	rx bitrate: 866.7 MBit/s VHT-MCS 9 80MHz short GI VHT-NSS 2
	tx bitrate: 650.0 MBit/s VHT-MCS 7 80MHz short GI VHT-NSS 2

	bss flags:	short-slot-time
	dtim period:	1
	beacon int:	100
`

func TestParseLinuxIwLink(t *testing.T) {
	i, mbps, ok := ParseLinuxIwLink(iwLinkSample)
	if !ok || i.Channel != 36 || i.Band != "5 GHz" || i.SignalPct != 84 || mbps != 650 {
		t.Fatalf("%+v %d %v", i, mbps, ok)
	}
	if _, _, ok := ParseLinuxIwLink("Not connected.\n"); ok {
		t.Fatal("not connected")
	}
	// Newer iw prints a decimal frequency; the signal line may carry an average.
	i, _, ok = ParseLinuxIwLink("Connected to aa:bb:cc:dd:ee:ff (on wlan0)\n\tfreq: 2437.0\n\tsignal: -71 dBm\n")
	if !ok || i.Channel != 6 || i.Band != "2.4 GHz" || i.SignalPct != 58 {
		t.Fatalf("%+v", i)
	}
	// Missing fields stay empty.
	i, mbps, ok = ParseLinuxIwLink("Connected to aa:bb:cc:dd:ee:ff (on wlan0)\n")
	if !ok || i.Channel != 0 || i.SignalPct != 0 || mbps != 0 {
		t.Fatalf("%+v", i)
	}
	if _, _, ok := ParseLinuxIwLink("command failed: Operation not supported (-95)"); ok {
		t.Fatal("error text is not a link")
	}
}

func TestLinuxChannelAndSignal(t *testing.T) {
	for f, want := range map[int]int{2412: 1, 2437: 6, 2462: 11, 2484: 14, 5180: 36, 5745: 149, 5955: 1, 6115: 33} {
		if c, _ := LinuxChannel(f); c != want {
			t.Errorf("freq %d: got channel %d want %d", f, c, want)
		}
	}
	if c, b := LinuxChannel(900); c != 0 || b != "" {
		t.Fatal("unknown frequency")
	}
	if _, b := LinuxChannel(5955); b != "6 GHz" {
		t.Fatal("6 GHz band")
	}
	if LinuxSignalPct(-30) != 100 || LinuxSignalPct(-100) != 0 || LinuxSignalPct(-110) != 0 || LinuxSignalPct(-75) != 50 {
		t.Fatal("signal percent")
	}
}

const iwScanSample = `BSS 3c:84:6a:aa:bb:cc(on wlp2s0) -- associated
	last seen: 120.0s [boottime]
	freq: 5180
	beacon interval: 100 TUs
	signal: -58.00 dBm
	SSID: HomeNet
BSS 10:20:30:40:50:60(on wlp2s0)
	freq: 2437
	signal: -80.00 dBm
	SSID: Neighbor
BSS 10:20:30:40:50:61(on wlp2s0)
	freq: 2437
	SSID: Neighbor2
BSS 10:20:30:40:50:62(on wlp2s0)
	freq: 5200
	SSID: Far
`

func TestParseLinuxIwScanDump(t *testing.T) {
	got := ParseLinuxIwScanDump(iwScanSample)
	if !reflect.DeepEqual(got, []int{6, 6, 40}) {
		t.Fatalf("own network must be left out: %v", got)
	}
	if got := ParseLinuxIwScanDump(""); len(got) != 0 {
		t.Fatalf("nothing read gives nothing: %v", got)
	}
	if got := ParseLinuxIwScanDump("command failed: Operation not permitted (-1)\n"); len(got) != 0 {
		t.Fatalf("not readable gives nothing, no guess: %v", got)
	}
}

func TestParseLinuxNmcliWifi(t *testing.T) {
	out := "*:36:5180 MHz:78\n :6:2437 MHz:40\n :6:2437 MHz:35\n :149:5745 MHz:20\n"
	rows := ParseLinuxNmcliWifi(out)
	if len(rows) != 4 || !rows[0].InUse || rows[0].Channel != 36 || rows[0].Freq != 5180 || rows[0].SignalPct != 78 || rows[1].InUse {
		t.Fatalf("%+v", rows)
	}
	if rows := ParseLinuxNmcliWifi("Error: NetworkManager is not running.\n"); len(rows) != 0 {
		t.Fatalf("error text is not a list: %+v", rows)
	}
	if rows := ParseLinuxNmcliWifi(""); len(rows) != 0 {
		t.Fatal("empty")
	}
	// A row with a missing frequency still keeps its channel.
	if rows := ParseLinuxNmcliWifi(" :11::50\n"); len(rows) != 1 || rows[0].Channel != 11 || rows[0].Freq != 0 {
		t.Fatalf("%+v", rows)
	}
}

const netDevSample = `Inter-|   Receive                                                |  Transmit
 face |bytes    packets errs drop fifo frame compressed multicast|bytes    packets errs drop fifo colls carrier compressed
    lo:  123456     1000    0    0    0     0          0         0   123456     1000    0    0    0     0       0          0
wlp2s0:9876543210 7654321    0    0    0     0          0         0 1234567890 4321000    0    0    0     0       0          0
enp3s0:       0       0    0    0    0     0          0         0        0       0    0    0    0     0       0          0
`

func TestParseLinuxNetDev(t *testing.T) {
	m := ParseLinuxNetDev(netDevSample)
	if len(m) != 3 || m["wlp2s0"].Rx != 9876543210 || m["wlp2s0"].Tx != 1234567890 || m["lo"].Rx != 123456 {
		t.Fatalf("%+v", m)
	}
	if len(ParseLinuxNetDev("")) != 0 || len(ParseLinuxNetDev("garbage: a b c")) != 0 {
		t.Fatal("unreadable gives nothing")
	}
}

const ssSample = `0      0         192.168.1.23:44322    142.250.80.46:443   users:(("firefox",pid=2345,fd=89))
0      0         192.168.1.23:44324    142.250.80.46:443   users:(("firefox",pid=2345,fd=91))
0      0         192.168.1.23:51000    52.1.2.3:443        users:(("code",pid=3000,fd=20),("code",pid=3001,fd=21))
0      0         192.168.1.23:22       192.168.1.40:50000
0      0         [fe80::1]:5353        [fe80::2]:1234      users:(("avahi-daemon",pid=700,fd=12))
`

func TestParseLinuxSS(t *testing.T) {
	p := ParseLinuxSS(ssSample)
	if len(p) != 3 || p[0].Name != "firefox" || p[0].Conns != 2 || p[1].Name != "avahi-daemon" || p[2].Name != "code" || p[2].Conns != 1 {
		t.Fatalf("ranked by connections, no-name lines left out: %+v", p)
	}
	if got := ParseLinuxSS("0 0 1.2.3.4:5 6.7.8.9:10\n"); len(got) != 0 {
		t.Fatalf("no names without rights, no guess: %+v", got)
	}
	if got := ParseLinuxSS(""); len(got) != 0 {
		t.Fatal("empty")
	}
}

const arpSample = `IP address       HW type     Flags       HW address            Mask     Device
192.168.1.50     0x1         0x0         00:00:00:00:00:00     *        wlp2s0
192.168.1.1      0x1         0x2         3c:84:6a:aa:bb:cc     *        wlp2s0
192.168.1.77     0x1         0x2         b8:27:eb:11:22:33     *        wlp2s0
192.168.1.9      0x1         0x2         d4:6e:0e:00:00:01     *        wlp2s0
192.168.1.60     0x1         0x6         da:a1:19:00:00:01     *        wlp2s0
192.168.1.99     0x1         0x2         01:00:5e:00:00:fb     *        wlp2s0
192.168.1.255    0x1         0x2         ff:ff:ff:ff:ff:ff     *        wlp2s0
192.168.1.30     0x1         0x2         aa:bb:cc:dd:ee:ff     *        docker0
10.0.0.5         0x1         0x2         aa:bb:cc:dd:ee:01     *        wlp2s0
192.168.1.44     0x1         0x2         00:00:00:00:00:00     *        wlp2s0
`

func TestParseLinuxArp(t *testing.T) {
	n := ParseLinuxArp(arpSample, "wlp2s0", "192.168.1.")
	var ips []string
	for _, x := range n {
		ips = append(ips, x.IP)
	}
	want := []string{"192.168.1.1", "192.168.1.9", "192.168.1.60", "192.168.1.77"}
	if !reflect.DeepEqual(ips, want) {
		t.Fatalf("incomplete, multicast, broadcast, other adapter and other subnet must go: %v", ips)
	}
	if n[0].MAC != "3C:84:6A:AA:BB:CC" {
		t.Fatalf("addresses use one form: %q", n[0].MAC)
	}
	if Maker(n[3].MAC) != "Raspberry Pi" {
		t.Fatal("maker from the existing list")
	}
	if len(ParseLinuxArp(arpSample, "", "192.168.1.")) != 5 {
		t.Fatal("with no adapter named, the docker0 entry is kept")
	}
	if len(ParseLinuxArp("", "wlp2s0", "192.168.1.")) != 0 || len(ParseLinuxArp("IP address HW type\n", "", "192.168.1.")) != 0 {
		t.Fatal("empty table")
	}
}

const resolvectlSample = `Global
         Protocols: -LLMNR -mDNS -DNSOverTLS DNSSEC=no/unsupported
  resolv.conf mode: stub
Fallback DNS Servers: 1.1.1.1#cloudflare-dns.com 9.9.9.10#dns10.quad9.net

Link 2 (enp3s0)
    Current Scopes: none
         Protocols: -DefaultRoute -LLMNR -mDNS -DNSOverTLS DNSSEC=no/unsupported

Link 3 (wlp2s0)
    Current Scopes: DNS
         Protocols: +DefaultRoute -LLMNR -mDNS -DNSOverTLS DNSSEC=no/unsupported
Current DNS Server: 192.168.1.1
       DNS Servers: 192.168.1.1 fe80::1%3
                    8.8.8.8
        DNS Domain: lan
`

func TestParseLinuxResolvectl(t *testing.T) {
	got := ParseLinuxResolvectl(resolvectlSample, "wlp2s0")
	if !reflect.DeepEqual(got, []string{"192.168.1.1", "fe80::1", "8.8.8.8"}) {
		t.Fatalf("servers of the adapter in use, including the wrapped line: %v", got)
	}
	// Servers set for the whole system.
	g := "Global\n       DNS Servers: 9.9.9.9 149.112.112.112\n\nLink 2 (eth0)\n      Current Scopes: none\n"
	if got := ParseLinuxResolvectl(g, "eth0"); !reflect.DeepEqual(got, []string{"9.9.9.9", "149.112.112.112"}) {
		t.Fatalf("global servers when the adapter has none: %v", got)
	}
	// Fallback servers are not the ones in use.
	if got := ParseLinuxResolvectl(resolvectlSample, "enp3s0"); len(got) != 3 {
		t.Fatalf("other adapter servers when this one has none: %v", got)
	}
	if got := ParseLinuxResolvectl("Failed to get global data: Unit dbus-org.freedesktop.resolve1.service not found.\n", "eth0"); len(got) != 0 {
		t.Fatalf("not readable gives nothing: %v", got)
	}
}

func TestParseLinuxResolvConf(t *testing.T) {
	stub := "# This is /run/systemd/resolve/stub-resolv.conf managed by man:systemd-resolved(8).\nnameserver 127.0.0.53\noptions edns0 trust-ad\nsearch lan\n"
	got := ParseLinuxResolvConf(stub)
	if !reflect.DeepEqual(got, []string{"127.0.0.53"}) || !LinuxIsStub(got) {
		t.Fatalf("the stub is known as the stub: %v", got)
	}
	real := "; comment\nnameserver 192.168.1.1\nnameserver 8.8.8.8\nnameserver 192.168.1.1\nnameserver not-an-address\n"
	got = ParseLinuxResolvConf(real)
	if !reflect.DeepEqual(got, []string{"192.168.1.1", "8.8.8.8"}) || LinuxIsStub(got) {
		t.Fatalf("%v", got)
	}
	if got := ParseLinuxResolvConf(""); len(got) != 0 || LinuxIsStub(got) {
		t.Fatal("empty file")
	}
}

func TestParseLinuxSmallFacts(t *testing.T) {
	if k, on := ParseLinuxPowerSave("Power save: on\n"); !k || !on {
		t.Fatal("on")
	}
	if k, on := ParseLinuxPowerSave("Power save: off\n"); !k || on {
		t.Fatal("off")
	}
	if k, _ := ParseLinuxPowerSave("command failed: Operation not supported (-95)\n"); k {
		t.Fatal("not readable means unknown")
	}
	if !ParseLinuxProxyMode("'manual'\n") || !ParseLinuxProxyMode("'auto'") || ParseLinuxProxyMode("'none'\n") || ParseLinuxProxyMode("") {
		t.Fatal("gsettings proxy mode")
	}
	if ParseLinuxProxyMode("No such schema 'org.gnome.system.proxy'") {
		t.Fatal("error text is not a mode")
	}
	env := map[string]string{"HTTPS_PROXY": "http://proxy:3128"}
	if !LinuxProxyEnv(func(k string) string { return env[k] }) || LinuxProxyEnv(func(string) string { return " " }) {
		t.Fatal("proxy variables")
	}
	if e := ParseLinuxNmcliState("GENERAL.STATE:120 (connection failed)\n"); len(e) != 1 {
		t.Fatalf("failed: %v", e)
	}
	if e := ParseLinuxNmcliState("GENERAL.STATE:60 (needs authentication)\n"); len(e) != 1 {
		t.Fatalf("needs login: %v", e)
	}
	if e := ParseLinuxNmcliState("GENERAL.STATE:100 (connected)\n"); len(e) != 0 {
		t.Fatalf("connected is fine: %v", e)
	}
	if e := ParseLinuxNmcliState("Error: NetworkManager is not running.\n"); len(e) != 0 {
		t.Fatalf("not readable: %v", e)
	}
}

const journalSample = `2026-11-10T08:15:32+0000 host kernel: e1000e 0000:00:1f.6 enp3s0: NIC Link is Down
2026-11-10T08:15:40+0000 host kernel: e1000e 0000:00:1f.6 enp3s0: NIC Link is Up 1000 Mbps Full Duplex, Flow Control: Rx/Tx
2026-11-10T08:15:40+0000 host kernel: IPv6: ADDRCONF(NETDEV_CHANGE): enp3s0: link becomes ready
2026-11-10T09:00:00+0000 host kernel: wlp2s0: deauthenticating from 3c:84:6a:aa:bb:cc by local choice (Reason: 3=DEAUTH_LEAVING)
2026-11-10T09:00:05+0000 host kernel: wlp2s0: associated
2026-11-10T09:30:00+0000 host kernel: br-1234: port 1(veth0) entered disabled state
2026-11-10T10:00:00.123456+0100 host kernel: r8169 0000:03:00.0 enp3s0: Link is Down
2026-11-10T10:00:30+0100 host kernel: r8169 0000:03:00.0 enp3s0: Link is Up - 1Gbps/Full - flow control rx/tx
-- No entries --
`

func TestParseLinuxJournalKernel(t *testing.T) {
	ev, err := ParseLinuxJournal(journalSample, "enp3s0")
	if err != nil || len(ev) != 5 {
		t.Fatalf("%+v %v", ev, err)
	}
	wantUp := []bool{false, true, true, false, true}
	for i, e := range ev {
		if e.Up != wantUp[i] {
			t.Errorf("event %d: Up=%v", i, e.Up)
		}
	}
	if ev[0].Time.UTC().Format(time.RFC3339) != "2026-11-10T08:15:32Z" || ev[3].Time.UTC().Format(time.RFC3339) != "2026-11-10T09:00:00Z" {
		t.Fatalf("times: %v %v", ev[0].Time, ev[3].Time)
	}
	merged := LinuxMergeEvents(ev)
	if len(merged) != 4 || merged[0].Up {
		t.Fatalf("repeat of the same state is dropped: %+v", merged)
	}
	w, _ := ParseLinuxJournal(journalSample, "wlp2s0")
	if len(w) != 2 || w[0].Up || !w[1].Up {
		t.Fatalf("Wi-Fi leave then join: %+v", w)
	}
	// The word match must not take eth0 for eth0.100 or enp3s0 for enp3s01.
	x, _ := ParseLinuxJournal("2026-11-10T08:00:00+0000 h kernel: enp3s01: Link is Down\n", "enp3s0")
	if len(x) != 0 {
		t.Fatalf("different adapter: %+v", x)
	}
}

func TestParseLinuxJournalNetworkManager(t *testing.T) {
	nm := `2026-11-10T09:00:00+0000 host NetworkManager[812]: <info>  [1731229200.1] device (wlp2s0): state change: activated -> disconnected (reason 'ssid-not-found', sys-iface-state: 'managed')
2026-11-10T09:00:03+0000 host NetworkManager[812]: <info>  [1731229203.2] device (wlp2s0): state change: ip-check -> secondaries (reason 'none', sys-iface-state: 'managed')
2026-11-10T09:00:06+0000 host NetworkManager[812]: <info>  [1731229206.3] device (wlp2s0): state change: secondaries -> activated (reason 'none', sys-iface-state: 'managed')
2026-11-10T09:10:00+0000 host NetworkManager[812]: <info>  [1731229800.0] device (docker0): state change: activated -> unmanaged (reason 'removed')
`
	ev, err := ParseLinuxJournal(nm, "wlp2s0")
	if err != nil || len(ev) != 2 || ev[0].Up || !ev[1].Up {
		t.Fatalf("%+v %v", ev, err)
	}
}

func TestParseLinuxJournalNotReadable(t *testing.T) {
	for _, s := range []string{
		"",
		"-- No entries --\n",
		"Hint: You are currently not seeing messages from other users and the system.\n      Users in groups 'adm', 'systemd-journal' can see all messages.\n-- No entries --\n",
		"No journal files were found.\n",
	} {
		if ev, err := ParseLinuxJournal(s, "wlp2s0"); err == nil || ev != nil {
			t.Errorf("%q must be an error, got %v %v", s, ev, err)
		}
	}
	// A readable log with no link changes is not an error.
	ev, err := ParseLinuxJournal("2026-11-10T08:00:00+0000 h kernel: something else\n", "wlp2s0")
	if err != nil || len(ev) != 0 {
		t.Fatalf("%v %v", ev, err)
	}
}

func TestLinuxMergeEventsOrder(t *testing.T) {
	k, _ := ParseLinuxJournal("2026-11-10T09:00:05+0000 h kernel: wlp2s0: associated\n2026-11-10T09:00:00+0000 h kernel: wlp2s0: disassociated\n", "wlp2s0")
	n, _ := ParseLinuxJournal("2026-11-10T09:00:06+0000 h NetworkManager[1]: device (wlp2s0): state change: secondaries -> activated (reason 'none')\n", "wlp2s0")
	m := LinuxMergeEvents(append(k, n...))
	if len(m) != 2 || m[0].Up || !m[1].Up || !m[0].Time.Before(m[1].Time) {
		t.Fatalf("sorted, with the repeat from the second log dropped: %+v", m)
	}
}

func TestLinuxErrorsArePlain(t *testing.T) {
	// Error text reaches the user, so keep it short and free of tool jargon.
	errs := []string{}
	_, e1 := ParseLinuxRoutes("x")
	_, e2 := ParseLinuxAddrs("x")
	_, e3 := ParseLinuxJournal("", "")
	for _, e := range []error{e1, e2, e3} {
		errs = append(errs, e.Error())
	}
	for _, s := range errs {
		if len(strings.Fields(s)) > 14 || strings.HasSuffix(s, ".") {
			t.Errorf("error text too long or has a final period: %q", s)
		}
	}
}
