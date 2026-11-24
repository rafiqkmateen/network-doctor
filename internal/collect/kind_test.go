package collect

import (
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// ND-10 (the Windows and Linux parts), ND-258: wired or Wi-Fi is told from sample facts, the
// way the Mac part is told from a sample port list. This file has no build tag and no system in
// its name, so the tests run on Windows, Mac and Linux alike. The adapter names are made up.

// ND-258: Windows decides from the adapter's physical media type first, then its description.
func TestWindowsKind(t *testing.T) {
	cases := []struct {
		name, media, desc string
		want              core.Kind
	}{
		{"Wi-Fi by media type", "Native 802.11", "Example Wireless Network Card", core.WiFi},
		{"Wi-Fi by media type, plain description", "Native 802.11", "Example Network Card", core.WiFi},
		{"wired by media type", "802.3", "Example Gigabit Network Controller", core.Wired},
		{"Wi-Fi by description only", "", "Example Wi-Fi 6 Adapter", core.WiFi},
		{"wireless word in description", "Unspecified", "Example Wireless LAN Card", core.WiFi},
		{"media type wins over description", "802.3", "Example Wireless Dock Port", core.Wired},
		{"neither", "Unspecified", "Example Bluetooth Personal Area Network", core.Unknown},
		{"empty", "", "", core.Unknown},
	}
	for _, c := range cases {
		if got := windowsKind(c.media, c.desc); got != c.want {
			t.Errorf("%s: windowsKind(%q, %q) = %v, want %v", c.name, c.media, c.desc, got, c.want)
		}
	}
}

// ND-258: Linux decides from the wireless folder, then the device folder, then the adapter name.
func TestLinuxKind(t *testing.T) {
	cases := []struct {
		name, dev           string
		wireless, hasDevice bool
		want                core.Kind
	}{
		{"Wi-Fi by wireless folder", "wlp2s0", true, true, core.WiFi},
		{"wired by device folder", "enp3s0", false, true, core.Wired},
		{"wired by device folder, name not a known wired name", "usb0", false, true, core.Wired},
		{"wired by name only", "eth0", false, false, core.Wired},
		{"virtual adapter", "docker0", false, false, core.Unknown},
		{"tunnel", "tun0", false, false, core.Unknown},
		{"empty", "", false, false, core.Unknown},
	}
	for _, c := range cases {
		if got := linuxKind(c.dev, c.wireless, c.hasDevice); got != c.want {
			t.Errorf("%s: linuxKind(%q, %v, %v) = %v, want %v",
				c.name, c.dev, c.wireless, c.hasDevice, got, c.want)
		}
	}
}

// A mobile-broadband modem (a name starting "ww") is not a cable adapter.
// With no wireless folder it is unknown, even with a device folder, which a modem has too.
// Real Wi-Fi and real wired adapters keep the answer they had before.
func TestLinuxKindMobileBroadband(t *testing.T) {
	cases := []struct {
		name, dev           string
		wireless, hasDevice bool
		want                core.Kind
	}{
		{"modem, no folders", "wwan0", false, false, core.Unknown},
		{"modem, device folder only", "wwp0s20f0u2i12", false, true, core.Unknown},
		{"Wi-Fi by wireless folder, unchanged", "wlp2s0", true, false, core.WiFi},
		{"Wi-Fi name, no folders, unchanged", "wlp2s0", false, false, core.Wired},
		{"wired by device folder, unchanged", "enp3s0", false, true, core.Wired},
		{"wired by name only, unchanged", "eth0", false, false, core.Wired},
	}
	for _, c := range cases {
		if got := linuxKind(c.dev, c.wireless, c.hasDevice); got != c.want {
			t.Errorf("%s: linuxKind(%q, %v, %v) = %v, want %v",
				c.name, c.dev, c.wireless, c.hasDevice, got, c.want)
		}
	}
}
