package collect

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// ND-01, ND-163: every command the Mac reader may run only reads.
func TestDarwinCommandsOnlyRead(t *testing.T) {
	okNames := map[string]bool{"route": true, "ifconfig": true, "networksetup": true, "netstat": true,
		"arp": true, "scutil": true, "system_profiler": true, "lsof": true, "ping": true}
	prefixes := []string{"set", "add", "delete", "flush", "restart", "start", "stop", "enable", "disable",
		"modify", "write", "remove", "change", "create", "renew", "release", "reset", "kill", "load", "unload"}
	exact := []string{"up", "down", "sudo", "rm", "mv", "on", "off", "clear"}
	for _, c := range darwinCommands {
		if !okNames[c[0]] {
			t.Errorf("command %q is not an approved reader", c[0])
		}
		for _, a := range c[1:] {
			w := strings.ToLower(strings.TrimLeft(a, "-"))
			for _, p := range prefixes {
				if strings.HasPrefix(w, p) {
					t.Errorf("%v uses change word %q", c, a)
				}
			}
			for _, e := range exact {
				if w == e {
					t.Errorf("%v uses change word %q", c, a)
				}
			}
		}
		if c[0] == "ifconfig" && len(c) != 2 {
			t.Errorf("ifconfig may only be given an adapter name: %v", c)
		}
		if c[0] == "networksetup" && c[1] != "-listallhardwareports" {
			t.Errorf("networksetup may only list ports: %v", c)
		}
	}
}

func TestDarwinRunRefusesUnlisted(t *testing.T) {
	bad := [][]string{
		{"ifconfig", "en0", "down"},
		{"ifconfig", "en0 up"},
		{"networksetup", "-setdnsservers", "Wi-Fi", "1.1.1.1"},
		{"route", "-n", "delete", "default"},
		{"scutil", "--set", "HostName", "x"},
		{"sudo", "ls"},
		{"ping", "-c", "1", "-W", "300", "not-an-address"},
		{"arp", "-d", "-a"},
	}
	for _, b := range bad {
		if darwinAllowed(b[0], b[1:]) {
			t.Errorf("should be refused: %v", b)
		}
		if out, err := darwinRun(context.Background(), b[0], b[1:]...); err == nil || out != "" {
			t.Errorf("darwinRun ran %v", b)
		}
	}
	if !darwinAllowed("ifconfig", []string{"en0"}) || !darwinAllowed("scutil", []string{"--dns"}) ||
		!darwinAllowed("ping", []string{"-c", "1", "-W", "300", "192.168.1.5"}) {
		t.Error("listed commands should be allowed")
	}
}

const routeOut = `   route to: default
destination: default
       mask: default
    gateway: 192.168.1.1
  interface: en0
      flags: <UP,GATEWAY,DONE,STATIC,PRCLONING,GLOBAL>
 recvpipe  sendpipe  ssthresh  rtt,msec    rttvar  hopcount      mtu     expire
       0         0         0         0         0         0     1500         0
`

const routeLinkOut = `   route to: default
destination: default
    gateway: link#18
  interface: utun4
`

func TestDarwinParseRoute(t *testing.T) {
	if i, g := darwinParseRoute(routeOut); i != "en0" || g != "192.168.1.1" {
		t.Fatal(i, g)
	}
	if i, g := darwinParseRoute(routeLinkOut); i != "utun4" || g != "" {
		t.Fatalf("a gateway that is not an address must stay empty: %q %q", i, g)
	}
	if i, _ := darwinParseRoute("route: writing to routing socket: not in table"); i != "" {
		t.Fatal("no route means no adapter")
	}
}

const ifWired = `en5: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500
	options=6467<RXCSUM,TXCSUM,VLAN_MTU,TSO4,TSO6,CHANNEL_IO,PARTIAL_CSUM,ZEROINVERT_CSUM>
	ether 00:e0:4c:68:00:01
	inet6 fe80::1c2b:3d4e:5f60:7182%en5 prefixlen 64 secured scopeid 0x9
	inet 10.0.4.23 netmask 0xfffffe00 broadcast 10.0.5.255
	nd6 options=201<PERFORMNUD,DAD>
	media: autoselect (1000baseT <full-duplex,flow-control>)
	status: active
`

const ifWiFi = `en0: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500
	options=400<CHANNEL_IO>
	ether a0:78:17:0a:0b:0c
	inet6 fe80::1840:aaaa:bbbb:cccc%en0 prefixlen 64 secured scopeid 0xe
	inet 192.168.1.23 netmask 0xffffff00 broadcast 192.168.1.255
	nd6 options=201<PERFORMNUD,DAD>
	media: autoselect
	status: active
`

const ifDown = `en5: flags=8822<BROADCAST,SMART,SIMPLEX,MULTICAST> mtu 1500
	ether 00:e0:4c:68:00:01
	media: autoselect (none)
	status: inactive
`

const ifTen = `en7: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500
	inet 172.16.0.9 netmask 0xffff0000 broadcast 172.16.255.255
	media: autoselect (10Gbase-T <full-duplex,flow-control,energy-efficient-ethernet>)
`

const ifHalf = `en6: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500
	inet 192.168.0.5 netmask 0xffffff00 broadcast 192.168.0.255
	media: autoselect (100baseTX <half-duplex>)
	status: active
`

func TestDarwinParseIfconfig(t *testing.T) {
	w := darwinParseIfconfig(ifWired)
	if !w.Up || w.Mbps != 1000 || w.Duplex != "full" || w.IP != "10.0.4.23" || w.Prefix != 23 {
		t.Fatalf("%+v", w)
	}
	f := darwinParseIfconfig(ifWiFi)
	if !f.Up || f.Mbps != 0 || f.Duplex != "" || f.IP != "192.168.1.23" || f.Prefix != 24 {
		t.Fatalf("wifi gives no speed and no duplex: %+v", f)
	}
	if d := darwinParseIfconfig(ifDown); d.Up || d.Mbps != 0 || d.IP != "" {
		t.Fatalf("%+v", d)
	}
	if x := darwinParseIfconfig(ifTen); x.Mbps != 10000 || x.Prefix != 16 || !x.Up {
		t.Fatalf("no status line: flags decide, G means thousand: %+v", x)
	}
	if h := darwinParseIfconfig(ifHalf); h.Mbps != 100 || h.Duplex != "half" {
		t.Fatalf("%+v", h)
	}
	if darwinPrefix("255.255.255.0") != 24 || darwinPrefix("junk") != 0 {
		t.Fatal("prefix")
	}
}

const portsOut = `
Hardware Port: Ethernet Adapter (en5)
Device: en5
Ethernet Address: 00:e0:4c:68:00:01

Hardware Port: Wi-Fi
Device: en0
Ethernet Address: a0:78:17:0a:0b:0c

Hardware Port: Thunderbolt Bridge
Device: bridge0
Ethernet Address: 36:00:0a:0b:0c:0d

VLAN Configurations
===================
`

func TestDarwinParsePorts(t *testing.T) {
	p := darwinParsePorts(portsOut)
	if p["en0"].Kind != core.WiFi || p["en0"].Name != "Wi-Fi" {
		t.Fatalf("%+v", p["en0"])
	}
	if p["en5"].Kind != core.Wired {
		t.Fatalf("%+v", p["en5"])
	}
	if p["bridge0"].Kind != core.Unknown {
		t.Fatalf("a bridge is neither: %+v", p["bridge0"])
	}
	if _, ok := p["en9"]; ok {
		t.Fatal("unknown device")
	}
}

const netstatA = `Name       Mtu   Network       Address            Ipkts Ierrs     Ibytes    Opkts Oerrs     Obytes  Coll
lo0        16384 <Link#1>                          500000     0   90000000   500000     0   90000000     0
lo0        16384 127           127.0.0.1           500000     -   90000000   500000     -   90000000     -
en0        1500  <Link#14>   a0:78:17:a:b:c      9000000     3 1000000000  4000000     1  200000000     0
en0        1500  192.168.1     192.168.1.23      9000000     -  1000000000  4000000     -  200000000     -
en5*       1500  <Link#9>    0:e0:4c:68:0:1            0     0          0        0     0          0     0
`

const netstatB = `Name       Mtu   Network       Address            Ipkts Ierrs     Ibytes    Opkts Oerrs     Obytes  Coll
lo0        16384 <Link#1>                          500010     0   90001000   500010     0   90001000     0
en0        1500  <Link#14>   a0:78:17:a:b:c      9000100     3 1004000000  4000050     1  200400000     0
`

func TestDarwinParseNetstat(t *testing.T) {
	a := darwinParseNetstat(netstatA)
	want := darwinCounters{InBytes: 1000000000, OutBytes: 200000000, InErrs: 3, OutErrs: 1}
	if a["en0"] != want {
		t.Fatalf("%+v", a["en0"])
	}
	if l := a["lo0"]; l.InBytes != 90000000 || l.InErrs != 0 {
		t.Fatalf("empty address column must not shift the numbers: %+v", l)
	}
	if _, ok := a["en5"]; !ok {
		t.Fatal("a star after a name must be dropped")
	}
	b := darwinParseNetstat(netstatB)
	r := darwinRates(a["en0"], b["en0"], 2, 1000)
	if r.RxMBps != 2 || r.TxMBps != 0.2 || r.BusyPct != 1 {
		t.Fatalf("%+v", r)
	}
	if z := darwinRates(b["en0"], a["en0"], 2, 1000); z.RxMBps != 0 || z.TxMBps != 0 {
		t.Fatalf("a counter going backwards counts as zero: %+v", z)
	}
	if z := darwinRates(a["en0"], b["en0"], 2, 0); z.BusyPct != 0 {
		t.Fatal("no link speed means no busy percent")
	}
}

const lsofOut = `COMMAND     PID  USER   FD   TYPE             DEVICE SIZE/OFF NODE NAME
Google\x20  801 rafiq   30u  IPv4 0x1a2b3c4d5e6f7081      0t0  TCP 192.168.1.23:50111->142.250.80.46:443 (ESTABLISHED)
Google\x20  801 rafiq   31u  IPv4 0x1a2b3c4d5e6f7082      0t0  TCP 192.168.1.23:50112->142.250.80.47:443 (ESTABLISHED)
Google\x20  801 rafiq   32u  IPv6 0x1a2b3c4d5e6f7083      0t0  TCP [2001:db8::5]:50113->[2607:f8b0::2]:443 (ESTABLISHED)
Slack       922 rafiq   20u  IPv4 0x1a2b3c4d5e6f7084      0t0  TCP 192.168.1.23:50200->52.1.2.3:443 (ESTABLISHED)
node       1200 rafiq   15u  IPv4 0x1a2b3c4d5e6f7085      0t0  TCP 127.0.0.1:3000->127.0.0.1:50300 (ESTABLISHED)
node       1200 rafiq   16u  IPv4 0x1a2b3c4d5e6f7086      0t0  TCP 127.0.0.1:50300->127.0.0.1:3000 (ESTABLISHED)
`

func TestDarwinParseLsof(t *testing.T) {
	p := darwinParseLsof(lsofOut)
	if len(p) != 2 || p[0].Name != "Google " || p[0].Conns != 3 || p[1].Name != "Slack" || p[1].Conns != 1 {
		t.Fatalf("ranked by connections, loopback left out: %+v", p)
	}
	if len(darwinParseLsof("")) != 0 {
		t.Fatal("empty")
	}
}

const arpOut = `? (192.168.1.1) at a:b:c:1:2:3 on en0 ifscope [ethernet]
? (192.168.1.20) at (incomplete) on en0 ifscope [ethernet]
? (192.168.1.7) at 3c:22:fb:0:aa:f on en0 ifscope [ethernet]
? (192.168.1.255) at ff:ff:ff:ff:ff:ff on en0 ifscope [ethernet]
? (224.0.0.251) at 1:0:5e:0:0:fb on en0 ifscope permanent [ethernet]
? (192.168.1.40) at ff:ff:ff:ff:ff:ff on en0 ifscope [ethernet]
router.lan (192.168.1.2) at DC:A6:32:0:0:1 on en0 ifscope [ethernet]
? (10.0.0.9) at 4:5:6:7:8:9 on en5 ifscope [ethernet]
? (192.168.1.1) at a:b:c:1:2:3 on en0 ifscope [ethernet]
`

func TestDarwinParseARP(t *testing.T) {
	got := darwinParseARP(arpOut, "")
	var s []string
	for _, n := range got {
		s = append(s, n.IP+"="+n.MAC)
	}
	want := "10.0.0.9=04:05:06:07:08:09 192.168.1.1=0A:0B:0C:01:02:03 192.168.1.2=DC:A6:32:00:00:01 192.168.1.7=3C:22:FB:00:AA:0F"
	if strings.Join(s, " ") != want {
		t.Fatalf("\n got %v\nwant %s", s, want)
	}
	if on := darwinParseARP(arpOut, "en5"); len(on) != 1 || on[0].IP != "10.0.0.9" {
		t.Fatalf("%+v", on)
	}
	if Maker(got[2].MAC) != "Raspberry Pi" {
		t.Fatal("padded address must find its maker")
	}
}

const dnsOut = `DNS configuration

resolver #1
  search domain[0] : lan
  nameserver[0] : 192.168.1.1
  nameserver[1] : 1.1.1.1
  if_index : 14 (en0)
  flags    : Request A records
  reach    : 0x00020002 (Reachable,Directly Reachable Address)

resolver #2
  domain   : local
  options  : mdns
  timeout  : 5
  flags    : Request A records
  reach    : 0x00000000 (Not Reachable)
  order    : 300000

resolver #3
  domain   : 254.169.in-addr.arpa
  options  : mdns
  timeout  : 5
  order    : 300200

DNS configuration (for scoped queries)

resolver #1
  search domain[0] : lan
  nameserver[0] : 192.168.1.1
  if_index : 14 (en0)
  flags    : Scoped, Request A records
`

const dnsVPNFirst = `DNS configuration

resolver #1
  domain   : corp.example.com
  nameserver[0] : 10.9.9.9
  flags    : Request A records

resolver #2
  nameserver[0] : 9.9.9.9
  if_index : 14 (en0)
`

const dnsOnlyScoped = `DNS configuration

resolver #1
  domain   : local
  options  : mdns

DNS configuration (for scoped queries)

resolver #1
  nameserver[0] : 8.8.4.4
  if_index : 14 (en0)
`

func TestDarwinParseDNS(t *testing.T) {
	if d := darwinParseDNS(dnsOut); strings.Join(d, ",") != "192.168.1.1,1.1.1.1" {
		t.Fatalf("first ordinary resolver: %v", d)
	}
	if d := darwinParseDNS(dnsVPNFirst); strings.Join(d, ",") != "9.9.9.9" {
		t.Fatalf("a resolver for one domain is skipped: %v", d)
	}
	if d := darwinParseDNS(dnsOnlyScoped); strings.Join(d, ",") != "8.8.4.4" {
		t.Fatalf("scoped list is the fallback: %v", d)
	}
	if d := darwinParseDNS("DNS configuration\n\nresolver #1\n  domain : local\n  options : mdns\n"); len(d) != 0 {
		t.Fatalf("mDNS only means none: %v", d)
	}
}

const proxyOn = `<dictionary> {
  ExceptionsList : <array> {
    0 : *.local
  }
  HTTPEnable : 1
  HTTPPort : 8080
  HTTPProxy : proxy.example.com
  HTTPSEnable : 0
}
`

const proxyOff = `<dictionary> {
  ExcludeSimpleHostnames : 0
  FTPPassive : 1
  HTTPEnable : 0
  HTTPSEnable : 0
  ProxyAutoDiscoveryEnable : 1
  SOCKSEnable : 0
}
`

const proxyPAC = `<dictionary> {
  ProxyAutoConfigEnable : 1
  ProxyAutoConfigURLString : http://wpad.example.com/proxy.pac
}
`

func TestDarwinParseProxy(t *testing.T) {
	if !darwinParseProxy(proxyOn) || !darwinParseProxy(proxyPAC) {
		t.Fatal("proxy on")
	}
	if darwinParseProxy(proxyOff) {
		t.Fatal("proxy off; auto-discovery alone is not a proxy")
	}
	if darwinParseProxy("") {
		t.Fatal("empty")
	}
}

const ncConnected = `Available network connection services in the current set (*=enabled):
* (Connected)      7A1B2C3D-0000-4000-8000-ABCDEF012345 VPN (com.example.vpn) "Work VPN"                [VPN:com.example.vpn]
* (Disconnected)   9F8E7D6C-0000-4000-8000-ABCDEF012345 VPN (com.wireguard.macos) "Home"               [VPN:com.wireguard.macos]
`

const ncIdle = `Available network connection services in the current set (*=enabled):
* (Disconnected)   9F8E7D6C-0000-4000-8000-ABCDEF012345 VPN (com.wireguard.macos) "Home"               [VPN:com.wireguard.macos]
`

func TestDarwinParseVPN(t *testing.T) {
	if !darwinParseVPN(ncConnected) || darwinParseVPN(ncIdle) || darwinParseVPN("") {
		t.Fatal("vpn")
	}
}

const spWiFi = `Wi-Fi:

      Software Versions:
          CoreWLAN: 16.0 (1657)
          CoreWLANKit: 16.0 (1657)
      Interfaces:
        en0:
          Card Type: Wi-Fi  (0x14E4, 0x4387)
          Firmware Version: wl0: Jun 20 2024 20:21:20 version 23.10.762.4.8.7.152 FWID 01-d0a2a1ae
          MAC Address: a0:78:17:0a:0b:0c
          Locale: FCC
          Country Code: US
          Supported PHY Modes: 802.11 a/b/g/n/ac/ax
          Supported Channels: 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 36, 40, 44, 48
          Wake On Wireless: Supported
          AirDrop: Supported
          Status: Connected
          Current Network Information:
            HomeNet:
              PHY Mode: 802.11ac
              Channel: 36 (5GHz, 80MHz)
              Country Code: US
              Network Type: Infrastructure
              Security: WPA2 Personal
              Signal / Noise: -60 dBm / -90 dBm
              Transmit Rate: 650
              MCS Index: 7
          Other Local Wi-Fi Networks:
            HomeNet:
              PHY Mode: 802.11
              Channel: 36 (5GHz, 80MHz)
              Network Type: Infrastructure
              Security: WPA2 Personal
              Signal / Noise: -61 dBm / -90 dBm
            NeighborA:
              PHY Mode: 802.11
              Channel: 6 (2GHz, 20MHz)
              Network Type: Infrastructure
              Security: WPA2 Personal
              Signal / Noise: -78 dBm / -92 dBm
            NeighborB:
              PHY Mode: 802.11
              Channel: 6 (2GHz, 20MHz)
              Network Type: Infrastructure
              Security: WPA2 Personal
              Signal / Noise: -80 dBm / -92 dBm
        awdl0:
          MAC Address: 7a:11:22:33:44:55
`

// A newer macOS hides the network name and gives no signal line.
const spWiFiMissing = `Wi-Fi:

      Interfaces:
        en0:
          Status: Connected
          Current Network Information:
            <redacted>:
              PHY Mode: 802.11ax
              Channel: 149 (5GHz, 40MHz)
              Network Type: Infrastructure
          Other Local Wi-Fi Networks:
            <redacted>:
              Channel: 1 (2GHz, 20MHz)
            <redacted>:
              Channel: 11 (2GHz, 20MHz)
`

const spWiFiOff = `Wi-Fi:

      Interfaces:
        en0:
          Status: Off
`

func TestDarwinParseWiFi(t *testing.T) {
	w := darwinParseWiFi(spWiFi)
	if !w.Connected || w.Channel != 36 || w.Band != "5 GHz" || w.SignalPct != 80 || w.TxMbps != 650 {
		t.Fatalf("%+v", w)
	}
	if len(w.OtherChannels) != 2 || w.OtherChannels[0] != 6 || w.OtherChannels[1] != 6 {
		t.Fatalf("own network is left out of the nearby list: %v", w.OtherChannels)
	}
	m := darwinParseWiFi(spWiFiMissing)
	if !m.Connected || m.Channel != 149 || m.Band != "5 GHz" || m.SignalPct != 0 || m.TxMbps != 0 {
		t.Fatalf("a missing signal stays zero: %+v", m)
	}
	if len(m.OtherChannels) != 2 {
		t.Fatalf("hidden names must not drop a neighbor: %v", m.OtherChannels)
	}
	if o := darwinParseWiFi(spWiFiOff); o.Connected || o.Channel != 0 || len(o.OtherChannels) != 0 {
		t.Fatalf("%+v", o)
	}
}

func TestDarwinSignalAndBand(t *testing.T) {
	cases := map[int]int{-40: 100, -50: 100, -60: 80, -75: 50, -90: 20, -100: 0, -110: 0, 0: 0}
	for dbm, want := range cases {
		if got := darwinSignalPct(dbm); got != want {
			t.Errorf("%d dBm: got %d want %d", dbm, got, want)
		}
	}
	if darwinBand("2") != "2.4 GHz" || darwinBand("5") != "5 GHz" || darwinBand("6") != "6 GHz" || darwinBand("") != "" {
		t.Fatal("band")
	}
}

// ND-171: history is reported as not available, in plain words.
func TestDarwinEventsUnsupported(t *testing.T) {
	var err error = macHistoryError{}
	if !errors.Is(err, ErrUnsupported) {
		t.Fatal("should count as unsupported")
	}
	if len(strings.Fields(err.Error())) > 20 {
		t.Fatalf("keep it short and plain: %q", err.Error())
	}
}
