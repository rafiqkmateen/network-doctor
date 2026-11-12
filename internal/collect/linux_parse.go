package collect

// This file has no build tag on purpose. The Linux reader turns the text of
// system commands and files into typed facts here, so the tests can check the
// reading on any system, and it holds the one gate every Linux command and
// file read must pass.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// linuxCommands lists EVERY command line the Linux reader may run. Words in
// braces are filled in: {dev} an adapter name, {n} a small number, {ip} an
// address, {since} a "--since=-7d" style time. linuxRun refuses anything else.
var linuxCommands = [][]string{
	{"ip", "-j", "route", "show"},
	{"ip", "-j", "addr", "show"},
	{"ethtool", "{dev}"},
	{"ethtool", "-i", "{dev}"},
	{"iw", "dev", "{dev}", "link"},
	{"iw", "dev", "{dev}", "get", "power_save"},
	{"iw", "dev", "{dev}", "scan", "dump"},
	{"nmcli", "-t", "-f", "IN-USE,CHAN,FREQ,SIGNAL", "device", "wifi", "list", "--rescan", "no"},
	{"nmcli", "-t", "-f", "GENERAL.STATE", "device", "show", "{dev}"},
	{"resolvectl", "status"},
	{"journalctl", "--no-pager", "-k", "-o", "short-iso", "{since}"},
	{"journalctl", "--no-pager", "-u", "NetworkManager", "-o", "short-iso", "{since}"},
	{"gsettings", "get", "org.gnome.system.proxy", "mode"},
	{"ss", "-H", "-tnp", "state", "established"},
	{"ping", "-c", "{n}", "-W", "1", "{ip}"},
}

// linuxFiles lists EVERY file the Linux reader may read. {dev} is an adapter name.
var linuxFiles = []string{
	"/proc/net/dev",
	"/proc/net/arp",
	"/etc/resolv.conf",
	"/run/systemd/resolve/resolv.conf",
	"/sys/class/net/{dev}/speed",
	"/sys/class/net/{dev}/duplex",
	"/sys/class/net/{dev}/operstate",
	"/sys/class/net/{dev}/statistics/rx_errors",
	"/sys/class/net/{dev}/statistics/tx_errors",
	"/sys/class/net/{dev}/wireless",
	"/sys/class/net/{dev}/phy80211",
	"/sys/class/net/{dev}/device",
}

var (
	linuxIfaceRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_.:@-]{0,14}$`)
	linuxNumRe   = regexp.MustCompile(`^[0-9]{1,3}$`)
	linuxSinceRe = regexp.MustCompile(`^--since=-[0-9]{1,3}d$`)
)

func linuxWordOK(pattern, got string) bool {
	switch pattern {
	case "{dev}":
		return linuxIfaceRe.MatchString(got)
	case "{n}":
		return linuxNumRe.MatchString(got)
	case "{ip}":
		return net.ParseIP(got) != nil
	case "{since}":
		return linuxSinceRe.MatchString(got)
	}
	return pattern == got
}

func linuxMatch(pattern, got []string) bool {
	if len(pattern) != len(got) {
		return false
	}
	for i := range pattern {
		if !linuxWordOK(pattern[i], got[i]) {
			return false
		}
	}
	return true
}

// linuxAllowed says whether a command line is on the list.
func linuxAllowed(argv []string) bool {
	for _, p := range linuxCommands {
		if linuxMatch(p, argv) {
			return true
		}
	}
	return false
}

func linuxFileAllowed(path string) bool {
	if !strings.HasPrefix(path, "/") || strings.Contains(path, "//") {
		return false
	}
	got := strings.Split(path[1:], "/")
	for _, p := range linuxFiles {
		if linuxMatch(strings.Split(p[1:], "/"), got) {
			return true
		}
	}
	return false
}

// linuxFindTool finds a program. Some systems keep ethtool and iw in sbin
// folders that an ordinary user's search path leaves out.
func linuxFindTool(name string) string {
	if p, err := exec.LookPath(name); err == nil {
		return p
	}
	for _, d := range []string{"/usr/sbin", "/sbin", "/usr/bin", "/bin"} {
		p := filepath.Join(d, name)
		if st, err := os.Stat(p); err == nil && !st.IsDir() {
			return p
		}
	}
	return name
}

// linuxRun runs one listed read-only command. Anything not on the list is
// refused. It returns what the command printed; when the command failed or
// printed nothing, what it said on the error side is added.
func linuxRun(ctx context.Context, name string, args ...string) (string, error) {
	argv := append([]string{name}, args...)
	if !linuxAllowed(argv) {
		return "", fmt.Errorf("the command %q is not on the read-only list", name)
	}
	ctx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, linuxFindTool(name), args...)
	hide(cmd)
	var so, se bytes.Buffer
	cmd.Stdout, cmd.Stderr = &so, &se
	err := cmd.Run()
	out := so.String()
	if err != nil || out == "" {
		out += se.String()
	}
	return out, err
}

// linuxReadFile reads one listed file.
func linuxReadFile(path string) (string, error) {
	if !linuxFileAllowed(path) {
		return "", fmt.Errorf("the file %q is not on the read list", path)
	}
	b, err := os.ReadFile(path)
	return string(b), err
}

// linuxExists says whether a listed path is there.
func linuxExists(path string) bool {
	if !linuxFileAllowed(path) {
		return false
	}
	_, err := os.Stat(path)
	return err == nil
}

// ParseLinuxInt reads a number from a /sys file. "-1" means the kernel does
// not know (a Wi-Fi link, or a cable that is not linked).
func ParseLinuxInt(s string) (int64, bool) {
	v, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	return v, err == nil
}

// linuxIsTunnel names the adapters VPN software makes.
func linuxIsTunnel(name string) bool {
	for _, p := range []string{"tun", "tap", "wg", "ppp", "tailscale", "ipsec", "vti", "zt", "nordlynx", "proton", "mullvad", "utun"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// linuxPhysicalName guesses, from the name alone, that an adapter is a real one.
func linuxPhysicalName(name string) bool {
	for _, p := range []string{"en", "eth", "em", "wl", "ww"} {
		if strings.HasPrefix(name, p) {
			return true
		}
	}
	return false
}

// LinuxRoute is the default route the system uses.
type LinuxRoute struct {
	Dev     string
	Gateway string
	Src     string
}

// ParseLinuxRoutes reads `ip -j route show` and returns the default route
// with the lowest metric. Routes through VPN adapters count only when there
// is no other.
func ParseLinuxRoutes(out string) (LinuxRoute, error) {
	var raw []struct {
		Dst     string `json:"dst"`
		Gateway string `json:"gateway"`
		Dev     string `json:"dev"`
		Prefsrc string `json:"prefsrc"`
		Metric  int    `json:"metric"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &raw); err != nil {
		return LinuxRoute{}, errors.New("the route list could not be read")
	}
	var best, bestTun *LinuxRoute
	var bm, bmt int
	for i := range raw {
		r := &raw[i]
		if r.Dst != "default" || r.Dev == "" {
			continue
		}
		lr := LinuxRoute{Dev: r.Dev, Gateway: r.Gateway, Src: r.Prefsrc}
		if linuxIsTunnel(r.Dev) {
			if bestTun == nil || r.Metric < bmt {
				bestTun, bmt = &lr, r.Metric
			}
			continue
		}
		if best == nil || r.Metric < bm {
			best, bm = &lr, r.Metric
		}
	}
	if best != nil {
		return *best, nil
	}
	if bestTun != nil {
		return *bestTun, nil
	}
	return LinuxRoute{}, errors.New("no network route was found")
}

// LinuxAddr is one adapter from `ip -j addr show`.
type LinuxAddr struct {
	Name   string
	Up     bool
	IP     string
	Prefix int
}

// ParseLinuxAddrs reads `ip -j addr show`.
func ParseLinuxAddrs(out string) ([]LinuxAddr, error) {
	var raw []struct {
		Name      string   `json:"ifname"`
		Flags     []string `json:"flags"`
		Operstate string   `json:"operstate"`
		Addrs     []struct {
			Family string `json:"family"`
			Local  string `json:"local"`
			Prefix int    `json:"prefixlen"`
			Scope  string `json:"scope"`
		} `json:"addr_info"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(out)), &raw); err != nil {
		return nil, errors.New("the adapter list could not be read")
	}
	var res []LinuxAddr
	for _, r := range raw {
		a := LinuxAddr{Name: r.Name}
		var hasUp, hasLower bool
		for _, f := range r.Flags {
			hasUp = hasUp || f == "UP"
			hasLower = hasLower || f == "LOWER_UP"
		}
		a.Up = strings.EqualFold(r.Operstate, "up") || (hasUp && hasLower)
		for _, ad := range r.Addrs {
			if ad.Family != "inet" || ad.Local == "" {
				continue
			}
			if a.IP == "" || (ad.Scope == "global" && a.Prefix == 0) {
				a.IP, a.Prefix = ad.Local, ad.Prefix
			}
		}
		res = append(res, a)
	}
	return res, nil
}

// LinuxFindAddr returns the adapter with that name.
func LinuxFindAddr(addrs []LinuxAddr, name string) (LinuxAddr, bool) {
	for _, a := range addrs {
		if a.Name == name {
			return a, true
		}
	}
	return LinuxAddr{}, false
}

// LinuxPickPhysical chooses a real adapter when there is no default route
// (for example, with the cable out). It prefers one that is up with an address.
func LinuxPickPhysical(addrs []LinuxAddr) (LinuxAddr, bool) {
	var firstAny, firstUp LinuxAddr
	var haveAny, haveUp bool
	for _, a := range addrs {
		if !linuxPhysicalName(a.Name) || linuxIsTunnel(a.Name) {
			continue
		}
		if a.Up && a.IP != "" {
			return a, true
		}
		if a.Up && !haveUp {
			firstUp, haveUp = a, true
		}
		if !haveAny {
			firstAny, haveAny = a, true
		}
	}
	if haveUp {
		return firstUp, true
	}
	return firstAny, haveAny
}

// LinuxVPNActive is true when a VPN adapter is up.
func LinuxVPNActive(addrs []LinuxAddr) bool {
	for _, a := range addrs {
		if a.Up && linuxIsTunnel(a.Name) {
			return true
		}
	}
	return false
}

var (
	ethSpeedRe  = regexp.MustCompile(`(?m)^\s*Speed:\s*(\d+)\s*Mb/s`)
	ethDuplexRe = regexp.MustCompile(`(?im)^\s*Duplex:\s*(Full|Half)\b`)
)

// ParseLinuxEthtool reads plain `ethtool dev`. A speed or duplex shown as
// "Unknown!" comes back as 0 and "".
func ParseLinuxEthtool(out string) (mbps int, duplex string) {
	if m := ethSpeedRe.FindStringSubmatch(out); m != nil {
		mbps, _ = strconv.Atoi(m[1])
	}
	if m := ethDuplexRe.FindStringSubmatch(out); m != nil {
		duplex = strings.ToLower(m[1])
	}
	return mbps, duplex
}

// LinuxDuplex turns the text of /sys/class/net/dev/duplex into "full", "half" or "".
func LinuxDuplex(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "full":
		return "full"
	case "half":
		return "half"
	}
	return ""
}

// ParseLinuxEthtoolDriver reads `ethtool -i dev`.
func ParseLinuxEthtoolDriver(out string) (driver, version string) {
	for _, l := range lines(out) {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(k) {
		case "driver":
			driver = strings.TrimSpace(v)
		case "version":
			version = strings.TrimSpace(v)
		}
	}
	return driver, version
}

var (
	iwFreqRe    = regexp.MustCompile(`(?m)^\s*freq:\s*(\d+)`)
	iwSignalRe  = regexp.MustCompile(`(?m)^\s*signal:\s*(-?\d+)`)
	iwTxRateRe  = regexp.MustCompile(`(?m)^\s*tx bitrate:\s*([\d.]+)\s*MBit/s`)
	iwPowerRe   = regexp.MustCompile(`(?im)^\s*Power save:\s*(on|off)\b`)
	iwAssocRe   = regexp.MustCompile(`^BSS\s+\S+.*--\s*associated`)
	iwBSSStarts = regexp.MustCompile(`^BSS\s+[0-9a-fA-F:]{17}`)
)

// LinuxChannel turns a Wi-Fi frequency in MHz into a channel number and band.
func LinuxChannel(freq int) (channel int, band string) {
	switch {
	case freq == 2484:
		return 14, "2.4 GHz"
	case freq >= 2412 && freq <= 2472:
		return (freq - 2407) / 5, "2.4 GHz"
	case freq >= 5160 && freq <= 5885:
		return (freq - 5000) / 5, "5 GHz"
	case freq >= 5955 && freq <= 7115:
		return (freq - 5950) / 5, "6 GHz"
	}
	return 0, ""
}

// LinuxSignalPct turns a signal in dBm into a percent: -50 or better is 100,
// -100 or worse is 0.
func LinuxSignalPct(dbm int) int {
	p := 2 * (dbm + 100)
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

// ParseLinuxIwLink reads `iw dev X link`. connected is false for "Not connected."
func ParseLinuxIwLink(out string) (info WiFiInfo, mbps int, connected bool) {
	if !strings.Contains(out, "Connected to") {
		return info, 0, false
	}
	if m := iwFreqRe.FindStringSubmatch(out); m != nil {
		f, _ := strconv.Atoi(m[1])
		info.Channel, info.Band = LinuxChannel(f)
	}
	if m := iwSignalRe.FindStringSubmatch(out); m != nil {
		d, _ := strconv.Atoi(m[1])
		info.SignalPct = LinuxSignalPct(d)
	}
	if m := iwTxRateRe.FindStringSubmatch(out); m != nil {
		f, _ := strconv.ParseFloat(m[1], 64)
		mbps = int(f + 0.5)
	}
	return info, mbps, true
}

// ParseLinuxIwScanDump reads `iw dev X scan dump` (the scan the system already
// has) and returns the channels of the other networks. The entry marked
// "associated" is the user's own and is left out.
func ParseLinuxIwScanDump(out string) []int {
	var chans []int
	inBlock, own := false, false
	flush := func(freq int) {
		if inBlock && !own && freq > 0 {
			if c, _ := LinuxChannel(freq); c > 0 {
				chans = append(chans, c)
			}
		}
	}
	freq := 0
	for _, l := range lines(out) {
		if iwBSSStarts.MatchString(l) {
			flush(freq)
			inBlock, own, freq = true, iwAssocRe.MatchString(l), 0
			continue
		}
		if m := iwFreqRe.FindStringSubmatch(l); m != nil && inBlock && freq == 0 {
			freq, _ = strconv.Atoi(m[1])
		}
	}
	flush(freq)
	return chans
}

// ParseLinuxPowerSave reads `iw dev X get power_save`.
func ParseLinuxPowerSave(out string) (known, on bool) {
	m := iwPowerRe.FindStringSubmatch(out)
	if m == nil {
		return false, false
	}
	return true, strings.EqualFold(m[1], "on")
}

// LinuxWifiRow is one network from `nmcli device wifi list`.
type LinuxWifiRow struct {
	InUse     bool
	Channel   int
	Freq      int
	SignalPct int
}

// ParseLinuxNmcliWifi reads `nmcli -t -f IN-USE,CHAN,FREQ,SIGNAL device wifi list`.
func ParseLinuxNmcliWifi(out string) []LinuxWifiRow {
	var rows []LinuxWifiRow
	for _, l := range lines(out) {
		f := strings.SplitN(l, ":", 4)
		if len(f) != 4 {
			continue
		}
		ch, err := strconv.Atoi(strings.TrimSpace(f[1]))
		if err != nil {
			continue
		}
		fr := 0
		if w := strings.Fields(f[2]); len(w) > 0 {
			fr, _ = strconv.Atoi(w[0])
		}
		sg, _ := strconv.Atoi(strings.TrimSpace(f[3]))
		rows = append(rows, LinuxWifiRow{InUse: strings.TrimSpace(f[0]) == "*", Channel: ch, Freq: fr, SignalPct: sg})
	}
	return rows
}

// LinuxCounters are the bytes an adapter has received and sent.
type LinuxCounters struct{ Rx, Tx uint64 }

// ParseLinuxNetDev reads /proc/net/dev.
func ParseLinuxNetDev(text string) map[string]LinuxCounters {
	res := map[string]LinuxCounters{}
	for _, l := range lines(text) {
		i := strings.Index(l, ":")
		if i < 0 {
			continue
		}
		f := strings.Fields(l[i+1:])
		if len(f) < 9 {
			continue
		}
		rx, e1 := strconv.ParseUint(f[0], 10, 64)
		tx, e2 := strconv.ParseUint(f[8], 10, 64)
		if e1 != nil || e2 != nil {
			continue
		}
		res[strings.TrimSpace(l[:i])] = LinuxCounters{Rx: rx, Tx: tx}
	}
	return res
}

var ssUserRe = regexp.MustCompile(`users:\(\("([^"]+)"`)

// ParseLinuxSS reads `ss -H -tnp state established` and counts connections
// per program. A connection with no program name (it belongs to another
// user, and showing it needs administrator rights) is left out.
func ParseLinuxSS(out string) []core.Proc {
	agg := map[string]int{}
	for _, l := range lines(out) {
		if m := ssUserRe.FindStringSubmatch(l); m != nil {
			agg[m[1]]++
		}
	}
	var procs []core.Proc
	for n, c := range agg {
		procs = append(procs, core.Proc{Name: n, Conns: c})
	}
	sort.Slice(procs, func(i, j int) bool {
		if procs[i].Conns != procs[j].Conns {
			return procs[i].Conns > procs[j].Conns
		}
		return procs[i].Name < procs[j].Name
	})
	return procs
}

// LinuxNeighbor is one device the computer has talked to lately.
type LinuxNeighbor struct{ IP, MAC string }

func linuxIPKey(s string) int {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return 0
	}
	return int(ip[0])<<24 | int(ip[1])<<16 | int(ip[2])<<8 | int(ip[3])
}

// ParseLinuxArp reads /proc/net/arp. It keeps finished entries on the given
// adapter (any adapter when dev is empty) inside the prefix, such as "192.168.1.".
// Unfinished entries (all-zero address), broadcast and multicast are dropped.
func ParseLinuxArp(text, dev, prefix3 string) []LinuxNeighbor {
	var res []LinuxNeighbor
	for _, l := range lines(text) {
		f := strings.Fields(l)
		if len(f) < 6 || f[0] == "IP" {
			continue
		}
		if dev != "" && f[5] != dev {
			continue
		}
		flags, err := strconv.ParseInt(f[2], 0, 64)
		if err != nil || flags&0x2 == 0 {
			continue
		}
		hw, err := net.ParseMAC(f[3])
		if err != nil || len(hw) != 6 {
			continue
		}
		if hw.String() == "00:00:00:00:00:00" || hw[0]&1 == 1 {
			continue
		}
		ip := net.ParseIP(f[0]).To4()
		if ip == nil || !strings.HasPrefix(f[0], prefix3) || ip[3] == 255 || ip[3] == 0 {
			continue
		}
		res = append(res, LinuxNeighbor{IP: f[0], MAC: core.NormalizeMAC(hw.String())})
	}
	sort.Slice(res, func(i, j int) bool { return linuxIPKey(res[i].IP) < linuxIPKey(res[j].IP) })
	return res
}

var resolvLinkRe = regexp.MustCompile(`^Link\s+\d+\s+\(([^)]+)\)`)

func linuxCleanAddr(s string) string {
	if i := strings.IndexAny(s, "#%"); i >= 0 {
		s = s[:i]
	}
	if net.ParseIP(s) == nil {
		return ""
	}
	return s
}

func linuxAddrList(s string) ([]string, bool) {
	f := strings.Fields(s)
	if len(f) == 0 {
		return nil, false
	}
	var res []string
	for _, w := range f {
		a := linuxCleanAddr(w)
		if a == "" {
			return nil, false
		}
		res = append(res, a)
	}
	return res, true
}

// ParseLinuxResolvectl reads `resolvectl status` and returns the DNS servers
// the system really asks: those of the adapter dev, else the global ones, else
// every adapter's.
func ParseLinuxResolvectl(out, dev string) []string {
	byScope := map[string][]string{}
	var order []string
	scope, inList := "", false
	add := func(addrs []string) {
		if _, seen := byScope[scope]; !seen {
			order = append(order, scope)
		}
		byScope[scope] = append(byScope[scope], addrs...)
	}
	for _, l := range lines(out) {
		t := strings.TrimSpace(l)
		if t == "" {
			inList = false
			continue
		}
		if t == "Global" {
			scope, inList = "", false
			continue
		}
		if m := resolvLinkRe.FindStringSubmatch(t); m != nil {
			scope, inList = m[1], false
			continue
		}
		if inList {
			if addrs, ok := linuxAddrList(t); ok {
				add(addrs)
				continue
			}
			inList = false
		}
		if strings.HasPrefix(t, "DNS Servers:") {
			inList = true
			if addrs, ok := linuxAddrList(strings.TrimPrefix(t, "DNS Servers:")); ok {
				add(addrs)
			}
		}
	}
	if v := byScope[dev]; dev != "" && len(v) > 0 {
		return linuxUnique(v)
	}
	if v := byScope[""]; len(v) > 0 {
		return linuxUnique(v)
	}
	var all []string
	for _, s := range order {
		all = append(all, byScope[s]...)
	}
	return linuxUnique(all)
}

func linuxUnique(in []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, s := range in {
		if !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}

// ParseLinuxResolvConf reads resolv.conf. A lone 127.0.0.53 means a local
// stub passes the questions on, and resolvectl has the real servers.
func ParseLinuxResolvConf(text string) []string {
	var res []string
	for _, l := range lines(text) {
		t := strings.TrimSpace(l)
		if strings.HasPrefix(t, "#") || strings.HasPrefix(t, ";") {
			continue
		}
		f := strings.Fields(t)
		if len(f) >= 2 && f[0] == "nameserver" {
			if a := linuxCleanAddr(f[1]); a != "" {
				res = append(res, a)
			}
		}
	}
	return linuxUnique(res)
}

// LinuxIsStub says the only server is the local systemd stub.
func LinuxIsStub(dns []string) bool {
	return len(dns) > 0 && len(dns) == len(linuxFilter(dns, true))
}

func linuxFilter(dns []string, stubOnly bool) []string {
	var out []string
	for _, d := range dns {
		stub := d == "127.0.0.53" || d == "127.0.0.54"
		if stub == stubOnly {
			out = append(out, d)
		}
	}
	return out
}

// ParseLinuxProxyMode reads `gsettings get org.gnome.system.proxy mode`.
func ParseLinuxProxyMode(out string) bool {
	m := strings.Trim(strings.TrimSpace(out), "'\"")
	return m == "manual" || m == "auto"
}

// LinuxProxyEnv is true when a proxy is set in the environment. get is
// os.Getenv in real use.
func LinuxProxyEnv(get func(string) string) bool {
	for _, k := range []string{"http_proxy", "https_proxy", "all_proxy", "HTTP_PROXY", "HTTPS_PROXY", "ALL_PROXY"} {
		if strings.TrimSpace(get(k)) != "" {
			return true
		}
	}
	return false
}

var nmStateRe = regexp.MustCompile(`(?m)^GENERAL\.STATE:\s*(\d+)`)

// ParseLinuxNmcliState reads `nmcli -t -f GENERAL.STATE device show dev` and
// returns plain-words problems. A failed device or one that needs a login is
// a problem; other states are normal.
func ParseLinuxNmcliState(out string) []string {
	m := nmStateRe.FindStringSubmatch(out)
	if m == nil {
		return nil
	}
	switch m[1] {
	case "120":
		return []string{"the network manager says the adapter failed to connect"}
	case "60":
		return []string{"the network needs a sign-in"}
	}
	return nil
}

var (
	nmChangeRe = regexp.MustCompile(`device \(([^)]+)\): state change: ([a-z-]+) -> ([a-z-]+)`)
	jTimeLays  = []string{"2006-01-02T15:04:05-0700", "2006-01-02T15:04:05Z07:00"}
)

func linuxParseJTime(s string) (time.Time, bool) {
	for _, lay := range jTimeLays {
		if t, err := time.Parse(lay, s); err == nil {
			return t, true
		}
	}
	return time.Time{}, false
}

func linuxHasWord(line, word string) bool {
	re := regexp.MustCompile(`(^|[^A-Za-z0-9_.-])` + regexp.QuoteMeta(word) + `([^A-Za-z0-9_.-]|$)`)
	return re.MatchString(line)
}

// linuxLineState says whether a log line is a link change for dev, and which way.
func linuxLineState(msg, dev string) (isEvent, up bool) {
	low := strings.ToLower(msg)
	if !strings.Contains(low, "state change") {
		if dev != "" && !strings.Contains(msg, dev) {
			return false, false
		}
	} else if m := nmChangeRe.FindStringSubmatch(low); m != nil {
		if dev != "" && m[1] != strings.ToLower(dev) {
			return false, false
		}
		switch {
		case m[3] == "activated":
			return true, true
		case m[2] == "activated":
			return true, false
		}
		return false, false
	}
	if dev != "" && !linuxHasWord(msg, dev) {
		return false, false
	}
	switch {
	case strings.Contains(low, "link is down"),
		strings.Contains(low, ": deauthenticating"),
		strings.Contains(low, ": deauthenticated from"),
		strings.Contains(low, ": disassociated"):
		return true, false
	case strings.Contains(low, "link is up"),
		strings.Contains(low, "link becomes ready"),
		strings.Contains(low, ": associated"):
		return true, true
	}
	return false, false
}

// ParseLinuxJournal reads `journalctl -o short-iso` text and returns the
// link up and down changes for dev. If the log shows no lines at all, it is
// not readable (usually it needs administrator rights) and that is an error.
func ParseLinuxJournal(out, dev string) ([]core.Event, error) {
	var ev []core.Event
	entries := 0
	for _, l := range lines(out) {
		sp := strings.IndexByte(l, ' ')
		if sp < 0 {
			continue
		}
		t, ok := linuxParseJTime(l[:sp])
		if !ok {
			continue
		}
		entries++
		if is, up := linuxLineState(l[sp+1:], dev); is {
			ev = append(ev, core.Event{Time: t, Up: up})
		}
	}
	if entries == 0 {
		return nil, errors.New("the system log could not be read without administrator rights")
	}
	return ev, nil
}

// LinuxMergeEvents sorts events by time and drops repeats of the same state,
// which the kernel log and the network manager often both write.
func LinuxMergeEvents(ev []core.Event) []core.Event {
	sorted := append([]core.Event(nil), ev...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Time.Before(sorted[j].Time) })
	var res []core.Event
	for _, e := range sorted {
		if n := len(res); n > 0 && res[n-1].Up == e.Up {
			continue
		}
		res = append(res, e)
	}
	return res
}
