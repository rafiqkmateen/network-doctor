package collect

// The wired-or-Wi-Fi decision for Windows and Linux, kept apart from the calls that run the
// system's commands and read its files. This file has no build tag, so the tests feed it sample
// facts on every system (ND-10, ND-258), the way darwinKind is tested for the Mac.

import (
	"regexp"
	"strings"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// windowsWiFiDesc finds a Wi-Fi adapter by its description when the media type does not say.
var windowsWiFiDesc = regexp.MustCompile(`(?i)wi-?fi|wireless|802\.11`)

// windowsKind decides wired or Wi-Fi from what Windows reports for the adapter: its physical
// media type (PhysicalMediaType) first, then its description. Anything else is unknown.
func windowsKind(media, desc string) core.Kind {
	switch {
	case strings.Contains(media, "802.11"):
		return core.WiFi
	case strings.Contains(media, "802.3"):
		return core.Wired
	case windowsWiFiDesc.MatchString(desc):
		return core.WiFi
	}
	return core.Unknown
}

// linuxKind decides wired or Wi-Fi from what Linux shows for the adapter: wireless is true when
// it has a wireless or phy80211 folder under /sys/class/net, hasDevice when it has a device
// folder there. Without either, a real adapter's name still counts as wired. A mobile-broadband
// modem (a name starting "ww") is not a cable adapter, and it has a device folder too, so with no
// wireless folder it is unknown.
func linuxKind(dev string, wireless, hasDevice bool) core.Kind {
	switch {
	case wireless:
		return core.WiFi
	case strings.HasPrefix(dev, "ww"):
		return core.Unknown
	case hasDevice || linuxPhysicalName(dev):
		return core.Wired
	}
	return core.Unknown
}
