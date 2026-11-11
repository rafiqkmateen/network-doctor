//go:build windows

package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// Windows reads the machine through PowerShell, netsh and ping, read-only.
type Windows struct{}

// NewSource returns the collector for this system.
func NewSource() Source { return Windows{} }

func runPS(ctx context.Context, script string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, "powershell.exe", "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-Command", script)
	hide(cmd)
	var out bytes.Buffer
	cmd.Stdout = &out
	if err := cmd.Run(); err != nil {
		return trimOut(out.Bytes()), err
	}
	return trimOut(out.Bytes()), nil
}

func runExe(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	hide(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

type rawAdapter struct {
	Name, Desc, Status, Media string
	LinkBps                   int64
	FullDuplex                bool
	Gateway                   string
	RxErr, TxErr              int64
	DriverVersion, DriverDate string
	PowerSaving, PowerHint    string
	IP                        string
	Prefix                    int
}

func (Windows) rawAdapter(ctx context.Context) (rawAdapter, error) {
	b, err := runPS(ctx, scriptAdapter)
	var a rawAdapter
	if len(b) == 0 {
		return a, fmt.Errorf("no adapter data: %v", err)
	}
	return a, json.Unmarshal(b, &a)
}

// Adapter describes the adapter carrying the default route.
func (w Windows) Adapter(ctx context.Context) (AdapterInfo, error) {
	a, err := w.rawAdapter(ctx)
	if err != nil {
		return AdapterInfo{}, err
	}
	info := AdapterInfo{
		Name: a.Name, Description: a.Desc, Up: a.Status == "Up",
		LinkMbps: int(a.LinkBps / 1_000_000), Errors: a.RxErr + a.TxErr,
		Gateway: a.Gateway, IP: a.IP, PrefixLength: a.Prefix, DriverVersion: a.DriverVersion,
		Kind: core.Unknown,
	}
	switch {
	case strings.Contains(a.Media, "802.11"):
		info.Kind = core.WiFi
	case strings.Contains(a.Media, "802.3"):
		info.Kind = core.Wired
	case regexp.MustCompile(`(?i)wi-?fi|wireless|802\.11`).MatchString(a.Desc):
		info.Kind = core.WiFi
	}
	if info.Kind == core.Wired {
		info.Duplex = "half"
		if a.FullDuplex {
			info.Duplex = "full"
		}
	}
	if t, e := time.Parse("2006-01-02", a.DriverDate); e == nil {
		info.DriverAgeDays = int(time.Since(t).Hours() / 24)
	}
	return info, nil
}

// WiFi reads the current Wi-Fi link and the networks nearby.
func (w Windows) WiFi(ctx context.Context) (WiFiInfo, error) {
	var info WiFiInfo
	out, err := runExe(ctx, "netsh", "wlan", "show", "interfaces")
	if err != nil && out == "" {
		return info, err
	}
	info = ParseWlanInterfaces(out)
	nets, _ := runExe(ctx, "netsh", "wlan", "show", "networks", "mode=bssid")
	info.OtherChannels = ParseWlanChannels(nets, info.Channel)
	return info, nil
}

// Ping runs the system ping.
func (Windows) Ping(ctx context.Context, host string, count int) (PingResult, error) {
	return doPing(ctx, host, count)
}

// Speed measures download and upload.
func (Windows) Speed(ctx context.Context) (float64, float64, error) { return doSpeed(ctx) }

// DNSWorks tests whether a web name resolves.
func (Windows) DNSWorks(ctx context.Context) bool { return doDNSWorks(ctx) }

// Throughput samples the line for two seconds.
func (Windows) Throughput(ctx context.Context, linkMbps int) (Throughput, error) {
	b, err := runPS(ctx, scriptThroughput)
	if len(b) == 0 {
		return Throughput{}, fmt.Errorf("no throughput data: %v", err)
	}
	return ParseThroughput(b, linkMbps)
}

// Events reads connection changes from the Windows log.
func (Windows) Events(ctx context.Context, days int) ([]core.Event, error) {
	b, err := runPS(ctx, strings.ReplaceAll(scriptEvents, "%DAYS%", strconv.Itoa(days)))
	if len(b) == 0 {
		return nil, fmt.Errorf("no log data: %v", err)
	}
	var raw []struct {
		T  string
		Id int
	}
	if err := looseList(b, &raw); err != nil {
		return nil, err
	}
	var ev []core.Event
	for _, r := range raw {
		t, e := time.Parse(time.RFC3339Nano, r.T)
		if e != nil {
			continue
		}
		ev = append(ev, core.Event{Time: t.Local(), Up: r.Id == 10000})
	}
	return ev, nil
}

// Devices sweeps the local /24, then reads the neighbor table.
func (Windows) Devices(ctx context.Context, ip string, prefix int) ([]core.Device, error) {
	base := net.ParseIP(ip).To4()
	if base == nil {
		return nil, fmt.Errorf("no local address")
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
			runExe(c, "ping", "-n", "1", "-w", "300", target)
		}()
	}
	wg.Wait()
	b, err := runPS(ctx, scriptNeighbors)
	if len(b) == 0 {
		return nil, fmt.Errorf("no neighbor data: %v", err)
	}
	var raw []struct{ IP, MAC string }
	if err := looseList(b, &raw); err != nil {
		return nil, err
	}
	var devs []core.Device
	prefix3 := fmt.Sprintf("%d.%d.%d.", base[0], base[1], base[2])
	for _, r := range raw {
		if !strings.HasPrefix(r.IP, prefix3) || strings.HasSuffix(r.IP, ".255") {
			continue
		}
		name := ""
		rs := net.Resolver{}
		c, cancel := context.WithTimeout(ctx, 800*time.Millisecond)
		if names, e := rs.LookupAddr(c, r.IP); e == nil && len(names) > 0 {
			name = strings.TrimSuffix(names[0], ".")
		}
		cancel()
		devs = append(devs, core.Device{Name: name, Maker: Maker(r.MAC), IP: r.IP, MAC: core.NormalizeMAC(r.MAC)})
	}
	sort.Slice(devs, func(i, j int) bool { return ipKey(devs[i].IP) < ipKey(devs[j].IP) })
	return devs, nil
}

func ipKey(s string) int {
	ip := net.ParseIP(s).To4()
	if ip == nil {
		return 0
	}
	return int(ip[0])<<24 | int(ip[1])<<16 | int(ip[2])<<8 | int(ip[3])
}

// Settings reads DNS, power saving, VPN, proxy, driver and adapter errors.
func (w Windows) Settings(ctx context.Context) (core.Settings, error) {
	var s core.Settings
	a, err := w.rawAdapter(ctx)
	if err == nil {
		s.PowerKnown, s.PowerSaving = PowerState(a.PowerSaving, a.PowerHint)
		s.DriverVersion = a.DriverVersion
		if t, e := time.Parse("2006-01-02", a.DriverDate); e == nil {
			s.DriverAgeDays = int(time.Since(t).Hours() / 24)
		}
	}
	b, err2 := runPS(ctx, scriptSettings)
	if len(b) == 0 {
		return s, fmt.Errorf("no settings data: %v", err2)
	}
	var raw struct {
		DNS    stringList
		Vpn    int
		Proxy  bool
		Errors stringList
	}
	if err := json.Unmarshal(b, &raw); err != nil {
		return s, err
	}
	s.DNS, s.VPNActive, s.ProxyOn, s.ConfigErrors = raw.DNS, raw.Vpn > 0, raw.Proxy, raw.Errors
	return s, nil
}
