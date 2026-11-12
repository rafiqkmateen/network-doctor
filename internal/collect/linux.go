//go:build linux

package collect

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// Linux reads the machine through the ip, ethtool, iw, nmcli, resolvectl,
// journalctl and ss commands and a few /proc and /sys files, read-only. Ping,
// the speed test and the DNS test come from Generic. Every command goes
// through linuxRun, which refuses anything not on the list.
type Linux struct{ Generic }

// NewSource returns the collector for this system.
func NewSource() Source { return Linux{} }

// primary finds the adapter the computer uses: the one with the default
// route, or a real adapter when there is no route.
func (Linux) primary(ctx context.Context) (LinuxRoute, []LinuxAddr, error) {
	out, err := linuxRun(ctx, "ip", "-j", "addr", "show")
	addrs, perr := ParseLinuxAddrs(out)
	if perr != nil {
		if err != nil {
			return LinuxRoute{}, nil, errors.New("the ip command could not be run")
		}
		return LinuxRoute{}, nil, perr
	}
	rout, _ := linuxRun(ctx, "ip", "-j", "route", "show")
	rt, rerr := ParseLinuxRoutes(rout)
	if rerr == nil {
		return rt, addrs, nil
	}
	if a, ok := LinuxPickPhysical(addrs); ok {
		return LinuxRoute{Dev: a.Name}, addrs, nil
	}
	return LinuxRoute{}, addrs, errors.New("no network adapter was found")
}

func (Linux) sysInt(dev, file string) (int64, bool) {
	s, err := linuxReadFile("/sys/class/net/" + dev + "/" + file)
	if err != nil {
		return 0, false
	}
	return ParseLinuxInt(s)
}

func (l Linux) isWireless(dev string) bool {
	return linuxExists("/sys/class/net/"+dev+"/wireless") || linuxExists("/sys/class/net/"+dev+"/phy80211")
}

// Adapter describes the adapter carrying the default route.
func (l Linux) Adapter(ctx context.Context) (AdapterInfo, error) {
	rt, addrs, err := l.primary(ctx)
	if err != nil {
		return AdapterInfo{}, err
	}
	info := AdapterInfo{Name: rt.Dev, Kind: core.Unknown, Gateway: rt.Gateway}
	if a, ok := LinuxFindAddr(addrs, rt.Dev); ok {
		info.Up, info.IP, info.PrefixLength = a.Up, a.IP, a.Prefix
	}
	if info.IP == "" && rt.Src != "" {
		info.IP = rt.Src
	}
	switch {
	case l.isWireless(rt.Dev):
		info.Kind = core.WiFi
	case linuxExists("/sys/class/net/"+rt.Dev+"/device") || linuxPhysicalName(rt.Dev):
		info.Kind = core.Wired
	}

	rx, _ := l.sysInt(rt.Dev, "statistics/rx_errors")
	tx, _ := l.sysInt(rt.Dev, "statistics/tx_errors")
	info.Errors = rx + tx

	switch info.Kind {
	case core.Wired:
		if v, ok := l.sysInt(rt.Dev, "speed"); ok && v > 0 {
			info.LinkMbps = int(v)
		}
		if s, e := linuxReadFile("/sys/class/net/" + rt.Dev + "/duplex"); e == nil {
			info.Duplex = LinuxDuplex(s)
		}
		if info.Up && (info.LinkMbps == 0 || info.Duplex == "") {
			if out, _ := linuxRun(ctx, "ethtool", rt.Dev); out != "" {
				mbps, dup := ParseLinuxEthtool(out)
				if info.LinkMbps == 0 {
					info.LinkMbps = mbps
				}
				if info.Duplex == "" {
					info.Duplex = dup
				}
			}
		}
	case core.WiFi:
		if out, _ := linuxRun(ctx, "iw", "dev", rt.Dev, "link"); out != "" {
			if _, mbps, ok := ParseLinuxIwLink(out); ok {
				info.LinkMbps = mbps
			}
		}
	}
	if out, _ := linuxRun(ctx, "ethtool", "-i", rt.Dev); out != "" {
		drv, ver := ParseLinuxEthtoolDriver(out)
		info.Description = drv
		info.DriverVersion = ver
	}
	return info, nil
}

// WiFi reads the current Wi-Fi link and the networks nearby.
func (l Linux) WiFi(ctx context.Context) (WiFiInfo, error) {
	rt, _, err := l.primary(ctx)
	if err != nil {
		return WiFiInfo{}, err
	}
	if !l.isWireless(rt.Dev) {
		return WiFiInfo{}, errors.New("the connection in use is not Wi-Fi")
	}
	var info WiFiInfo
	got := false
	if out, _ := linuxRun(ctx, "iw", "dev", rt.Dev, "link"); out != "" {
		i, _, connected := ParseLinuxIwLink(out)
		if connected {
			info, got = i, true
		} else if strings.Contains(out, "Not connected") {
			return WiFiInfo{}, errors.New("Wi-Fi is not connected")
		}
	}
	// What the system already knows about nearby networks. This does not
	// start a new scan and needs no administrator rights.
	var others []int
	haveOthers := false
	nm, _ := linuxRun(ctx, "nmcli", "-t", "-f", "IN-USE,CHAN,FREQ,SIGNAL", "device", "wifi", "list", "--rescan", "no")
	if rows := ParseLinuxNmcliWifi(nm); len(rows) > 0 {
		haveOthers = true
		for _, r := range rows {
			if r.InUse {
				if !got {
					_, band := LinuxChannel(r.Freq)
					info = WiFiInfo{SignalPct: r.SignalPct, Channel: r.Channel, Band: band}
					got = true
				}
				continue
			}
			others = append(others, r.Channel)
		}
	}
	if !haveOthers {
		if out, _ := linuxRun(ctx, "iw", "dev", rt.Dev, "scan", "dump"); out != "" {
			others = ParseLinuxIwScanDump(out)
		}
	}
	if !got {
		return WiFiInfo{}, errors.New("the Wi-Fi link could not be read (iw is missing)")
	}
	info.OtherChannels = others
	return info, nil
}

// Throughput samples the line for two seconds from /proc/net/dev.
func (l Linux) Throughput(ctx context.Context, linkMbps int) (Throughput, error) {
	rt, _, err := l.primary(ctx)
	if err != nil {
		return Throughput{}, err
	}
	t1, err := linuxReadFile("/proc/net/dev")
	if err != nil {
		return Throughput{}, errors.New("the adapter counters could not be read")
	}
	start := time.Now()
	c1, ok := ParseLinuxNetDev(t1)[rt.Dev]
	if !ok {
		return Throughput{}, errors.New("the adapter counters could not be found")
	}
	var th Throughput
	if out, _ := linuxRun(ctx, "ss", "-H", "-tnp", "state", "established"); out != "" {
		th.Procs = ParseLinuxSS(out)
	}
	if wait := 2*time.Second - time.Since(start); wait > 0 {
		select {
		case <-time.After(wait):
		case <-ctx.Done():
			return Throughput{}, ctx.Err()
		}
	}
	t2, err := linuxReadFile("/proc/net/dev")
	if err != nil {
		return Throughput{}, errors.New("the adapter counters could not be read")
	}
	secs := time.Since(start).Seconds()
	c2, ok := ParseLinuxNetDev(t2)[rt.Dev]
	if !ok || secs <= 0 {
		return Throughput{}, errors.New("the adapter counters could not be found")
	}
	var rx, tx float64
	if c2.Rx >= c1.Rx {
		rx = float64(c2.Rx-c1.Rx) / secs
	}
	if c2.Tx >= c1.Tx {
		tx = float64(c2.Tx-c1.Tx) / secs
	}
	th.RxMBps, th.TxMBps = rx/1e6, tx/1e6
	if linkMbps > 0 {
		th.BusyPct = int((rx + tx) * 8 / (float64(linkMbps) * 1e6) * 100)
	}
	return th, nil
}

// Events reads seven days of link changes from the system log.
func (l Linux) Events(ctx context.Context, days int) ([]core.Event, error) {
	if days < 1 || days > 365 {
		days = 7
	}
	rt, _, err := l.primary(ctx)
	if err != nil {
		return nil, err
	}
	since := fmt.Sprintf("--since=-%dd", days)
	var all []core.Event
	readable := false
	k, _ := linuxRun(ctx, "journalctl", "--no-pager", "-k", "-o", "short-iso", since)
	if ev, e := ParseLinuxJournal(k, rt.Dev); e == nil {
		readable = true
		all = append(all, ev...)
	}
	n, _ := linuxRun(ctx, "journalctl", "--no-pager", "-u", "NetworkManager", "-o", "short-iso", since)
	if ev, e := ParseLinuxJournal(n, rt.Dev); e == nil {
		readable = true
		all = append(all, ev...)
	}
	if !readable {
		return nil, errors.New("the system log could not be read without administrator rights")
	}
	ev := LinuxMergeEvents(all)
	for i := range ev {
		ev[i].Time = ev[i].Time.Local()
	}
	return ev, nil
}

// Devices sweeps the local /24 with ping, then reads the neighbor table.
func (l Linux) Devices(ctx context.Context, ip string, prefix int) ([]core.Device, error) {
	base := net.ParseIP(ip).To4()
	if base == nil {
		return nil, errors.New("there is no local address")
	}
	rt, _, _ := l.primary(ctx)
	var wg sync.WaitGroup
	sem := make(chan struct{}, 48)
	for i := 1; i < 255; i++ {
		target := net.IPv4(base[0], base[1], base[2], byte(i)).String()
		if target == ip {
			continue
		}
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			c, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			linuxRun(c, "ping", "-c", "1", "-W", "1", target)
		}()
	}
	wg.Wait()
	text, err := linuxReadFile("/proc/net/arp")
	if err != nil {
		return nil, errors.New("the neighbor table could not be read")
	}
	prefix3 := fmt.Sprintf("%d.%d.%d.", base[0], base[1], base[2])
	neigh := ParseLinuxArp(text, rt.Dev, prefix3)
	devs := make([]core.Device, len(neigh))
	for i, n := range neigh {
		devs[i] = core.Device{Maker: Maker(n.MAC), IP: n.IP, MAC: n.MAC}
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
			defer cancel()
			rs := net.Resolver{}
			if names, e := rs.LookupAddr(c, n.IP); e == nil && len(names) > 0 {
				devs[i].Name = strings.TrimSuffix(names[0], ".")
			}
		}()
	}
	wg.Wait()
	return devs, nil
}

// Settings reads DNS, power saving, VPN, proxy, driver and adapter errors.
func (l Linux) Settings(ctx context.Context) (core.Settings, error) {
	var s core.Settings
	read := 0
	rt, addrs, err := l.primary(ctx)
	if err == nil {
		read++
		s.VPNActive = LinuxVPNActive(addrs)
	}
	dev := rt.Dev

	if out, _ := linuxRun(ctx, "resolvectl", "status"); out != "" {
		if dns := ParseLinuxResolvectl(out, dev); len(dns) > 0 {
			s.DNS = dns
			read++
		}
	}
	if len(s.DNS) == 0 {
		for _, f := range []string{"/run/systemd/resolve/resolv.conf", "/etc/resolv.conf"} {
			text, e := linuxReadFile(f)
			if e != nil {
				continue
			}
			if dns := ParseLinuxResolvConf(text); len(dns) > 0 {
				read++
				if !LinuxIsStub(dns) || f == "/etc/resolv.conf" {
					s.DNS = dns
					break
				}
			}
		}
	}

	if dev != "" {
		if l.isWireless(dev) {
			if out, _ := linuxRun(ctx, "iw", "dev", dev, "get", "power_save"); out != "" {
				s.PowerKnown, s.PowerSaving = ParseLinuxPowerSave(out)
				if s.PowerKnown {
					read++
				}
			}
		}
		if out, _ := linuxRun(ctx, "ethtool", "-i", dev); out != "" {
			_, s.DriverVersion = ParseLinuxEthtoolDriver(out)
		}
		if out, _ := linuxRun(ctx, "nmcli", "-t", "-f", "GENERAL.STATE", "device", "show", dev); out != "" {
			s.ConfigErrors = ParseLinuxNmcliState(out)
			read++
		}
	}

	s.ProxyOn = LinuxProxyEnv(os.Getenv)
	if !s.ProxyOn {
		if out, _ := linuxRun(ctx, "gsettings", "get", "org.gnome.system.proxy", "mode"); out != "" {
			s.ProxyOn = ParseLinuxProxyMode(out)
		}
	}
	if read == 0 {
		return core.Settings{}, errors.New("the settings could not be read")
	}
	return s, nil
}
