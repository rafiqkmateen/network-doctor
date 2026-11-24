package fix

import (
	"errors"
	"fmt"
	"net"
	"sort"
	"strconv"
	"strings"
	"time"
)

// UnixSystem is a Mac or Linux computer for the fixes (ND-303 to ND-307). Every program it runs
// goes through Run and every program it needs is looked up with Look, so the tests drive it with
// pretend commands only. It has no build tag, so those tests run on any computer. Real() fills it
// in on a Mac or Linux computer.
//
// Mac: networksetup for the web proxy, the name servers and switching the network service off and
// on; ipconfig to ask for a new address. Linux: gsettings for the proxy; nmcli for the name
// servers, a new address, and switching the adapter off and on. A Mac change that needs
// administrator rights asks for them at that moment only, through the Mac's own permission box
// (ND-94); on Linux, NetworkManager asks by itself when it must.
type UnixSystem struct {
	GOOS  string
	Run   func(name string, args ...string) (string, error)
	Look  func(name string) (string, error)
	Index func(device string) int    // the system's number for an adapter, 0 when unknown
	Conds func() (Conditions, error) // reads the situation at the press; nil when it cannot
	Sleep func(time.Duration)        // waits between read-backs
	Flush func() error               // clears the saved names; nil uses the system's own tool
}

var (
	_ ExtraSystem     = UnixSystem{}
	_ ConditionSystem = UnixSystem{}
	_ ToolChecker     = UnixSystem{}
	_ AdminAsker      = UnixSystem{}
)

// mayAsk names the fixes whose tool may ask for administrator rights: on a Mac every networksetup
// or ipconfig change (dscacheutil never asks); on Linux NetworkManager's changes and resolvectl,
// which may ask through the system (gsettings is the person's own setting and never asks).
var mayAsk = map[string]map[string]bool{
	"darwin": {proxy: true, publicDNS: true, renewAddress: true, resetAdapter: true},
	"linux":  {nameCache: true, publicDNS: true, renewAddress: true, resetAdapter: true},
}

// MayAskAdmin says whether the fix may ask for administrator rights on this computer. Power saving
// is never changed here, so it never asks.
func (u UnixSystem) MayAskAdmin(name string) bool { return mayAsk[u.GOOS][name] }

// The words of a refusal on Mac and Linux (ND-307).
const (
	powerNeverReason  = "This tool never changes power saving on this computer, because it could drop the connection you are using. Nothing was changed."
	otherSystemReason = "This fix does not work on this kind of computer, so nothing was changed."
	macAutoProxyWords = "automatic setup from "
)

func missingToolReason(tool string) string {
	return "This fix needs the program " + tool + ", and this computer does not have it. Nothing was changed."
}

// fixTools names the programs each fix needs on each system.
var fixTools = map[string]map[string][]string{
	"darwin": {
		nameCache:    {"dscacheutil"},
		proxy:        {"route", "networksetup"},
		publicDNS:    {"route", "networksetup"},
		renewAddress: {"route", "networksetup", "ipconfig", "ifconfig"},
		resetAdapter: {"route", "networksetup", "ifconfig", "sh"},
	},
	"linux": {
		nameCache:    {"resolvectl"},
		proxy:        {"gsettings"},
		publicDNS:    {"ip", "nmcli"},
		renewAddress: {"ip", "nmcli"},
		resetAdapter: {"ip", "nmcli"},
	},
}

// MissingTool says why a fix cannot work here, or "" when every program it needs is there.
// Power saving is always refused (ND-307).
func (u UnixSystem) MissingTool(name string) string {
	if name == power {
		return powerNeverReason
	}
	tools, ok := fixTools[u.GOOS][name]
	if !ok {
		return otherSystemReason
	}
	for _, t := range tools {
		if _, err := u.Look(t); err != nil {
			return missingToolReason(t)
		}
	}
	return ""
}

func (u UnixSystem) mac() bool { return u.GOOS == "darwin" }

func (u UnixSystem) out(name string, args ...string) (string, error) {
	o, err := u.Run(name, args...)
	if err != nil {
		return o, fmt.Errorf("%s did not work: %s", name, strings.TrimSpace(o))
	}
	return o, nil
}

func (u UnixSystem) wait(d time.Duration) {
	if u.Sleep != nil {
		u.Sleep(d)
	} else {
		time.Sleep(d)
	}
}

// macAdminScript runs the program and arguments it is given with administrator rights. Each one
// is quoted by AppleScript itself (quoted form of), so no argument can become part of a command.
var macAdminScript = []string{
	"-e", "on run argv",
	"-e", `set c to ""`,
	"-e", "repeat with a in argv",
	"-e", `set c to c & quoted form of (a as text) & " "`,
	"-e", "end repeat",
	"-e", "do shell script c with administrator privileges",
	"-e", "end run",
}

// change runs one change. On a Mac a change that needs administrator rights is tried as it is,
// and only when the Mac refuses is the person asked, through the Mac's own permission box.
func (u UnixSystem) change(name string, args ...string) error {
	o, err := u.Run(name, args...)
	if err == nil && !strings.Contains(o, "** Error") { // networksetup can print an error and still exit 0
		return nil
	}
	if !u.mac() {
		return fmt.Errorf("%s did not work: %s", name, strings.TrimSpace(o))
	}
	if _, lerr := u.Look("osascript"); lerr != nil {
		return fmt.Errorf("%s did not work: %s", name, strings.TrimSpace(o))
	}
	full := append(append(append([]string{}, macAdminScript...), name), args...)
	o, err = u.Run("osascript", full...)
	if err != nil {
		if strings.Contains(o, "-128") || strings.Contains(strings.ToLower(o), "canceled") {
			return errors.New("administrator rights were not given")
		}
		return fmt.Errorf("the Mac refused the change: %s", strings.TrimSpace(o))
	}
	if strings.Contains(o, "** Error") {
		return fmt.Errorf("the Mac refused the change: %s", strings.TrimSpace(o))
	}
	return nil
}

// Elevate does nothing by itself: the Mac asks at the moment of the change, and NetworkManager
// asks by itself when it must.
func (UnixSystem) Elevate() error { return nil }

// FlushDNS clears the saved list of website addresses.
func (u UnixSystem) FlushDNS() error {
	if u.Flush != nil {
		return u.Flush()
	}
	if u.mac() {
		_, err := u.out("dscacheutil", "-flushcache")
		return err
	}
	_, err := u.out("resolvectl", "flush-caches")
	return err
}

// Power saving is never read or changed on Mac and Linux (ND-307).
func (UnixSystem) PowerSaving() (string, error) { return "", errors.New(powerNeverReason) }
func (UnixSystem) SetPowerSaving(string) error  { return errors.New(powerNeverReason) }
func (UnixSystem) PowerSavingDropsLink() bool   { return true }

// ---------------------------------------------------------------------------------------------
// The adapter in use

// macService is one Mac network service and the adapter under it.
type macService struct{ Name, Device string }

// parseMacServices reads `networksetup -listnetworkserviceorder`.
func parseMacServices(out string) []macService {
	var list []macService
	name := ""
	for _, l := range strings.Split(out, "\n") {
		l = strings.TrimSpace(l)
		switch {
		case strings.HasPrefix(l, "(Hardware Port:"):
			if i := strings.Index(l, "Device:"); i >= 0 && name != "" {
				dev := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l[i+len("Device:"):]), ")"))
				if dev != "" {
					list = append(list, macService{name, dev})
				}
			}
			name = ""
		case strings.HasPrefix(l, "("):
			if i := strings.Index(l, ") "); i > 0 {
				name = strings.TrimSpace(l[i+2:])
				if strings.HasPrefix(l, "(*)") {
					name = "" // a service that is turned off is never the one in use
				}
			}
		}
	}
	return list
}

// parseMacRouteInterface reads the adapter from `route -n get default`.
func parseMacRouteInterface(out string) string {
	for _, l := range strings.Split(out, "\n") {
		if k, v, ok := strings.Cut(strings.TrimSpace(l), ":"); ok && strings.TrimSpace(k) == "interface" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

// macInUse finds the service carrying the connection: the one under the default route. With
// fallback, when there is none (no address yet), the first service whose adapter is active, as
// the Windows fixes pick the first adapter that is up.
func (u UnixSystem) macInUse(fallback bool) (macService, error) {
	o, err := u.out("networksetup", "-listnetworkserviceorder")
	if err != nil {
		return macService{}, err
	}
	list := parseMacServices(o)
	if r, err := u.Run("route", "-n", "get", "default"); err == nil {
		if dev := parseMacRouteInterface(r); dev != "" {
			for _, s := range list {
				if s.Device == dev {
					return s, nil
				}
			}
		}
	}
	for _, s := range list {
		if fallback && u.macActive(s.Device) {
			return s, nil
		}
	}
	return macService{}, errors.New(noAdapterInUse)
}

func (u UnixSystem) macActive(dev string) bool {
	o, err := u.Run("ifconfig", dev)
	return err == nil && strings.Contains(o, "status: active")
}

// parseLinuxDefaultDevices reads `ip route show default` and gives the adapters of the default
// routes, lowest metric first.
func parseLinuxDefaultDevices(out string) []string {
	type route struct {
		dev    string
		metric int
	}
	var list []route
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) == 0 || f[0] != "default" {
			continue
		}
		r := route{}
		for i := 0; i+1 < len(f); i++ {
			switch f[i] {
			case "dev":
				r.dev = f[i+1]
			case "metric":
				r.metric, _ = strconv.Atoi(f[i+1])
			}
		}
		if r.dev != "" {
			list = append(list, r)
		}
	}
	sort.SliceStable(list, func(a, b int) bool { return list[a].metric < list[b].metric })
	out2 := make([]string, len(list))
	for k, r := range list {
		out2[k] = r.dev
	}
	return out2
}

// tunnelName is true for the names VPN and tunnel adapters get, which never carry the connection
// the fixes act on.
func tunnelName(dev string) bool {
	for _, p := range []string{"tun", "tap", "wg", "ppp", "utun", "ipsec"} {
		if strings.HasPrefix(dev, p) {
			return true
		}
	}
	return false
}

// tunnelKind is true for the NetworkManager kinds of VPN and tunnel adapters.
func tunnelKind(kind string) bool {
	switch kind {
	case "tun", "wireguard", "vpn", "ppp", "ip-tunnel":
		return true
	}
	return false
}

// nmUnescape undoes nmcli's terse escaping of colons and backslashes.
func nmUnescape(s string) string {
	return strings.NewReplacer(`\:`, ":", `\\`, `\`).Replace(strings.TrimSpace(s))
}

// splitNM splits one terse nmcli line on the colons that are not escaped.
func splitNM(l string) []string {
	var parts []string
	var b strings.Builder
	esc := false
	for _, r := range l {
		switch {
		case esc:
			b.WriteRune(r)
			esc = false
		case r == '\\':
			esc = true
		case r == ':':
			parts = append(parts, b.String())
			b.Reset()
		default:
			b.WriteRune(r)
		}
	}
	return append(parts, b.String())
}

// nmConn is a NetworkManager connection: its name, shown to the person, and its unique id.
type nmConn struct{ Name, UUID string }

// nmRef is how nmcli is told which connection: by its unique id when there is one, else by name.
func nmRef(name, uuid string) []string {
	if uuid != "" {
		return []string{"uuid", uuid}
	}
	return []string{"id", name}
}

// linuxInUse finds the adapter carrying the connection and its NetworkManager connection: the
// default route's adapter with the lowest metric that is not a VPN or tunnel (by its name or the
// kind NetworkManager gives it). With fallback, when there is no such route (no address yet), the
// first wired or Wi-Fi adapter that is connecting or connected.
func (u UnixSystem) linuxInUse(fallback bool) (dev string, conn nmConn, err error) {
	kinds := map[string]string{}
	var devices [][]string
	if o, derr := u.Run("nmcli", "-t", "-f", "DEVICE,TYPE,STATE", "device"); derr == nil {
		for _, l := range strings.Split(o, "\n") {
			if f := splitNM(strings.TrimSpace(l)); len(f) >= 3 {
				kinds[f[0]] = f[1]
				devices = append(devices, f)
			}
		}
	}
	if o, rerr := u.Run("ip", "route", "show", "default"); rerr == nil {
		for _, d := range parseLinuxDefaultDevices(o) {
			if !tunnelName(d) && !tunnelKind(kinds[d]) {
				dev = d
				break
			}
		}
	}
	if dev == "" && fallback {
		for _, f := range devices {
			if (f[1] == "ethernet" || f[1] == "wifi") && strings.HasPrefix(f[2], "connect") && !tunnelName(f[0]) {
				dev = f[0]
				break
			}
		}
	}
	if dev == "" {
		return "", nmConn{}, errors.New(noAdapterInUse)
	}
	o, err := u.out("nmcli", "-t", "-g", "GENERAL.CONNECTION,GENERAL.CON-UUID", "device", "show", dev)
	if err != nil {
		return "", nmConn{}, err
	}
	lines := strings.Split(strings.TrimSpace(o), "\n")
	conn.Name = nmUnescape(lines[0])
	if len(lines) > 1 {
		conn.UUID = strings.TrimSpace(lines[1])
	}
	if conn.Name == "" || conn.Name == "--" {
		return "", nmConn{}, errors.New("the adapter in use has no NetworkManager connection")
	}
	return dev, conn, nil
}

// ---------------------------------------------------------------------------------------------
// Web proxy (ND-303)

// parseMacWebProxy reads `networksetup -getwebproxy SERVICE`.
func parseMacWebProxy(out string) ProxyState {
	var s ProxyState
	var server, port string
	for _, l := range strings.Split(out, "\n") {
		k, v, ok := strings.Cut(l, ":")
		if !ok {
			continue
		}
		v = strings.TrimSpace(v)
		switch strings.TrimSpace(k) {
		case "Enabled":
			s.Enabled = strings.EqualFold(v, "Yes")
		case "Server":
			server = v
		case "Port":
			port = v
		}
	}
	if server != "" {
		s.Server = server
		if port != "" && port != "0" {
			s.Server += ":" + port
		}
	}
	return s
}

func gsettingsValue(s string) string { return strings.Trim(strings.TrimSpace(s), `'"`) }

// Proxy reads the web proxy: on a Mac for the service in use, on Linux the desktop's own setting.
func (u UnixSystem) Proxy() (ProxyState, error) {
	if u.mac() {
		svc, err := u.macInUse(false)
		if err != nil {
			return ProxyState{}, err
		}
		o, err := u.out("networksetup", "-getwebproxy", svc.Name)
		if err != nil {
			return ProxyState{}, err
		}
		return parseMacWebProxy(o), nil
	}
	read := func(args ...string) (string, error) {
		o, err := u.out("gsettings", append([]string{"get"}, args...)...)
		return gsettingsValue(o), err
	}
	mode, err := read("org.gnome.system.proxy", "mode")
	if err != nil {
		return ProxyState{}, err
	}
	// Every part must be read: a failed read is an error, never an empty server (ND-316).
	host, err := read("org.gnome.system.proxy.http", "host")
	if err != nil {
		return ProxyState{}, err
	}
	port, err := read("org.gnome.system.proxy.http", "port")
	if err != nil {
		return ProxyState{}, err
	}
	pac, err := read("org.gnome.system.proxy", "autoconfig-url")
	if err != nil {
		return ProxyState{}, err
	}
	// The mode itself is kept, so an undo puts back exactly that mode. The server words do not
	// depend on the mode, so they read the same before and after the fix: the manual server when
	// one is set, else the automatic setup address.
	s := ProxyState{Enabled: mode == "manual" || mode == "auto", Mode: mode}
	switch {
	case host != "":
		s.Server = host
		if port != "" && port != "0" {
			s.Server += ":" + port
		}
	case pac != "":
		s.Server = macAutoProxyWords + pac
	}
	return s, nil
}

// SetProxy turns the web proxy on or off, leaving its server as it is, then reads it back (ND-243).
func (u UnixSystem) SetProxy(s ProxyState) error {
	if u.mac() {
		svc, err := u.macInUse(false)
		if err != nil {
			return err
		}
		state := "off"
		if s.Enabled {
			state = "on"
		}
		if err := u.change("networksetup", "-setwebproxystate", svc.Name, state); err != nil {
			return err
		}
	} else {
		mode := "none"
		if s.Enabled {
			mode = s.Mode // exactly the saved mode
			if mode == "" || mode == "none" {
				mode = "manual"
				if strings.HasPrefix(s.Server, macAutoProxyWords) {
					mode = "auto"
				}
			}
		}
		if err := u.change("gsettings", "set", "org.gnome.system.proxy", "mode", mode); err != nil {
			return err
		}
		s.Mode = mode
	}
	back, err := u.Proxy()
	if err != nil {
		return fmt.Errorf("the proxy could not be read back: %w", err)
	}
	if back.Enabled != s.Enabled || back.Mode != s.Mode {
		return errors.New("the computer said yes, but the proxy did not change when read back")
	}
	return nil
}

// ---------------------------------------------------------------------------------------------
// Name servers (ND-304)

func ipList(fields []string) []string {
	var out []string
	for _, f := range fields {
		if net.ParseIP(f) != nil {
			out = append(out, f)
		}
	}
	return out
}

// parseMacDNS reads `networksetup -getdnsservers SERVICE`. None set means set by the router.
func parseMacDNS(out string) []string {
	if strings.Contains(out, "aren't any DNS Servers") {
		return nil
	}
	return ipList(strings.Fields(out))
}

// parseLinuxDNS reads `nmcli -t -g ipv4.dns,ipv4.ignore-auto-dns connection show NAME`: the typed
// servers (none means set by the router) and, on the last line, whether the router's servers are
// left out. A leading empty line may have been trimmed away, so the last line is read first.
func parseLinuxDNS(out string) (servers []string, ignore *bool) {
	lines := strings.Split(strings.TrimSpace(out), "\n")
	last := strings.TrimSpace(lines[len(lines)-1])
	if last == "yes" || last == "no" {
		v := last == "yes"
		ignore = &v
		lines = lines[:len(lines)-1]
	}
	text := nmUnescape(strings.Join(lines, ","))
	return ipList(strings.FieldsFunc(text, func(r rune) bool { return r == ',' || r == ' ' })), ignore
}

func (u UnixSystem) index(dev string) int {
	if u.Index != nil {
		if n := u.Index(dev); n > 0 {
			return n
		}
	}
	return 0
}

func (u UnixSystem) macDNS(svc macService) (DNSState, error) {
	o, err := u.out("networksetup", "-getdnsservers", svc.Name)
	if err != nil {
		return DNSState{}, err
	}
	servers := parseMacDNS(o)
	return DNSState{Adapter: svc.Name, IfIndex: u.index(svc.Device), Servers: servers, Automatic: len(servers) == 0, InUse: true}, nil
}

func (u UnixSystem) linuxDNS(dev string, conn nmConn) (DNSState, error) {
	o, err := u.out("nmcli", append([]string{"-t", "-g", "ipv4.dns,ipv4.ignore-auto-dns", "connection", "show"}, nmRef(conn.Name, conn.UUID)...)...)
	if err != nil {
		return DNSState{}, err
	}
	servers, ignore := parseLinuxDNS(o)
	return DNSState{Adapter: conn.Name, ConnUUID: conn.UUID, IfIndex: u.index(dev), Servers: servers, Automatic: len(servers) == 0, InUse: true, IgnoreAuto: ignore}, nil
}

// DNS reads the name servers of the adapter in use.
func (u UnixSystem) DNS() ([]DNSState, error) {
	if u.mac() {
		svc, err := u.macInUse(false)
		if err != nil {
			return nil, err
		}
		s, err := u.macDNS(svc)
		if err != nil {
			return nil, err
		}
		return []DNSState{s}, nil
	}
	dev, conn, err := u.linuxInUse(false)
	if err != nil {
		return nil, err
	}
	s, err := u.linuxDNS(dev, conn)
	if err != nil {
		return nil, err
	}
	return []DNSState{s}, nil
}

// SetDNS sets the name servers of the given adapters (by service on a Mac, where "Empty" means set
// by the router; by connection on Linux, then applied to its adapter), then reads them back.
func (u UnixSystem) SetDNS(in []DNSState) error {
	if len(in) == 0 {
		return errors.New("no adapter was given")
	}
	for _, s := range in {
		if s.Adapter == "" {
			return errors.New("an adapter name is missing")
		}
		if !s.Automatic && len(s.Servers) == 0 {
			return errors.New("no servers were given for " + s.Adapter)
		}
		for _, ip := range s.Servers {
			if net.ParseIP(ip) == nil {
				return fmt.Errorf("%q is not a server address", ip)
			}
		}
		if u.mac() {
			args := []string{"-setdnsservers", s.Adapter}
			if s.Automatic {
				args = append(args, "Empty")
			} else {
				args = append(args, s.Servers...)
			}
			if err := u.change("networksetup", args...); err != nil {
				return err
			}
			continue
		}
		// Only these two settings are changed. A saved state puts back exactly what was read.
		servers, ignore := strings.Join(s.Servers, " "), "yes"
		if s.Automatic {
			servers, ignore = "", "no"
		}
		if s.IgnoreAuto != nil {
			ignore = map[bool]string{true: "yes", false: "no"}[*s.IgnoreAuto]
		}
		ref := nmRef(s.Adapter, s.ConnUUID)
		args := append(append([]string{"connection", "modify"}, ref...), "ipv4.dns", servers, "ipv4.ignore-auto-dns", ignore)
		if err := u.change("nmcli", args...); err != nil {
			return err
		}
		if o, err := u.Run("nmcli", append([]string{"-t", "-g", "GENERAL.DEVICES", "connection", "show"}, ref...)...); err == nil {
			if dev := nmUnescape(o); dev != "" {
				if err := u.change("nmcli", "device", "reapply", dev); err != nil {
					return err
				}
			}
		}
	}
	back, err := u.DNS()
	if err != nil {
		return err
	}
	if !allMatch(in, back) {
		return errors.New("the computer said yes, but the servers did not change when read back")
	}
	return nil
}

// ---------------------------------------------------------------------------------------------
// A new address (ND-305) and the adapter off and on (ND-306)

// parseLinuxAddress reads the IPv4 address from `ip -4 -o addr show dev DEVICE`.
func parseLinuxAddress(out string) string {
	f := strings.Fields(out)
	for i := 0; i+1 < len(f); i++ {
		if f[i] == "inet" {
			ip, _, _ := strings.Cut(f[i+1], "/")
			return ip
		}
	}
	return ""
}

func usable(ip string) bool {
	return net.ParseIP(ip) != nil && !strings.HasPrefix(ip, "169.254.")
}

// Renew asks the router for a new address for the adapter in use, then checks a usable one is there.
func (u UnixSystem) Renew() error {
	var read func() string
	if u.mac() {
		svc, err := u.macInUse(true)
		if err != nil {
			return err
		}
		if err := u.change("ipconfig", "set", svc.Device, "DHCP"); err != nil {
			return err
		}
		read = func() string { o, _ := u.Run("ipconfig", "getifaddr", svc.Device); return strings.TrimSpace(o) }
	} else {
		dev, _, err := u.linuxInUse(true)
		if err != nil {
			return err
		}
		if err := u.change("nmcli", "device", "reapply", dev); err != nil {
			return err
		}
		read = func() string {
			o, _ := u.Run("ip", "-4", "-o", "addr", "show", "dev", dev)
			return parseLinuxAddress(o)
		}
	}
	for i := 0; i < 10; i++ {
		if usable(read()) {
			return nil
		}
		u.wait(time.Second)
	}
	return errors.New("the router did not give a usable address")
}

// macResetScript switches a service off and always back on, in one step, so one permission box
// covers both. The service name is the script's own argument, never part of its text.
// It fails when either step fails, so a refused "off" is never taken for a reset.
const macResetScript = `networksetup -setnetworkserviceenabled "$1" off; a=$?; sleep 2; networksetup -setnetworkserviceenabled "$1" on; b=$?; [ $a -eq 0 ] && [ $b -eq 0 ]`

// ResetAdapter switches the adapter in use off and on (always back on), then checks it came back.
func (u UnixSystem) ResetAdapter() error {
	var up func() bool
	if u.mac() {
		svc, err := u.macInUse(true)
		if err != nil {
			return err
		}
		if err := u.change("sh", "-c", macResetScript, "sh", svc.Name); err != nil {
			return err
		}
		up = func() bool { return u.macActive(svc.Device) }
	} else {
		dev, _, err := u.linuxInUse(true)
		if err != nil {
			return err
		}
		offOut, offErr := u.Run("nmcli", "device", "disconnect", dev)
		u.wait(2 * time.Second)
		if err := u.change("nmcli", "device", "connect", dev); err != nil { // back on whatever happened
			return err
		}
		if offErr != nil {
			return fmt.Errorf("the adapter could not be turned off: %s", strings.TrimSpace(offOut))
		}
		up = func() bool {
			o, err := u.Run("nmcli", "-t", "-g", "GENERAL.STATE", "device", "show", dev)
			return err == nil && strings.HasPrefix(strings.TrimSpace(o), "100")
		}
	}
	for i := 0; i < 30; i++ {
		if up() {
			return nil
		}
		u.wait(time.Second)
	}
	return errors.New("the adapter did not come back on")
}

// Conditions reads the situation now (ND-244).
func (u UnixSystem) Conditions() (Conditions, error) {
	if u.Conds == nil {
		return Conditions{}, errors.New("the connection cannot be read on this computer")
	}
	return u.Conds()
}
