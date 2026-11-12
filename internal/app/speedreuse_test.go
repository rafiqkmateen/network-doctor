package app

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// ND-184: a speed measured under ten minutes ago is reused, so back-to-back checks go easy on the server.
func TestSpeedIsReusedWithinTenMinutes(t *testing.T) {
	f := healthy()
	last := &SpeedSample{At: time.Now().Add(-3 * time.Minute), Down: 811, Up: 700}
	r := Check(context.Background(), f, Options{PlanMbps: 300, LastSpeed: last})
	if speedCalls(f) != 0 {
		t.Fatal("a speed from three minutes ago must be reused, not measured again")
	}
	if r.Reading.DownMbps != 811 || r.Reading.UpMbps != 700 || !r.Reading.SpeedMeasured {
		t.Errorf("the reused speed should fill the reading: %+v", r.Reading)
	}
	if r.SpeedAgeSeconds < 170 || r.SpeedAgeSeconds > 200 {
		t.Errorf("age %d seconds, want about 180", r.SpeedAgeSeconds)
	}
	if !strings.Contains(strings.Join(r.Notes, " "), "3 minutes ago") {
		t.Errorf("the note should say how old the speed is: %v", r.Notes)
	}

	g := healthy()
	old := &SpeedSample{At: time.Now().Add(-11 * time.Minute), Down: 811, Up: 700}
	fresh := Check(context.Background(), g, Options{PlanMbps: 300, LastSpeed: old})
	if speedCalls(g) != 1 || fresh.SpeedAgeSeconds != 0 || fresh.Reading.DownMbps != 290 {
		t.Errorf("a speed from eleven minutes ago must be measured again: calls %d, age %d, down %.0f", speedCalls(g), fresh.SpeedAgeSeconds, fresh.Reading.DownMbps)
	}
}

// ND-184: the last measured speed is kept in a small file between runs; only a measured speed is ever saved.
func TestLastSpeedIsKeptBetweenRuns(t *testing.T) {
	dir := t.TempDir()
	f := healthy()
	var out1, out2 bytes.Buffer
	Run(context.Background(), []string{"check", "--json"}, f, dir, &out1)
	Run(context.Background(), []string{"check", "--json"}, f, dir, &out2)
	if speedCalls(f) != 1 {
		t.Fatalf("two checks in a row should measure once, measured %d times", speedCalls(f))
	}
	var second map[string]any
	if err := json.Unmarshal(out2.Bytes(), &second); err != nil {
		t.Fatal(err)
	}
	if age, _ := second["speed_age_seconds"].(float64); age < 0 {
		t.Errorf("age %v", second["speed_age_seconds"])
	}
	if rd, _ := second["reading"].(map[string]any); rd["down_mbps"] != float64(290) || rd["speed_measured"] != true {
		t.Errorf("the second check should carry the first speed: %v", second["reading"])
	}

	refusedDir := t.TempDir()
	Run(context.Background(), []string{"check", "--json"}, refusedMachine(), refusedDir, &bytes.Buffer{})
	if _, err := os.Stat(filepath.Join(refusedDir, "speed-last.json")); err == nil {
		t.Error("a speed that was not measured must never be saved")
	}
	if LoadLastSpeed(t.TempDir()) != nil {
		t.Error("no file means no last speed")
	}
	os.WriteFile(filepath.Join(refusedDir, "speed-last.json"), []byte("{broken"), 0o644)
	if LoadLastSpeed(refusedDir) != nil {
		t.Error("a damaged file means no last speed")
	}
}
