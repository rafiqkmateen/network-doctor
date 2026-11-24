package app

import (
	"bytes"
	"context"
	"net"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// vpnFake is a pretend machine with a VPN on.
type vpnFake struct{ *fake }

func (v vpnFake) Settings(context.Context) (core.Settings, error) {
	return core.Settings{VPNActive: true}, nil
}

type serveCall struct {
	addr  string
	allow []string
}

// fakeServe stands in for the web page. It records how it was started, can
// pretend a phone opened the page, and runs until the context ends.
func fakeServe(call *serveCall, phoneAt time.Duration, phone string) func(context.Context, string, []string, func() ([]byte, error), func(string)) error {
	var mu sync.Mutex
	return func(ctx context.Context, addr string, allow []string, run func() ([]byte, error), onRemote func(string)) error {
		mu.Lock()
		call.addr, call.allow = addr, allow
		mu.Unlock()
		if phone != "" {
			go func() {
				time.Sleep(phoneAt)
				onRemote(phone)
			}()
		}
		<-ctx.Done()
		return nil
	}
}

func shareRun(t *testing.T, src collect.Source, serve func(context.Context, string, []string, func() ([]byte, error), func(string)) error, wait time.Duration, ifaces []ifaceInfo) (int, string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	var out bytes.Buffer
	code := Share(ctx, src, ShareOptions{Port: 8765, Wait: wait, Serve: serve, Interfaces: ifaces}, Options{PlanMbps: 300}, &out)
	return code, out.String()
}

// ND-130 and ND-133: the page opens on the real adapter's address and allows only its own network.
func TestShareUsesRealAdapterAndOwnNetworkOnly(t *testing.T) {
	var call serveCall
	code, out := shareRun(t, healthy(), fakeServe(&call, 0, ""), time.Second, nil)
	if code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if call.addr != "192.168.1.20:8765" {
		t.Errorf("should listen only on the adapter's address, got %q", call.addr)
	}
	if len(call.allow) != 1 || call.allow[0] != "192.168.1.0/24" {
		t.Errorf("should allow only its own network, got %v", call.allow)
	}
	if !strings.Contains(out, "http://192.168.1.20:8765/") {
		t.Errorf("the address must be printed:\n%s", out)
	}
}

// ND-130: a VPN adapter is not the address to hand to a phone.
func TestShareSkipsVPNAdapter(t *testing.T) {
	f := healthy()
	f.adapter.Description = "WireGuard Tunnel"
	f.adapter.IP = "10.6.0.2"
	f.adapter.PrefixLength = 24
	ifaces := []ifaceInfo{
		{Name: "wg0", Up: true, Addrs: []string{"10.6.0.2/24"}},
		{Name: "Ethernet", Up: true, Addrs: []string{"fe80::1/64", "192.168.1.20/24"}},
	}
	var call serveCall
	if code, out := shareRun(t, f, fakeServe(&call, 0, ""), time.Second, ifaces); code != 0 {
		t.Fatalf("code %d\n%s", code, out)
	}
	if call.addr != "192.168.1.20:8765" {
		t.Errorf("should pick the real network, got %q", call.addr)
	}
}

// ND-131: the screen shows a code to scan. Light squares on a dark screen, with a clear border.
func TestShareDrawsCode(t *testing.T) {
	var call serveCall
	_, out := shareRun(t, healthy(), fakeServe(&call, 0, ""), time.Second, nil)
	lines := qrLines(out)
	if len(lines) < 12 {
		t.Fatalf("a code needs many lines, got %d\n%s", len(lines), out)
	}
	w := len([]rune(lines[0]))
	for _, l := range lines {
		if len([]rune(l)) != w {
			t.Fatal("every line of the code must be as wide as the first")
		}
	}
	if strings.Trim(lines[0], "█") != "" || strings.Trim(lines[len(lines)-1], "█") != "" {
		t.Error("the code needs a clear border of light squares top and bottom")
	}
	if !strings.ContainsAny(strings.Join(lines, ""), " ▀▄") {
		t.Error("the code should have dark squares too")
	}
}

// qrLines picks the lines made only of block characters and spaces after the code heading.
func qrLines(out string) []string {
	re := regexp.MustCompile(`^[█▀▄ ]{20,}$`)
	var lines []string
	for _, l := range strings.Split(out, "\n") {
		if re.MatchString(l) {
			lines = append(lines, l)
		}
	}
	return lines
}

// ND-132: the first other device is announced; with none, plain advice follows the wait.
func TestShareAnnouncesPhoneOrAdvises(t *testing.T) {
	var call serveCall
	_, out := shareRun(t, healthy(), fakeServe(&call, 30*time.Millisecond, "192.168.1.57"), 400*time.Millisecond, nil)
	if !strings.Contains(out, "192.168.1.57") || !strings.Contains(out, "reached") {
		t.Errorf("should say a phone reached the page:\n%s", out)
	}
	if strings.Contains(out, "Allow") {
		t.Errorf("no advice once the phone got through:\n%s", out)
	}

	_, out = shareRun(t, healthy(), fakeServe(&call, 0, ""), 100*time.Millisecond, nil)
	for _, want := range []string{"Allow", "same Wi-Fi", "guest"} {
		if !strings.Contains(out, want) {
			t.Errorf("advice should mention %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "VPN is on") {
		t.Error("no VPN advice when no VPN is on")
	}

	_, out = shareRun(t, vpnFake{healthy()}, fakeServe(&call, 0, ""), 100*time.Millisecond, nil)
	if !strings.Contains(out, "VPN is on") {
		t.Errorf("should name an active VPN:\n%s", out)
	}
}

// ND-134: no usable address means a plain message and no page.
func TestShareWithNoAddress(t *testing.T) {
	f := healthy()
	f.adapter.IP = ""
	called := false
	serve := func(context.Context, string, []string, func() ([]byte, error), func(string)) error {
		called = true
		return nil
	}
	code, out := shareRun(t, f, serve, time.Second, []ifaceInfo{{Name: "lo", Up: true, Addrs: []string{"127.0.0.1/8"}}})
	if code == 0 || called {
		t.Fatalf("should refuse: code %d, started %v", code, called)
	}
	if !strings.Contains(out, "could not find") {
		t.Errorf("should say so plainly:\n%s", out)
	}
}

func TestFindAddressRejectsNonPrivate(t *testing.T) {
	ad := collect.AdapterInfo{IP: "8.8.8.8", PrefixLength: 24}
	if _, _, ok := findAddress(ad, nil); ok {
		t.Fatal("a public address is not a local network address")
	}
	ip, n, ok := findAddress(collect.AdapterInfo{IP: "192.168.4.9", PrefixLength: 22}, nil)
	if !ok || ip.String() != "192.168.4.9" || n.String() != "192.168.4.0/22" {
		t.Fatalf("%v %v %v", ip, n, ok)
	}
	var _ net.IP = ip
}

// ND-05: the words of the share command read at grade 8 or lower.
func TestShareTextsReadable(t *testing.T) {
	for _, s := range shareTexts() {
		if g := core.GradeLevel(s); g > 8.0 {
			t.Errorf("grade %.1f too high: %q", g, s)
		}
	}
}
