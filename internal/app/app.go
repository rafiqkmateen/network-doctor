// Package app runs the checks and the commands.
package app

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// Version of the tool.
const Version = "0.1.0"

// Options control one check.
type Options struct {
	PlanMbps float64
	Quick    bool
	Known    map[string]bool
	// SpeedHoldUntil: while this time is in the future the speed test is not run,
	// because the test server asked for a wait.
	SpeedHoldUntil time.Time
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
}

const internetHost = "1.1.1.1"

// Check runs every check the options allow. It only reads.
func Check(ctx context.Context, src collect.Source, opt Options) Report {
	now := time.Now()
	rep := Report{Time: now, Quick: opt.Quick}
	note := func(s string) { rep.Notes = append(rep.Notes, s) }

	ad, err := src.Adapter(ctx)
	if err != nil {
		note("Adapter facts: " + err.Error() + ".")
	}
	rd := core.Reading{Kind: ad.Kind, AdapterUp: ad.Up, LinkMbps: ad.LinkMbps, Duplex: ad.Duplex, ErrorCount: ad.Errors, PlanMbps: opt.PlanMbps}
	if rd.Kind == "" {
		rd.Kind = core.Unknown
	}
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
	rd.LocalBusyPct = thr.BusyPct
	if !rd.Online {
		note("You are offline, so the speed test and the internet checks were skipped.")
	} else {
		rd.DNSFails = !dnsOK
		if !opt.Quick {
			if opt.SpeedHoldUntil.After(now) {
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
				}
			}
		}
	}

	rep.Reading = rd
	rep.Finding = core.Diagnose(rd)
	switch rd.Kind {
	case core.Wired:
		rep.LinkAdvice = core.WiredAdvice(rd.LinkMbps, rd.Duplex, rd.ErrorCount)
	case core.WiFi:
		if wifiErr != nil {
			note("Wi-Fi facts: " + wifiErr.Error() + ".")
		} else {
			rep.LinkAdvice = core.WiFiAdvice(wifi.SignalPct, wifi.Band, wifi.Channel, wifi.OtherChannels)
		}
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
				devs[i].Known = opt.Known[core.NormalizeMAC(devs[i].MAC)] || devs[i].IP == ad.IP
			}
			rep.Devices = devs
			for _, d := range devs {
				if !d.Known {
					rep.Unknown = append(rep.Unknown, d)
				}
			}
		}
	}
	return rep
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
