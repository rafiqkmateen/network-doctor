package app

import (
	"context"
	"fmt"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// speedExtras is the part of collect.Extras this file uses: the second test. A source without
// it is not asked.
type speedExtras interface {
	SpeedCloudflare(ctx context.Context) (collect.SpeedPair, error)
}

// fillSpeedExtras runs the second speed test when the first looks wrong (ND-234). It never sets
// a VPN hint: a region setting that differs from the line's country is normal, so that guess was
// removed.
//
// It runs only after a first speed test measured in this check: never in quick mode, never
// inside a speed hold, never with an earlier reading standing in, and never after a refusal.
func fillSpeedExtras(ctx context.Context, rep *Report, rd *core.Reading, src collect.Source, opt Options) {
	if opt.Quick || !rd.Online || !rd.SpeedMeasured || rep.SpeedAgeSeconds != 0 || rep.SpeedRetryAfterSeconds != 0 || opt.SpeedHoldUntil.After(time.Now()) {
		return
	}
	sx, ok := src.(speedExtras)
	if !ok {
		return
	}
	if firstLooksWrong(rd.DownMbps, rd.UpMbps, opt.LastWeeklyDown) {
		pair, err := sx.SpeedCloudflare(ctx)
		rc := &SpeedRecheck{FirstDown: rd.DownMbps, FirstUp: rd.UpMbps, FirstTool: rep.SpeedTool, Tool: collect.ToolCloudflare}
		if err != nil {
			// A refusal here never sets a hold: the hold belongs to the first test only.
			rc.Headline = fmt.Sprintf("The first speed number (%.0f down) looks wrong, and a second test could not finish.", rd.DownMbps)
			rc.Why = "Only the first number is shown, so take it with care."
		} else {
			rc.SecondDown, rc.SecondUp, rc.Finished = pair.Down, pair.Up, true
			rc.Headline = fmt.Sprintf("The first speed number looks wrong. A second test gave %.0f down and %.0f up.", pair.Down, pair.Up)
			rc.Why = fmt.Sprintf("The first test gave %.0f down and %.0f up. The plan check uses the higher download.", rd.DownMbps, rd.UpMbps)
			if pair.Down > rd.DownMbps {
				// The plan check in the finding uses the higher number, so one odd number is
				// never the answer. The shown pair is then the second test's, named as Cloudflare.
				rd.DownMbps, rd.UpMbps = pair.Down, pair.Up
				rep.SpeedTool = collect.ToolCloudflare
			}
		}
		rep.SpeedRecheck = rc
	}
}

// firstLooksWrong is true when the download is under one fifth of the upload (with upload at
// least 20 megabits), or under one fifth of the last weekly download (0 means none).
func firstLooksWrong(down, up, weekly float64) bool {
	if up >= 20 && down < up/5 {
		return true
	}
	return weekly > 0 && down < weekly/5
}
