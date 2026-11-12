//go:build darwin

package collect

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// Darwin reads a Mac with read-only system commands. Ping, speed and the DNS
// test come from Generic. Every command goes through darwinRun.
type Darwin struct{ Generic }

// NewSource returns the collector for this system.
func NewSource() Source { return Darwin{} }

type darwinAdapter struct {
	Iface, Gateway, Port string
	Kind                 core.Kind
	If                   darwinIfconfig
	Errors               int64
}

func (Darwin) adapter(ctx context.Context) (darwinAdapter, error) {
	var a darwinAdapter
	out, _ := darwinRun(ctx, "route", "-n", "get", "default")
	a.Iface, a.Gateway = darwinParseRoute(out)
	if a.Iface == "" {
		return a, errors.New("no network route was found. The Mac may be offline")
	}
	a.Kind = core.Unknown
	if ifOut, err := darwinRun(ctx, "ifconfig", a.Iface); err == nil || ifOut != "" {
		a.If = darwinParseIfconfig(ifOut)
	} else {
		return a, errors.New("could not read the adapter " + a.Iface)
	}
	if ports, _ := darwinRun(ctx, "networksetup", "-listallhardwareports"); ports != "" {
		if p, ok := darwinParsePorts(ports)[a.Iface]; ok {
			a.Port, a.Kind = p.Name, p.Kind
		}
	}
	if ns, _ := darwinRun(ctx, "netstat", "-ib"); ns != "" {
		c := darwinParseNetstat(ns)[a.Iface]
		a.Errors = c.InErrs + c.OutErrs
	}
	return a, nil
}

// Adapter describes the adapter carrying the default route.
func (d Darwin) Adapter(ctx context.Context) (AdapterInfo, error) {
	a, err := d.adapter(ctx)
	if err != nil {
		return AdapterInfo{}, err
	}
	desc := a.Iface
	if a.Port != "" {
		desc = a.Port + " (" + a.Iface + ")"
	}
	info := AdapterInfo{
		Name: a.Iface, Description: desc, Kind: a.Kind, Up: a.If.Up,
		LinkMbps: a.If.Mbps, Errors: a.Errors, Gateway: a.Gateway,
		IP: a.If.IP, PrefixLength: a.If.Prefix,
	}
	switch a.Kind {
	case core.Wired:
		info.Duplex = a.If.Duplex
	case core.WiFi:
		// ifconfig gives no speed for Wi-Fi; the profiler's transmit rate does.
		if w, err := d.profile(ctx); err == nil {
			info.LinkMbps = w.TxMbps
		}
	}
	return info, nil
}

func (Darwin) profile(ctx context.Context) (darwinWiFi, error) {
	c, cancel := context.WithTimeout(ctx, 25*time.Second)
	defer cancel()
	out, err := darwinRun(c, "system_profiler", "SPAirPortDataType")
	if out == "" {
		return darwinWiFi{}, fmt.Errorf("could not read the Wi-Fi report: %v", err)
	}
	return darwinParseWiFi(out), nil
}

// WiFi reads the current Wi-Fi link and the networks nearby.
func (d Darwin) WiFi(ctx context.Context) (WiFiInfo, error) {
	w, err := d.profile(ctx)
	if err != nil {
		return WiFiInfo{}, err
	}
	return w.WiFiInfo, nil
}

// Throughput samples the line for two seconds from two netstat readings.
func (d Darwin) Throughput(ctx context.Context, linkMbps int) (Throughput, error) {
	out, _ := darwinRun(ctx, "route", "-n", "get", "default")
	iface, _ := darwinParseRoute(out)
	if iface == "" {
		return Throughput{}, errors.New("no network route was found. The Mac may be offline")
	}
	first, _ := darwinRun(ctx, "netstat", "-ib")
	t0 := time.Now()
	select {
	case <-ctx.Done():
		return Throughput{}, ctx.Err()
	case <-time.After(2 * time.Second):
	}
	second, _ := darwinRun(ctx, "netstat", "-ib")
	secs := time.Since(t0).Seconds()
	a, okA := darwinParseNetstat(first)[iface]
	b, okB := darwinParseNetstat(second)[iface]
	if !okA || !okB {
		return Throughput{}, errors.New("could not read the byte counts for " + iface)
	}
	t := darwinRates(a, b, secs, linkMbps)
	c, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	lsof, _ := darwinRun(c, "lsof", "-nP", "-iTCP", "-sTCP:ESTABLISHED")
	t.Procs = darwinParseLsof(lsof)
	return t, nil
}

// Events is not read on a Mac. `log show` over seven days takes minutes, the
// Mac often keeps less than seven days of these messages, and the wording
// changes between macOS versions, so a result could be wrong or missing
// without a way to tell. The tool says so and does not guess.
func (Darwin) Events(context.Context, int) ([]core.Event, error) {
	return nil, macHistoryError{}
}

// Devices sweeps the local /24 with ping, then reads the neighbor table.
func (Darwin) Devices(ctx context.Context, ip string, prefix int) ([]core.Device, error) {
	base := net.ParseIP(ip).To4()
	if base == nil {
		return nil, errors.New("no local address")
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, 48)
	for i := 1; i < 255; i++ {
		target := net.IPv4(base[0], base[1], base[2], byte(i)).String()
		wg.Add(1)
		sem <- struct{}{}
		go func() {
			defer wg.Done()
			defer func() { <-sem }()
			c, cancel := context.WithTimeout(ctx, 2*time.Second)
			defer cancel()
			darwinRun(c, "ping", "-c", "1", "-W", "300", target)
		}()
	}
	wg.Wait()
	out, err := darwinRun(ctx, "arp", "-an")
	if out == "" {
		return nil, fmt.Errorf("could not read the list of nearby devices: %v", err)
	}
	prefix3 := fmt.Sprintf("%d.%d.%d.", base[0], base[1], base[2])
	var devs []core.Device
	for _, n := range darwinParseARP(out, "") {
		if !strings.HasPrefix(n.IP, prefix3) {
			continue
		}
		name := ""
		rs := net.Resolver{}
		c, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		if names, e := rs.LookupAddr(c, n.IP); e == nil && len(names) > 0 {
			name = strings.TrimSuffix(names[0], ".")
		}
		cancel()
		devs = append(devs, core.Device{Name: name, Maker: Maker(n.MAC), IP: n.IP, MAC: n.MAC})
	}
	sort.Slice(devs, func(i, j int) bool { return darwinIPKey(devs[i].IP) < darwinIPKey(devs[j].IP) })
	return devs, nil
}

// Settings reads DNS, VPN, proxy and adapter problems. Power saving and the
// driver are not readable on a Mac, so they are left as "not known".
func (d Darwin) Settings(ctx context.Context) (core.Settings, error) {
	var s core.Settings
	dns, dnsErr := darwinRun(ctx, "scutil", "--dns")
	proxy, proxyErr := darwinRun(ctx, "scutil", "--proxy")
	nc, ncErr := darwinRun(ctx, "scutil", "--nc", "list")
	if dns == "" && proxy == "" && nc == "" {
		return s, fmt.Errorf("could not read the network settings: %v", errors.Join(dnsErr, proxyErr, ncErr))
	}
	s.DNS = darwinParseDNS(dns)
	s.ProxyOn = darwinParseProxy(proxy)
	s.VPNActive = darwinParseVPN(nc)
	if dns != "" && len(s.DNS) == 0 {
		s.ConfigErrors = append(s.ConfigErrors, "No DNS server is set.")
	}
	if a, err := d.adapter(ctx); err == nil && strings.HasPrefix(a.If.IP, "169.254.") {
		s.ConfigErrors = append(s.ConfigErrors, "The adapter gave itself an address because the router did not give it one.")
	}
	return s, nil
}
