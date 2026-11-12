package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
)

func refusedMachine() *fake {
	f := healthy()
	f.down, f.up = 0, 0
	f.speedErr = &collect.SpeedRefusedError{RetryAfter: 56 * time.Minute}
	return f
}

func speedCalls(f *fake) int {
	n := 0
	for _, c := range f.calls {
		if c == "speed" {
			n++
		}
	}
	return n
}

// ND-180: a speed test the server refused is "not measured", never a reading of 0.
func TestRefusedSpeedIsNotMeasuredNotZero(t *testing.T) {
	ok := Check(context.Background(), healthy(), Options{PlanMbps: 300})
	if !ok.Reading.SpeedMeasured {
		t.Error("a finished speed test must say it was measured")
	}
	r := Check(context.Background(), refusedMachine(), Options{PlanMbps: 300})
	if r.Reading.SpeedMeasured {
		t.Error("a refused speed test must not say it was measured")
	}
	if r.SpeedRetryAfterSeconds != 3360 {
		t.Errorf("wait %d seconds, want 3360", r.SpeedRetryAfterSeconds)
	}
	joined := strings.ToLower(strings.Join(r.Notes, " "))
	if !strings.Contains(joined, "try again") || !strings.Contains(joined, "56 minutes") {
		t.Errorf("the note should say to try again in 56 minutes: %v", r.Notes)
	}
	quick := Check(context.Background(), healthy(), Options{Quick: true})
	if quick.Reading.SpeedMeasured {
		t.Error("a quick check skips the speed test, so nothing was measured")
	}
}

// ND-181: while the server asked us to wait, the speed test is not run at all.
func TestSpeedHoldSkipsTheTest(t *testing.T) {
	f := healthy()
	r := Check(context.Background(), f, Options{PlanMbps: 300, SpeedHoldUntil: time.Now().Add(30 * time.Minute)})
	if speedCalls(f) != 0 {
		t.Fatal("the speed test must not run during a hold")
	}
	if r.Reading.SpeedMeasured {
		t.Error("nothing was measured during a hold")
	}
	if !strings.Contains(strings.ToLower(strings.Join(r.Notes, " ")), "paused") {
		t.Errorf("the note should say the speed test is paused: %v", r.Notes)
	}
	g := healthy()
	Check(context.Background(), g, Options{PlanMbps: 300, SpeedHoldUntil: time.Now().Add(-time.Minute)})
	if speedCalls(g) != 1 {
		t.Error("a hold that has ended must not stop the test")
	}
}

// ND-181: the hold is kept in a small file, and a missing, old or damaged file means no hold.
func TestSpeedHoldFile(t *testing.T) {
	dir := t.TempDir()
	if !LoadSpeedHold(dir).IsZero() {
		t.Error("no file means no hold")
	}
	until := time.Now().Add(time.Hour).Truncate(time.Second)
	SaveSpeedHold(dir, until)
	if got := LoadSpeedHold(dir); !got.Equal(until) {
		t.Errorf("got %v, want %v", got, until)
	}
	SaveSpeedHold(dir, time.Now().Add(-time.Hour))
	if !LoadSpeedHold(dir).IsZero() {
		t.Error("a hold that has ended counts as none")
	}
	os.WriteFile(filepath.Join(dir, "speed-hold.txt"), []byte("not a time"), 0o644)
	if !LoadSpeedHold(dir).IsZero() {
		t.Error("a damaged file counts as none")
	}
}

// ND-181: after a refusal the next check does not ask the server again.
func TestCheckKeepsTheHoldBetweenRuns(t *testing.T) {
	dir := t.TempDir()
	f := refusedMachine()
	var out bytes.Buffer
	Run(context.Background(), []string{"check", "--json"}, f, dir, &out)
	if speedCalls(f) != 1 {
		t.Fatalf("first check should try once, tried %d", speedCalls(f))
	}
	if LoadSpeedHold(dir).IsZero() {
		t.Fatal("a refusal must be remembered")
	}
	out.Reset()
	Run(context.Background(), []string{"check", "--json"}, f, dir, &out)
	if speedCalls(f) != 1 {
		t.Errorf("the second check must not ask again, tried %d times in all", speedCalls(f))
	}
}

// ND-71, ND-182: an unmeasured speed is never saved as a weekly row, so it cannot start a false complaint.
func TestWeeklyDoesNotSaveAnUnmeasuredSpeed(t *testing.T) {
	dir := t.TempDir()
	f := refusedMachine()
	for i := 0; i < 3; i++ {
		var out bytes.Buffer
		Run(context.Background(), []string{"weekly", "--plan", "300"}, f, dir, &out)
		if i == 0 && !strings.Contains(out.String(), "not saved") {
			t.Errorf("the weekly check should say the row was not saved: %q", out.String())
		}
		os.Remove(filepath.Join(dir, "speed-hold.txt")) // ask again each time, as three real weeks would
	}
	if b, err := os.ReadFile(filepath.Join(dir, "weekly.csv")); err == nil && strings.Count(string(b), "\n") > 1 {
		t.Errorf("no row may be saved without a measured speed: %q", b)
	}
	if _, err := os.Stat(filepath.Join(dir, "complaint-draft.txt")); err == nil {
		t.Error("no complaint may be drafted from unmeasured speeds")
	}
}
