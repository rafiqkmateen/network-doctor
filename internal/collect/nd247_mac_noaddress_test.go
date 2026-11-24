package collect

import (
	"errors"
	"testing"
)

const nd247Ports = "Hardware Port: Ethernet\nDevice: en5\nEthernet Address: aa:bb:cc:00:00:05\n\n" +
	"Hardware Port: Wi-Fi\nDevice: en0\nEthernet Address: aa:bb:cc:00:00:00\n\n"

func nd247Ifconfig(status, inet string) string {
	out := "en0: flags=8863<UP,BROADCAST,SMART,RUNNING,SIMPLEX,MULTICAST> mtu 1500\n\tether aa:bb:cc:00:00:00\n"
	if inet != "" {
		out += "\t" + inet + "\n"
	}
	return out + "\tmedia: autoselect\n\tstatus: " + status + "\n"
}

// ND-247: a Mac that joined Wi-Fi but got no address has no default route. The check must still see the
// joined Wi-Fi and say the router gave no address, as it does on Windows and Linux (ND-239).
func TestND247MacJoinedWithNoAddress(t *testing.T) {
	ports := darwinParsePorts(nd247Ports)
	for name, inet := range map[string]string{
		"no address at all":       "",
		"a self-made 169.254 one": "inet 169.254.12.7 netmask 0xffff0000 broadcast 169.254.255.255",
	} {
		info, ok := darwinNoRouteAdapter(ports, func(dev string) string {
			if dev != "en0" {
				t.Errorf("%s: only the Wi-Fi port should be read, got %s", name, dev)
			}
			return nd247Ifconfig("active", inet)
		})
		if !ok || !info.NoAddress || !info.WiFiJoined || !info.Up || info.Name != "en0" {
			t.Errorf("%s: want a joined Wi-Fi with no address, got %v %+v", name, ok, info)
		}
	}
}

// ND-247: not joined, or joined with a real address, is not this finding.
func TestND247MacOtherCasesAreNotThisFinding(t *testing.T) {
	ports := darwinParsePorts(nd247Ports)
	for name, ifc := range map[string]string{
		"Wi-Fi off the network": nd247Ifconfig("inactive", ""),
		"a real address":        nd247Ifconfig("active", "inet 192.168.1.20 netmask 0xffffff00 broadcast 192.168.1.255"),
	} {
		if info, ok := darwinNoRouteAdapter(ports, func(string) string { return ifc }); ok {
			t.Errorf("%s: must not claim a missing address: %+v", name, info)
		}
	}
	if _, ok := darwinNoRouteAdapter(darwinParsePorts("Hardware Port: Ethernet\nDevice: en5\n"), func(string) string { return "" }); ok {
		t.Error("a Mac with no Wi-Fi port has nothing to say")
	}
}

// ND-247: with a normal route, the adapter carries the same two flags as on Windows and Linux.
func TestND247MacAdapterFlagsFromIfconfig(t *testing.T) {
	info := AdapterInfo{Up: true, IP: "169.254.9.9"}
	darwinMarkAddress(&info)
	if !info.NoAddress {
		t.Error("a self-made address with the adapter up is no usable address")
	}
	ok := AdapterInfo{Up: true, IP: "10.0.0.5"}
	darwinMarkAddress(&ok)
	if ok.NoAddress {
		t.Error("a real address is fine")
	}
}

// ND-247, audit: a Wi-Fi that works over IPv6 only is not a missing address, even with no IPv4 default route.
func TestND247MacIPv6OnlyIsNotThisFinding(t *testing.T) {
	ports := darwinParsePorts(nd247Ports)
	ipv6 := "inet6 fe80::1c2b:3d4e:5f60:7182%en0 prefixlen 64 scopeid 0x4\n\tinet6 2001:db8::5 prefixlen 64 autoconf secured"
	if info, ok := darwinNoRouteAdapter(ports, func(string) string { return nd247Ifconfig("active", ipv6) }); ok {
		t.Errorf("an IPv6 address means the network works: %+v", info)
	}
	// A link-local IPv6 address alone (every joined Wi-Fi has one) does not count as a working address.
	linkOnly := "inet6 fe80::1c2b:3d4e:5f60:7182%en0 prefixlen 64 scopeid 0x4"
	if _, ok := darwinNoRouteAdapter(ports, func(string) string { return nd247Ifconfig("active", linkOnly) }); !ok {
		t.Error("only a link-local IPv6 address and no IPv4 is still no address from the router")
	}
}

// ND-247, audit: the no-route read runs only when no default route was found, never when a route exists but
// its adapter could not be read.
func TestND247MacFallbackOnlyWithoutARoute(t *testing.T) {
	boom := errors.New("could not read the adapter en5")
	if darwinTryNoRoute("en5", boom) {
		t.Error("a route to en5 exists; the Wi-Fi guess must not replace it")
	}
	if !darwinTryNoRoute("", errors.New("no network route was found")) {
		t.Error("no route at all is the case this read is for")
	}
	if darwinTryNoRoute("en0", nil) {
		t.Error("no error, nothing to replace")
	}
}
