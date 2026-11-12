package app

import (
	"context"
	"encoding/json"
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

// ageWords says how old a reading is in plain words: "3 minutes ago".
func ageWords(d time.Duration) string {
	mins := int(d / time.Minute)
	switch {
	case mins < 1:
		return "less than a minute ago"
	case mins == 1:
		return "1 minute ago"
	}
	return fmt.Sprintf("%d minutes ago", mins)
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

// minSpeedGap is how long a measured speed is reused before the test runs again.
// Checks that run back to back then go easy on the public speed server.
const minSpeedGap = 10 * time.Minute

const lastSpeedFile = "speed-last.json"

// SpeedSample is one measured speed and when it was measured.
type SpeedSample struct {
	At   time.Time `json:"at"`
	Down float64   `json:"down_mbps"`
	Up   float64   `json:"up_mbps"`
}

// LoadLastSpeed gives the last measured speed, or nil when there is none or the file is damaged.
func LoadLastSpeed(dir string) *SpeedSample {
	b, err := os.ReadFile(filepath.Join(dir, lastSpeedFile))
	if err != nil {
		return nil
	}
	var s SpeedSample
	if json.Unmarshal(b, &s) != nil || s.At.IsZero() {
		return nil
	}
	return &s
}

// SaveLastSpeed remembers a measured speed. Only a measured speed is ever saved.
func SaveLastSpeed(dir string, s SpeedSample) {
	if b, err := json.Marshal(s); err == nil {
		_ = os.WriteFile(filepath.Join(dir, lastSpeedFile), b, 0o644)
	}
}

// runCheck is Check with the speed memory kept between runs: it honors a saved
// hold, reuses a recent speed, saves a new hold when the speed server refuses a
// test, and saves each newly measured speed.
func runCheck(ctx context.Context, src collect.Source, dir string, opt Options) Report {
	opt.SpeedHoldUntil = LoadSpeedHold(dir)
	opt.LastSpeed = LoadLastSpeed(dir)
	rep := Check(ctx, src, opt)
	if rep.SpeedRetryAfterSeconds > 0 {
		SaveSpeedHold(dir, rep.Time.Add(time.Duration(rep.SpeedRetryAfterSeconds)*time.Second))
	}
	if rep.Reading.SpeedMeasured && rep.SpeedAgeSeconds == 0 {
		SaveLastSpeed(dir, SpeedSample{At: rep.Time, Down: rep.Reading.DownMbps, Up: rep.Reading.UpMbps})
	}
	return rep
}
