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
	PlanMbps     float64 `json:"plan_mbps"`
	LinkMbps     int     `json:"link_mbps"`
	Duplex       string  `json:"duplex"`
	ErrorCount   int64   `json:"error_count"`
	SignalPct    int     `json:"signal_pct"`
	LocalBusyPct int     `json:"local_busy_pct"`
	DNSFails     bool    `json:"dns_fails"`
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
	switch {
	case r.LocalBusyPct >= busyLimitPct:
		return Finding{CauseComputer,
			"Programs on this computer are using most of the line.",
			"Look at the program list and find what is sending or getting a lot of data."}
	case !r.AdapterUp:
		if r.Kind == WiFi {
			return Finding{CauseCableOrSignal, "The Wi-Fi is not connected.", "Join your Wi-Fi network and run the check again."}
		}
		return Finding{CauseCableOrSignal, "The cable is not giving a link.", "Check that the cable is plugged in at both ends."}
	case routerBad && r.Kind == Wired && (r.ErrorCount > 0 || (r.LinkMbps > 0 && r.LinkMbps <= 100) || r.Duplex == "half"):
		return Finding{CauseCableOrSignal,
			"The cable or port looks bad.",
			"Try a different cable and a different port on the router."}
	case routerBad && r.Kind == WiFi && r.SignalPct > 0 && r.SignalPct < 50:
		return Finding{CauseCableOrSignal,
			"The Wi-Fi signal is weak.",
			"Move closer to the router, or try the 5 GHz band."}
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
// 0 healthy, 10 computer, 11 cable or signal, 12 router, 13 provider.
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
	if signalPct > 0 && signalPct < 50 {
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
