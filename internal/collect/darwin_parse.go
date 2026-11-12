package collect

// Mac reader, the part that has no build tag: the list of commands the Mac
// reader may run, the one gate every command goes through, and pure functions
// that turn the text of those commands into typed data. Keeping this part
// free of a build tag lets the tests run on every system (ND-172).

import (
	"context"
	"fmt"
	"math/bits"
	"net"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// darwinCommands lists EVERY command line the Mac reader may run. A word in
// braces stands for a value checked by darwinAllowed: {iface} is an adapter
// name such as en0, {ip} is an IP address, {n} is a small count.
// All of them only read (ND-01, ND-163).
var darwinCommands = [][]string{
	{"route", "-n", "get", "default"},
	{"ifconfig", "{iface}"},
	{"networksetup", "-listallhardwareports"},
	{"netstat", "-ib"},
	{"arp", "-an"},
	{"scutil", "--dns"},
	{"scutil", "--proxy"},
	{"scutil", "--nc", "list"},
	{"system_profiler", "SPAirPortDataType"},
	{"lsof", "-nP", "-iTCP", "-sTCP:ESTABLISHED"},
	{"ping", "-c", "1", "-W", "300", "{ip}"},
	// The next line is run by doPing (see pingCommand), not by darwinRun.
	{"ping", "-c", "{n}", "-W", "1000", "{ip}"},
}

var (
	ifaceRe = regexp.MustCompile(`^[A-Za-z]+[0-9]+$`)
	countRe = regexp.MustCompile(`^[1-9][0-9]?$`)
)

func darwinArgOK(want, got string) bool {
	switch want {
	case "{iface}":
		return ifaceRe.MatchString(got)
	case "{ip}":
		return net.ParseIP(got) != nil
	case "{n}":
		return countRe.MatchString(got)
	}
	return want == got
}

// darwinAllowed says whether a command line is on the list.
func darwinAllowed(name string, args []string) bool {
	for _, c := range darwinCommands {
		if c[0] != name || len(c)-1 != len(args) {
			continue
		}
		ok := true
		for i, a := range args {
			if !darwinArgOK(c[i+1], a) {
				ok = false
				break
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// darwinRun runs a listed read-only command and returns what it printed.
// It refuses anything that is not on the list. Error output is dropped, so
// warnings do not mix into the text we read.
func darwinRun(ctx context.Context, name string, args ...string) (string, error) {
	if !darwinAllowed(name, args) {
		return "", fmt.Errorf("refused: %s is not on the list of read-only commands", name)
	}
	cmd := exec.CommandContext(ctx, name, args...)
	hide(cmd)
	out, err := cmd.Output()
	return string(out), err
}

// ---- route and ifconfig -------------------------------------------------

// darwinParseRoute reads `route -n get default`.
func darwinParseRoute(out string) (iface, gateway string) {
	for _, l := range lines(out) {
		k, v, ok := strings.Cut(strings.TrimSpace(l), ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "interface":
			iface = v
		case "gateway":
			if net.ParseIP(v) != nil {
				gateway = v
			}
		}
	}
	return
}

// darwinIfconfig is what we read from `ifconfig IFACE`.
type darwinIfconfig struct {
	Up     bool
	Mbps   int
	Duplex string // "full", "half" or ""
	IP     string
	Prefix int
}

var (
	mediaSpeedRe  = regexp.MustCompile(`(?i)(\d+)(g?)base`)
	mediaDuplexRe = regexp.MustCompile(`(?i)(full|half)-duplex`)
	inetRe        = regexp.MustCompile(`^inet (\d+\.\d+\.\d+\.\d+)(?: --> \S+)? netmask (\S+)`)
)

// darwinParseIfconfig reads `ifconfig IFACE`.
func darwinParseIfconfig(out string) darwinIfconfig {
	var r darwinIfconfig
	haveStatus, flagsUp := false, false
	for _, raw := range lines(out) {
		l := strings.TrimSpace(raw)
		switch {
		case strings.Contains(l, "flags=") && strings.Contains(l, "<"):
			f := l[strings.Index(l, "<"):]
			flagsUp = strings.Contains(f, "UP") && strings.Contains(f, "RUNNING")
		case strings.HasPrefix(l, "status:"):
			haveStatus = true
			r.Up = strings.TrimSpace(strings.TrimPrefix(l, "status:")) == "active"
		case strings.HasPrefix(l, "media:"):
			media := strings.TrimPrefix(l, "media:")
			if m := mediaSpeedRe.FindStringSubmatch(media); m != nil {
				n, _ := strconv.Atoi(m[1])
				if m[2] != "" {
					n *= 1000
				}
				r.Mbps = n
			}
			if m := mediaDuplexRe.FindStringSubmatch(media); m != nil {
				r.Duplex = strings.ToLower(m[1])
			}
		case r.IP == "":
			if m := inetRe.FindStringSubmatch(l); m != nil && !strings.HasPrefix(m[1], "127.") {
				r.IP, r.Prefix = m[1], darwinPrefix(m[2])
			}
		}
	}
	if !haveStatus {
		r.Up = flagsUp
	}
	return r
}

// darwinPrefix turns 0xffffff00 (or 255.255.255.0) into 24.
func darwinPrefix(mask string) int {
	if strings.HasPrefix(mask, "0x") {
		v, err := strconv.ParseUint(mask[2:], 16, 32)
		if err != nil {
			return 0
		}
		return bits.OnesCount32(uint32(v))
	}
	if ip := net.ParseIP(mask).To4(); ip != nil {
		ones, _ := net.IPMask(ip).Size()
		return ones
	}
	return 0
}

// darwinPort is one line of the hardware port list.
type darwinPort struct {
	Name string
	Kind core.Kind
}

// darwinParsePorts reads `networksetup -listallhardwareports` and maps the
// device name (en0) to its port name ("Wi-Fi") and kind.
func darwinParsePorts(out string) map[string]darwinPort {
	res := map[string]darwinPort{}
	port := ""
	for _, l := range lines(out) {
		k, v, ok := strings.Cut(strings.TrimSpace(l), ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "Hardware Port":
			port = v
		case "Device":
			if v != "" && port != "" {
				res[v] = darwinPort{Name: port, Kind: darwinKind(port)}
			}
		}
	}
	return res
}

func darwinKind(port string) core.Kind {
	p := strings.ToLower(port)
	switch {
	case p == "wi-fi" || strings.Contains(p, "airport") || strings.Contains(p, "wi-fi"):
		return core.WiFi
	case strings.Contains(p, "ethernet") || strings.Contains(p, "lan"):
		return core.Wired
	}
	return core.Unknown
}

// ---- netstat -ib ---------------------------------------------------------

// darwinCounters are the totals for one adapter.
type darwinCounters struct {
	InBytes, OutBytes, InErrs, OutErrs int64
}

// darwinParseNetstat reads `netstat -ib`, one entry per adapter (its "Link"
// row). Columns are found from the right-hand end because the address column
// can be empty.
func darwinParseNetstat(out string) map[string]darwinCounters {
	res := map[string]darwinCounters{}
	var hdr []string
	for _, l := range lines(out) {
		f := strings.Fields(l)
		if len(f) == 0 {
			continue
		}
		if f[0] == "Name" {
			hdr = f
			continue
		}
		if hdr == nil || len(f) < 5 || !strings.HasPrefix(f[2], "<Link#") {
			continue
		}
		at := func(col string) int64 {
			for i, h := range hdr {
				if h == col {
					j := len(f) - (len(hdr) - i)
					if j >= 0 && j < len(f) {
						v, _ := strconv.ParseInt(f[j], 10, 64)
						return v
					}
				}
			}
			return 0
		}
		name := strings.TrimSuffix(f[0], "*")
		if _, seen := res[name]; seen {
			continue
		}
		res[name] = darwinCounters{
			InBytes: at("Ibytes"), OutBytes: at("Obytes"),
			InErrs: at("Ierrs"), OutErrs: at("Oerrs"),
		}
	}
	return res
}

// darwinRates turns two readings taken secs seconds apart into a sample.
// A counter that went backwards (it wrapped or was reset) counts as zero.
func darwinRates(a, b darwinCounters, secs float64, linkMbps int) Throughput {
	var t Throughput
	if secs <= 0 {
		return t
	}
	diff := func(x, y int64) float64 {
		if y < x {
			return 0
		}
		return float64(y-x) / secs
	}
	rx, tx := diff(a.InBytes, b.InBytes), diff(a.OutBytes, b.OutBytes)
	t.RxMBps, t.TxMBps = rx/1e6, tx/1e6
	if linkMbps > 0 {
		t.BusyPct = int((rx + tx) * 8 / (float64(linkMbps) * 1e6) * 100)
	}
	return t
}

// ---- lsof ----------------------------------------------------------------

var hexEscRe = regexp.MustCompile(`\\x([0-9a-fA-F]{2})`)

// darwinParseLsof reads `lsof -nP -iTCP -sTCP:ESTABLISHED` and counts open
// connections per program. Connections to this same computer are left out
// because they do not use the line. Without administrator rights lsof only
// lists the user's own programs.
func darwinParseLsof(out string) []core.Proc {
	agg := map[string]int{}
	for _, l := range lines(out) {
		if !strings.Contains(l, "(ESTABLISHED)") {
			continue
		}
		if strings.Contains(l, "->127.") || strings.Contains(l, "->[::1]") {
			continue
		}
		f := strings.Fields(l)
		if len(f) < 2 || f[0] == "COMMAND" {
			continue
		}
		name := hexEscRe.ReplaceAllStringFunc(f[0], func(s string) string {
			v, _ := strconv.ParseUint(s[2:], 16, 8)
			return string(rune(v))
		})
		agg[name]++
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

// ---- arp -----------------------------------------------------------------

// darwinNeighbor is one finished entry of the neighbor table.
type darwinNeighbor struct{ IP, MAC string }

var arpRe = regexp.MustCompile(`\((\d+\.\d+\.\d+\.\d+)\) at (\S+) on (\S+)`)

// darwinParseARP reads `arp -an`. It drops entries that are "(incomplete)",
// multicast and broadcast addresses. The Mac prints each byte of the hardware
// address without a leading zero (a:b:c:1:2:3), so each byte is padded to two
// digits. If iface is not empty only entries on that adapter are kept.
func darwinParseARP(out, iface string) []darwinNeighbor {
	var res []darwinNeighbor
	seen := map[string]bool{}
	for _, l := range lines(out) {
		m := arpRe.FindStringSubmatch(l)
		if m == nil || (iface != "" && m[3] != iface) {
			continue
		}
		mac, ok := darwinPadMAC(m[2])
		if !ok || seen[m[1]] || strings.HasSuffix(m[1], ".255") {
			continue
		}
		b, _ := strconv.ParseUint(mac[:2], 16, 8)
		if b&1 == 1 { // multicast and broadcast
			continue
		}
		seen[m[1]] = true
		res = append(res, darwinNeighbor{IP: m[1], MAC: core.NormalizeMAC(mac)})
	}
	sort.Slice(res, func(i, j int) bool { return darwinIPKey(res[i].IP) < darwinIPKey(res[j].IP) })
	return res
}

// darwinPadMAC turns a:b:c:1:2:3 into 0a:0b:0c:01:02:03.
func darwinPadMAC(s string) (string, bool) {
	parts := strings.Split(s, ":")
	if len(parts) != 6 {
		return "", false
	}
	for i, p := range parts {
		if len(p) == 0 || len(p) > 2 {
			return "", false
		}
		if _, err := strconv.ParseUint(p, 16, 8); err != nil {
			return "", false
		}
		if len(p) == 1 {
			parts[i] = "0" + p
		}
	}
	return strings.Join(parts, ":"), true
}

func darwinIPKey(s string) uint32 {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return 0
	}
	return uint32(ip[0])<<24 | uint32(ip[1])<<16 | uint32(ip[2])<<8 | uint32(ip[3])
}

// ---- scutil --------------------------------------------------------------

var nsRe = regexp.MustCompile(`^nameserver\[\d+\]\s*:\s*(\S+)`)

// darwinParseDNS reads `scutil --dns`. It takes the name servers of the first
// ordinary resolver in the main list. A resolver that is only for one domain
// (a "domain :" line) or only for local names (mDNS) is skipped. If the main
// list has none, the list for scoped queries is tried.
func darwinParseDNS(out string) []string {
	var main, scoped [][]string
	var cur []string
	skip := false
	inScoped := false
	flush := func() {
		if len(cur) > 0 && !skip {
			if inScoped {
				scoped = append(scoped, cur)
			} else {
				main = append(main, cur)
			}
		}
		cur, skip = nil, false
	}
	for _, raw := range lines(out) {
		l := strings.TrimSpace(raw)
		switch {
		case strings.HasPrefix(l, "DNS configuration"):
			flush()
			inScoped = strings.Contains(l, "scoped")
		case strings.HasPrefix(l, "resolver #"):
			flush()
		case strings.HasPrefix(l, "domain") && strings.Contains(l, ":"):
			skip = true
		case strings.HasPrefix(l, "options") && strings.Contains(l, "mdns"):
			skip = true
		default:
			if m := nsRe.FindStringSubmatch(l); m != nil {
				cur = append(cur, m[1])
			}
		}
	}
	flush()
	if len(main) > 0 {
		return main[0]
	}
	if len(scoped) > 0 {
		return scoped[0]
	}
	return nil
}

var proxyKeys = map[string]bool{
	"HTTPEnable": true, "HTTPSEnable": true, "SOCKSEnable": true, "FTPEnable": true,
	"RTSPEnable": true, "GopherEnable": true, "ProxyAutoConfigEnable": true,
}

// darwinParseProxy reads `scutil --proxy`. It is true when a web, secure web,
// SOCKS, FTP, streaming or script (PAC) proxy is switched on. Automatic
// discovery alone does not count, because it sets no proxy by itself.
func darwinParseProxy(out string) bool {
	for _, l := range lines(out) {
		k, v, ok := strings.Cut(strings.TrimSpace(l), ":")
		if ok && proxyKeys[strings.TrimSpace(k)] && strings.TrimSpace(v) == "1" {
			return true
		}
	}
	return false
}

// darwinParseVPN reads `scutil --nc list`. It is true when any VPN service
// shows (Connected).
func darwinParseVPN(out string) bool {
	for _, l := range lines(out) {
		if strings.Contains(l, "(Connected)") {
			return true
		}
	}
	return false
}

// ---- system_profiler -----------------------------------------------------

// darwinWiFi is what we read from `system_profiler SPAirPortDataType`.
type darwinWiFi struct {
	WiFiInfo
	Connected bool
	TxMbps    int
}

var (
	spChannelRe = regexp.MustCompile(`^Channel:\s*(\d+)(?:\s*\((\d+(?:\.\d+)?)\s*GHz)?`)
	spSignalRe  = regexp.MustCompile(`^Signal / Noise:\s*(-?\d+)\s*dBm`)
	spRateRe    = regexp.MustCompile(`^Transmit Rate:\s*(\d+)`)
)

// darwinSignalPct turns a signal strength in dBm into a percent:
// percent = 2 x (dBm + 100), kept between 0 and 100. So -50 dBm or better is
// 100, -60 is 80, -75 is 50 and -100 or worse is 0. It is the same simple
// rule many tools use. A reading of 0 dBm means "not read" and gives 0.
func darwinSignalPct(dbm int) int {
	if dbm == 0 {
		return 0
	}
	p := 2 * (dbm + 100)
	if p < 0 {
		return 0
	}
	if p > 100 {
		return 100
	}
	return p
}

func darwinBand(ghz string) string {
	switch ghz {
	case "2", "2.4":
		return "2.4 GHz"
	case "5":
		return "5 GHz"
	case "6":
		return "6 GHz"
	}
	return ""
}

// darwinParseWiFi reads `system_profiler SPAirPortDataType`. A field that is
// not in the text stays empty or zero; nothing is guessed. The band comes
// only from the "(5GHz, 80MHz)" part of the channel line. The network's own
// entry is left out of the nearby list if it appears there.
func darwinParseWiFi(out string) darwinWiFi {
	var r darwinWiFi
	const (
		none = iota
		current
		other
	)
	state := none
	ssid := ""
	seenSSID, haveChannel, haveSignal := false, false, false
	skippedSelf, skipNext := false, false
	for _, raw := range lines(out) {
		l := strings.TrimSpace(raw)
		switch {
		case l == "Current Network Information:":
			state = current
			seenSSID = false
			continue
		case l == "Other Local Wi-Fi Networks:":
			state = other
			continue
		}
		switch state {
		case current:
			if !seenSSID {
				if strings.HasSuffix(l, ":") {
					ssid = strings.TrimSuffix(l, ":")
					seenSSID = true
					r.Connected = true
				}
				continue
			}
			if m := spChannelRe.FindStringSubmatch(l); m != nil && !haveChannel {
				haveChannel = true
				r.Channel, _ = strconv.Atoi(m[1])
				r.Band = darwinBand(m[2])
			} else if m := spSignalRe.FindStringSubmatch(l); m != nil && !haveSignal {
				haveSignal = true
				dbm, _ := strconv.Atoi(m[1])
				r.SignalPct = darwinSignalPct(dbm)
			} else if m := spRateRe.FindStringSubmatch(l); m != nil && r.TxMbps == 0 {
				r.TxMbps, _ = strconv.Atoi(m[1])
			}
		case other:
			if strings.HasSuffix(l, ":") && !strings.Contains(l, ": ") {
				name := strings.TrimSuffix(l, ":")
				skipNext = !skippedSelf && ssid != "" && ssid != "<redacted>" && name == ssid
				continue
			}
			if m := spChannelRe.FindStringSubmatch(l); m != nil {
				c, _ := strconv.Atoi(m[1])
				if skipNext {
					skipNext, skippedSelf = false, true
					continue
				}
				r.OtherChannels = append(r.OtherChannels, c)
			}
		}
	}
	return r
}

// macHistoryError is plain text for the user and still counts as
// ErrUnsupported for the code.
type macHistoryError struct{}

func (macHistoryError) Error() string {
	return "this Mac keeps too little network history that this tool can read safely"
}
func (macHistoryError) Is(target error) bool { return target == ErrUnsupported }
