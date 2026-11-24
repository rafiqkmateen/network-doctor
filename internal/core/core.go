// Package core holds the checks and explanations. It does no input or output
// of its own, so it behaves the same on every system.
package core

import "fmt"

// Kind is the type of the active connection.
type Kind string

// Connection kinds.
const (
	Wired   Kind = "wired"
	WiFi    Kind = "wifi"
	Unknown Kind = "unknown"
)

// Cause is the most likely place a slowdown starts.
type Cause string

// Possible causes.
const (
	CauseNone          Cause = "none"
	CauseComputer      Cause = "this computer"
	CauseCableOrSignal Cause = "cable or signal"
	CauseRouter        Cause = "router"
	CauseProvider      Cause = "provider"
	CauseSignIn        Cause = "sign-in page"
)

// Reading is one set of measurements taken from a machine.
type Reading struct {
	Kind         Kind    `json:"kind"`
	Online       bool    `json:"online"`
	AdapterUp    bool    `json:"adapter_up"`
	RouterMs     float64 `json:"router_ms"`
	InternetMs   float64 `json:"internet_ms"`
	RouterLoss   float64 `json:"router_loss_pct"`
	InternetLoss float64 `json:"internet_loss_pct"`
	DownMbps     float64 `json:"down_mbps"`
	UpMbps       float64 `json:"up_mbps"`
	// SpeedMeasured is true only when the speed test finished. When false, the
	// two speeds above are 0 because nothing was measured, not because the line is slow.
	SpeedMeasured bool    `json:"speed_measured"`
	PlanMbps      float64 `json:"plan_mbps"`
	LinkMbps      int     `json:"link_mbps"`
	Duplex        string  `json:"duplex"`
	ErrorCount    int64   `json:"error_count"`
	SignalPct     int     `json:"signal_pct"`
	LocalBusyPct  int     `json:"local_busy_pct"`
	DNSFails      bool    `json:"dns_fails"`
	// What the machine has, read with the adapter. Used only when no link is up,
	// so the check does not guess which link failed.
	HasWired     bool `json:"has_wired,omitempty"`
	HasWiFi      bool `json:"has_wifi,omitempty"`
	NoPhysicalUp bool `json:"no_physical_up,omitempty"`
	// WiFiHidden says Windows hid the Wi-Fi signal, so a weak signal cannot be ruled out.
	WiFiHidden bool `json:"wifi_hidden,omitempty"`
	// NoAddress says the adapter is up but has no usable address from the router (ND-236, ND-239).
	NoAddress bool `json:"no_address,omitempty"`
	// SignIn says a sign-in page stands between this computer and the internet (ND-231).
	SignIn bool `json:"sign_in,omitempty"`
}

// Finding is the plain-words answer to a diagnosis.
type Finding struct {
	Cause   Cause  `json:"cause"`
	Summary string `json:"summary"`
	First   string `json:"check_first"`
}

const (
	slowRouterMs   = 30.0
	lossLimitPct   = 2.0
	slowInternetMs = 150.0
	busyLimitPct   = 80
)

// Diagnose names the most likely cause and the first thing to check.
func Diagnose(r Reading) Finding {
	routerBad := r.RouterLoss >= lossLimitPct || r.RouterMs > slowRouterMs
	// An answering internet proves some link works, so a stale "adapter down"
	// is not blamed on the cable or the Wi-Fi, even when the router ignores pings.
	linkWorks := r.Online
	switch {
	case r.LocalBusyPct >= busyLimitPct:
		return Finding{CauseComputer,
			"Programs on this computer are using most of the line.",
			"Look at the program list and find what is sending or getting a lot of data."}
	case !r.AdapterUp && !linkWorks:
		if r.HasWired && r.HasWiFi && r.NoPhysicalUp {
			// Both kinds are down: do not guess which one the person meant to use.
			return Finding{CauseCableOrSignal,
				"You are not connected: the cable has no link and the Wi-Fi is not connected.",
				"Plug in the cable, or join your Wi-Fi network, then run the check again."}
		}
		if r.Kind == WiFi {
			return Finding{CauseCableOrSignal, "The Wi-Fi is not connected.", "Join your Wi-Fi network and run the check again."}
		}
		if r.Kind != Wired {
			// The kind is not known (for example a mobile-broadband modem): name no cable.
			return Finding{CauseCableOrSignal, "The network connection is not working.",
				"Check that the network device is on and connected, then run the check again."}
		}
		return Finding{CauseCableOrSignal, "The cable is not giving a link.", "Check that the cable is plugged in at both ends."}
	case r.Kind == WiFi && r.AdapterUp && r.NoAddress && !r.Online:
		// Joined, but the router gave no address: the cable is not the question here (ND-239).
		return Finding{CauseRouter,
			"The Wi-Fi is connected but got no address from the router.",
			"Restart the router, or press the button that asks it for a new address."}
	case r.SignIn:
		// A sign-in page stands in the way, so pings and speed say nothing about the router or the line (ND-231).
		return Finding{CauseSignIn,
			"This Wi-Fi has a sign-in page that must be filled in before the internet works.",
			"Open a web browser and look for the sign-in page."}
	case routerBad && r.Kind == Wired && (r.ErrorCount > 0 || (r.LinkMbps > 0 && r.LinkMbps <= 100) || r.Duplex == "half"):
		return Finding{CauseCableOrSignal,
			"The cable or port looks bad.",
			"Try a different cable and a different port on the router."}
	case routerBad && r.Kind == WiFi && r.SignalPct > 0 && r.SignalPct <= 50:
		return Finding{CauseCableOrSignal,
			"The Wi-Fi signal is weak.",
			"Move closer to the router, or try the 5 GHz band."}
	case routerBad && r.Kind == WiFi && r.WiFiHidden:
		// The signal could not be read, so a weak signal is as likely as a slow router.
		return Finding{CauseRouter,
			"The router is slow to answer, or your Wi-Fi signal is weak. Windows is hiding the signal, so this check cannot tell which.",
			"Move closer to the router and run the check again. If it is still slow, restart the router."}
	case routerBad:
		return Finding{CauseRouter,
			"The router is slow to answer.",
			"Restart the router and see how many devices use it."}
	case r.InternetLoss >= lossLimitPct || r.InternetMs > slowInternetMs ||
		(r.PlanMbps > 0 && r.DownMbps > 0 && r.DownMbps < 0.5*r.PlanMbps):
		return Finding{CauseProvider,
			"The trouble is past your router, with your internet provider.",
			"Check the provider's outage page. Call them if it stays slow."}
	case r.DNSFails:
		return Finding{CauseComputer,
			"This computer cannot look up web names.",
			"Check the DNS setting on this computer."}
	}
	return Finding{CauseNone, "Your connection looks healthy.", "Nothing to fix right now."}
}

// ExitCode maps a finding to the exit code of the command-line mode.
// 0 healthy, 10 computer, 11 cable or signal, 12 router, 13 provider, 14 sign-in page.
func ExitCode(f Finding) int {
	switch f.Cause {
	case CauseComputer:
		return 10
	case CauseCableOrSignal:
		return 11
	case CauseRouter:
		return 12
	case CauseProvider:
		return 13
	case CauseSignIn:
		return 14
	}
	return 0
}

// WiredAdvice flags a slow link, half duplex and counted errors.
func WiredAdvice(linkMbps int, duplex string, errs int64) []string {
	var out []string
	if linkMbps > 0 && linkMbps <= 100 {
		out = append(out, "The link runs at 100 Mbps or less. A bad cable or an old port is a common cause.")
	}
	if duplex == "half" {
		out = append(out, "The link is in half duplex, so it can only send or get at one time. Try another cable or port.")
	}
	if errs > 0 {
		out = append(out, fmt.Sprintf("The adapter counted %d errors. Errors often come from a worn cable.", errs))
	}
	return out
}

// WiFiAdvice flags weak signal, the slower band and a crowded channel.
func WiFiAdvice(signalPct int, band string, channel int, otherChannels []int) []string {
	var out []string
	// 50 percent is about -75 dBm on Windows' scale, which counts as weak (ND-233); 0 means not read.
	if signalPct > 0 && signalPct <= 50 {
		out = append(out, fmt.Sprintf("The signal is weak, at %d percent. Move closer to the router.", signalPct))
	}
	if band == "2.4 GHz" {
		out = append(out, "You are on the 2.4 GHz band. The 5 GHz band is faster, but it does not reach as far.")
	}
	same := 0
	for _, c := range otherChannels {
		if c == channel {
			same++
		}
	}
	if same >= 2 {
		out = append(out, fmt.Sprintf("%d nearby networks use channel %d. Pick a less busy channel in the router.", same, channel))
	}
	return out
}
