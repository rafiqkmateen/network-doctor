// Package app runs the checks and the commands.
package app

import (
	"context"
	"errors"
	"net"
	"sync"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/store"
)

// Version of the tool.
const Version = "0.4.0"

// Options control one check.
type Options struct {
	PlanMbps float64
	Quick    bool
	Known    map[string]bool
	// LoadKnown, when set, reads the known list again; the watcher calls it at every full check.
	// When nil, Known is used as it is.
	LoadKnown func() map[string]bool
	// SpeedHoldUntil: while this time is in the future the speed test is not run,
	// because the test server asked for a wait.
	SpeedHoldUntil time.Time
	// LastSpeed: a speed measured less than ten minutes ago stands in for a new test.
	LastSpeed *SpeedSample
	// FixDir is the results folder that holds the fixes' saved values, the one "fix undo" reads.
	// Empty means no undos are listed.
	FixDir string
	// LastWeeklyDown is the download of the last weekly check in megabits; 0 means none (ND-234).
	LastWeeklyDown float64
	// States holds the person's choice and saved name for each device on the known list (ND-264).
	// LoadStates, when set, reads them again; the watcher calls it at every full check.
	States     map[string]store.KnownEntry
	LoadStates func() map[string]store.KnownEntry
	// NoNewDeviceWarnings is true when the person turned warnings about new devices off (ND-265).
	// NewDeviceWarningsOff, when set, is asked again at every full check by the watcher.
	NoNewDeviceWarnings  bool
	NewDeviceWarningsOff func() bool
}

// Report is the full result of a check.
type Report struct {
	Time            time.Time       `json:"time"`
	Reading         core.Reading    `json:"reading"`
	Finding         core.Finding    `json:"finding"`
	LinkAdvice      []string        `json:"link_advice"`
	Drops           core.DropReport `json:"drops"`
	Procs           []core.Proc     `json:"procs"`
	BandwidthAdvice []string        `json:"bandwidth_advice"`
	LineDownMBps    float64         `json:"line_down_mbytes_per_sec"`
	LineUpMBps      float64         `json:"line_up_mbytes_per_sec"`
	Devices         []core.Device   `json:"devices"`
	Unknown         []core.Device   `json:"unknown_devices"`
	Settings        []core.Advice   `json:"settings"`
	Notes           []string        `json:"notes"`
	Quick           bool            `json:"quick"`
	// SpeedRetryAfterSeconds is set only when the speed server refused the test.
	SpeedRetryAfterSeconds int `json:"speed_retry_after_seconds,omitempty"`
	// SpeedAgeSeconds is set only when the speed is an earlier reading, not a new one.
	SpeedAgeSeconds int `json:"speed_age_seconds,omitempty"`
	// SpeedWaitSeconds is set only with an earlier reading: how long until a new test is allowed.
	SpeedWaitSeconds int `json:"speed_wait_seconds,omitempty"`
	// SpeedRetryAt is set only with an earlier reading: the time a new test is allowed, the one
	// the too-soon note names, so the page can show it in the viewer's own time format (ND-262).
	SpeedRetryAt *time.Time `json:"speed_retry_at,omitempty"`
	// SpeedTool names the tool that measured the speed shown (Ookla Speedtest or Cloudflare).
	SpeedTool string `json:"speed_tool,omitempty"`
	// Fixes names the small fixes that fit what the check found; FixOffers says what each would change.
	// Undos lists the fixes that can be put back now. None of these changes anything.
	Fixes     []string   `json:"fixes"`
	FixOffers []FixOffer `json:"fix_offers"`
	Undos     []FixUndo  `json:"undos"`
	// WiFiHidden is set only on Wi-Fi when Windows hides the signal, band and
	// channel because Location services is off.
	WiFiHidden *WiFiHidden `json:"wifi_hidden,omitempty"`
	// SignIn, Clock and VPNHint are plain notes set only when the check finds the fact (ND-231, ND-232, ND-241).
	SignIn  *Note `json:"sign_in,omitempty"`
	Clock   *Note `json:"clock,omitempty"`
	VPNHint *Note `json:"vpn_hint,omitempty"`
	// ExitPlace is where the internet leaves from, in plain words; VPNHint is the note when that place changed (ND-248).
	ExitPlace string `json:"exit_place,omitempty"`
	// SpeedRecheck is set only when a first speed number looked wrong and a second test ran (ND-234).
	SpeedRecheck *SpeedRecheck `json:"speed_recheck,omitempty"`
	// KnownCount is how many devices on the tool's own list the person marked mine or not mine (ignoring
	// alone is not counted), so the page can word the list neutrally until then (ND-264).
	KnownCount int `json:"known_count"`
	// WarnNewDevices is the "Warn me about new devices" switch; on unless turned off (ND-265).
	WarnNewDevices bool `json:"warn_new_devices"`
	// RouterPage is the router's own page, set only when the router address the check uses is a
	// private address. The page links to it; Network Doctor never logs in there (ND-264).
	RouterPage string `json:"router_page,omitempty"`
	// Clock24h is this computer's own time setting, true for a 24-hour clock; left out when not
	// known, and the page then uses the browser's format (ND-262).
	Clock24h *bool `json:"clock_24h,omitempty"`
}

// Note is a plain headline and reason.
type Note struct {
	Headline string `json:"headline"`
	Why      string `json:"why"`
}

// SpeedRecheck holds both speed results when the first one looked wrong.
type SpeedRecheck struct {
	FirstDown  float64 `json:"first_down"`
	FirstUp    float64 `json:"first_up"`
	SecondDown float64 `json:"second_down"`
	SecondUp   float64 `json:"second_up"`
	Tool       string  `json:"tool"`
	FirstTool  string  `json:"first_tool"` // the plain name of the tool that ran the first test; "" when unknown
	Finished   bool    `json:"finished"`
	Headline   string  `json:"headline"`
	Why        string  `json:"why"`
}

// WiFiHidden is the plain headline and reason shown when Windows hides the Wi-Fi details.
type WiFiHidden struct {
	Headline string `json:"headline"`
	Why      string `json:"why"`
}

// hiddenWiFiWords is the one place these words live; they state a fact and ask nothing of the user.
var hiddenWiFiWords = WiFiHidden{
	Headline: "Windows is hiding your Wi-Fi signal, band and channel.",
	Why:      "Windows only shares them with programs while Location is on. Network Doctor does not use your location and never changes that setting. Speed, drops and devices are not affected.",
}

// neutralHiddenWiFiWords are used when only the failed request and the consent value say the
// details are hidden (a Windows set to another language), so no setting is named (ND-242).
var neutralHiddenWiFiWords = WiFiHidden{
	Headline: "Windows did not share the Wi-Fi details.",
	Why:      "Windows would not give the signal, band and channel to this program. Speed, drops and devices are not affected.",
}

const internetHost = "1.1.1.1"

// speedTooSoonNote opens the note shown while an earlier speed stands in. The page finds that note
// by these words and shows its own sentence instead, with the time in the viewer's format (ND-262).
const speedTooSoonNote = "Too soon to test again."

// Check runs every check the options allow. It only reads.
func Check(ctx context.Context, src collect.Source, opt Options) Report {
	now := time.Now()
	// Read the lists and the switch again when the caller gave the ways to, so a page that runs
	// for a long time (share) sees choices made since it started.
	if opt.LoadKnown != nil {
		opt.Known = opt.LoadKnown()
	}
	if opt.LoadStates != nil {
		opt.States = opt.LoadStates()
	}
	if opt.NewDeviceWarningsOff != nil {
		opt.NoNewDeviceWarnings = opt.NewDeviceWarningsOff()
	}
	rep := Report{Time: now, Quick: opt.Quick, WarnNewDevices: !opt.NoNewDeviceWarnings, Clock24h: systemClock24h()}
	// Choices made: devices on the tool's own list marked mine or not mine. A trusted list kept by
	// another program does not count, and neither does ignoring a device (ND-264).
	for _, e := range opt.States {
		if e.State == store.Mine || e.State == store.NotMine {
			rep.KnownCount++
		}
	}
	note := func(s string) { rep.Notes = append(rep.Notes, s) }

	ad, err := src.Adapter(ctx)
	if err != nil {
		note("Adapter facts: " + err.Error() + ".")
	}
	rd := core.Reading{Kind: ad.Kind, AdapterUp: ad.Up, LinkMbps: ad.LinkMbps, Duplex: ad.Duplex, ErrorCount: ad.Errors, PlanMbps: opt.PlanMbps,
		HasWired: ad.HasWired, HasWiFi: ad.HasWiFi, NoPhysicalUp: ad.NoPhysicalUp}
	if rd.Kind == "" {
		rd.Kind = core.Unknown
	}
	rep.RouterPage = routerPage(ad.Gateway)
	count := 8
	if opt.Quick {
		count = 4
	}

	var (
		wg            sync.WaitGroup
		routerP, netP collect.PingResult
		wifi          collect.WiFiInfo
		wifiErr       error
		events        []core.Event
		evErr         error
		set           core.Settings
		setErr        error
		thr           collect.Throughput
		thrErr        error
		dnsOK         bool
	)
	run := func(f func()) { wg.Add(1); go func() { defer wg.Done(); f() }() }
	if ad.Gateway != "" {
		run(func() { routerP, _ = src.Ping(ctx, ad.Gateway, count) })
	}
	run(func() { netP, _ = src.Ping(ctx, internetHost, count) })
	run(func() { dnsOK = src.DNSWorks(ctx) })
	if rd.Kind == core.WiFi {
		run(func() { wifi, wifiErr = src.WiFi(ctx) })
	}
	run(func() { thr, thrErr = src.Throughput(ctx, ad.LinkMbps) })
	run(func() { set, setErr = src.Settings(ctx) })
	if !opt.Quick {
		run(func() { events, evErr = src.Events(ctx, 7) })
	}
	wg.Wait()

	rd.RouterMs, rd.RouterLoss = routerP.AvgMs, routerP.LossPct
	rd.InternetMs, rd.InternetLoss = netP.AvgMs, netP.LossPct
	rd.Online = netP.LossPct < 100
	rd.SignalPct = wifi.SignalPct
	rd.NoAddress = ad.NoAddress
	rd.LocalBusyPct = thr.BusyPct
	if !rd.Online {
		note("You are offline, so the speed test and the internet checks were skipped.")
	} else {
		rd.DNSFails = !dnsOK
		if !opt.Quick {
			if opt.LastSpeed != nil && now.Sub(opt.LastSpeed.At) >= 0 && now.Sub(opt.LastSpeed.At) < holdFor(opt.LastSpeed.Tool) {
				// A speed measured a few minutes ago stands in: nothing is measured inside the hold.
				rd.DownMbps, rd.UpMbps = opt.LastSpeed.Down, opt.LastSpeed.Up
				rd.SpeedMeasured = true
				age := now.Sub(opt.LastSpeed.At)
				wait := holdFor(opt.LastSpeed.Tool) - age
				rep.SpeedAgeSeconds = int(age.Seconds())
				if rep.SpeedAgeSeconds < 1 {
					rep.SpeedAgeSeconds = 1
				}
				rep.SpeedWaitSeconds = int(wait.Seconds())
				rep.SpeedTool = opt.LastSpeed.Tool
				at := now.Add(wait)
				rep.SpeedRetryAt = &at
				note(speedTooSoonNote + " This is the reading from " + ageWords(age) + ". Come back " + waitWords(wait) + " (at " + at.Format("1504") + ").")
			} else if opt.SpeedHoldUntil.After(now) {
				note("The speed test is paused. The test server asked us to wait. It can run again " + waitWords(opt.SpeedHoldUntil.Sub(now)) + ".")
			} else {
				down, up, err := src.Speed(ctx)
				rd.DownMbps, rd.UpMbps = down, up
				var refused *collect.SpeedRefusedError
				switch {
				case errors.As(err, &refused):
					rd.DownMbps, rd.UpMbps = 0, 0
					rep.SpeedRetryAfterSeconds = int(refused.RetryAfter.Seconds())
					if rep.SpeedRetryAfterSeconds <= 0 {
						rep.SpeedRetryAfterSeconds = defaultSpeedWaitSeconds
					}
					note("The speed test server has had enough tests for now. Try again " + waitWords(time.Duration(rep.SpeedRetryAfterSeconds)*time.Second) + ".")
				case err != nil:
					note("The speed test could not finish.")
				default:
					rd.SpeedMeasured = true
					rep.SpeedTool = collect.SpeedToolUsed()
				}
			}
		}
	}

	fillSpeedExtras(ctx, &rep, &rd, src, opt)
	fillReadingExtras(ctx, &rep, &rd, src, opt)

	// Hidden details matter only while the Wi-Fi is up; when it is down the
	// finding already says it is not connected.
	rd.WiFiHidden = rd.Kind == core.WiFi && rd.AdapterUp && wifiErr == nil && wifi.Hidden
	rep.Reading = rd
	rep.Finding = core.Diagnose(rd)
	switch rd.Kind {
	case core.Wired:
		rep.LinkAdvice = core.WiredAdvice(rd.LinkMbps, rd.Duplex, rd.ErrorCount)
	case core.WiFi:
		switch {
		case wifiErr != nil:
			note("Wi-Fi facts: " + wifiErr.Error() + ".")
		case rd.WiFiHidden:
			words := hiddenWiFiWords
			if wifi.HiddenNeutral {
				words = neutralHiddenWiFiWords
			}
			rep.WiFiHidden = &words
		case wifi.Hidden:
			// The Wi-Fi is down: nothing more to say about its details.
		default:
			rep.LinkAdvice = core.WiFiAdvice(wifi.SignalPct, wifi.Band, wifi.Channel, wifi.OtherChannels)
		}
		// The link speed comes from the adapter, so it is known even when the details are hidden (ND-233).
		rep.LinkAdvice = append(rep.LinkAdvice, core.WiFiLinkAdvice(rd.LinkMbps)...)
	}
	if thrErr != nil {
		note("Program use: " + thrErr.Error() + ".")
	}
	rep.Procs = core.RankProcs(thr.Procs)
	rep.LineDownMBps, rep.LineUpMBps = thr.RxMBps, thr.TxMBps
	rep.BandwidthAdvice = core.BandwidthAdvice(thr.RxMBps, thr.TxMBps, rep.Procs)

	if setErr != nil {
		note("Settings: " + setErr.Error() + ".")
		set = core.Settings{}
	}
	set.DriverVersion, set.DriverAgeDays = pick(set.DriverVersion, ad.DriverVersion), maxInt(set.DriverAgeDays, ad.DriverAgeDays)
	rep.Settings = core.ReviewSettings(set)

	if !opt.Quick {
		if evErr != nil {
			note("Connection history: " + evErr.Error() + ".")
		}
		rep.Drops = core.AnalyzeDrops(events, now)
		if ad.IP != "" {
			devs, err := src.Devices(ctx, ad.IP, ad.PrefixLength)
			if err != nil {
				note("Devices: " + err.Error() + ".")
			}
			for i := range devs {
				mac := core.NormalizeMAC(devs[i].MAC)
				devs[i].This = devs[i].IP == ad.IP
				devs[i].Known = opt.Known[mac] || devs[i].This
				if e, ok := opt.States[mac]; ok {
					devs[i].State, devs[i].Label = string(e.State), e.Name
				}
			}
			rep.Devices = devs
			for _, d := range devs {
				// An ignored device is never counted as a stranger (ND-264).
				if !d.Known && d.State != string(store.Ignored) {
					rep.Unknown = append(rep.Unknown, d)
				}
			}
		}
	}
	// The fixes that fit, cut to those the program can offer (reading a setting changes nothing).
	rep.FixOffers = fixOffers(core.SuggestFixes(rd, set, rep.Drops), "")
	rep.Fixes = []string{}
	for _, o := range rep.FixOffers {
		rep.Fixes = append(rep.Fixes, o.Name)
	}
	rep.Undos = fixUndos(opt.FixDir, rep.Fixes)
	return rep
}

// routerPage is the address of the router's own page, or "" unless the router address is a
// private one (home and office networks). Only the address the check already pings is used.
func routerPage(gateway string) string {
	ip := net.ParseIP(gateway)
	if ip == nil || !ip.IsPrivate() {
		return ""
	}
	if ip.To4() == nil {
		return "http://[" + ip.String() + "]/"
	}
	return "http://" + ip.String() + "/"
}

func pick(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
