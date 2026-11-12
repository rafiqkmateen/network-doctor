package app

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
)

// defaultSpeedWaitSeconds is the wait used when the speed server refuses a test
// but does not say how long to wait.
const defaultSpeedWaitSeconds = 600

const speedHoldFile = "speed-hold.txt"

// waitWords says a wait in plain words: "in about 56 minutes".
func waitWords(d time.Duration) string {
	if d <= 0 {
		return "in a little while"
	}
	mins := int((d + time.Minute - 1) / time.Minute)
	if mins <= 1 {
		return "in about 1 minute"
	}
	return fmt.Sprintf("in about %d minutes", mins)
}

// LoadSpeedHold gives the time before which the speed test must not run, or the
// zero time when there is no hold. A missing, damaged or finished hold is none.
func LoadSpeedHold(dir string) time.Time {
	b, err := os.ReadFile(filepath.Join(dir, speedHoldFile))
	if err != nil {
		return time.Time{}
	}
	until, err := time.Parse(time.RFC3339, strings.TrimSpace(string(b)))
	if err != nil || !until.After(time.Now()) {
		return time.Time{}
	}
	return until
}

// SaveSpeedHold remembers that the speed test must wait until the given time.
func SaveSpeedHold(dir string, until time.Time) {
	_ = os.WriteFile(filepath.Join(dir, speedHoldFile), []byte(until.Format(time.RFC3339)+"\n"), 0o644)
}

// runCheck is Check with the speed hold kept between runs: it honors a saved
// hold and saves a new one when the speed server refuses a test.
func runCheck(ctx context.Context, src collect.Source, dir string, opt Options) Report {
	opt.SpeedHoldUntil = LoadSpeedHold(dir)
	rep := Check(ctx, src, opt)
	if rep.SpeedRetryAfterSeconds > 0 {
		SaveSpeedHold(dir, rep.Time.Add(time.Duration(rep.SpeedRetryAfterSeconds)*time.Second))
	}
	return rep
}
