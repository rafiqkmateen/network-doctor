package app

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
)

// ND-204: the hold on a new speed test is 2 minutes when Ookla's tool measured and 10 minutes when Cloudflare measured.
func TestHoldDependsOnTheTool(t *testing.T) {
	cases := []struct {
		tool    string
		ago     time.Duration
		reused  bool
		comment string
	}{
		{collect.ToolOokla, 1 * time.Minute, true, "Ookla, 1 minute ago: still held"},
		{collect.ToolOokla, 3 * time.Minute, false, "Ookla, 3 minutes ago: free"},
		{collect.ToolCloudflare, 3 * time.Minute, true, "Cloudflare, 3 minutes ago: still held"},
		{collect.ToolCloudflare, 11 * time.Minute, false, "Cloudflare, 11 minutes ago: free"},
		{"", 5 * time.Minute, true, "an older reading with no tool name counts as Cloudflare"},
	}
	for _, c := range cases {
		f := healthy()
		last := &SpeedSample{At: time.Now().Add(-c.ago), Down: 811, Up: 700, Tool: c.tool}
		_ = Check(context.Background(), f, Options{PlanMbps: 300, LastSpeed: last})
		if got := speedCalls(f) == 0; got != c.reused {
			t.Errorf("%s: reused=%v, want %v", c.comment, got, c.reused)
		}
	}
}

// ND-205: a press inside the hold measures nothing and says so plainly, with how long to wait and the time to come back.
func TestTooSoonMessageGivesTheWaitAndTheTime(t *testing.T) {
	f := healthy()
	last := &SpeedSample{At: time.Now().Add(-1 * time.Minute), Down: 811, Up: 700, Tool: collect.ToolOokla}
	r := Check(context.Background(), f, Options{PlanMbps: 300, LastSpeed: last})
	if speedCalls(f) != 0 {
		t.Fatal("nothing may be measured inside the hold")
	}
	if r.SpeedTool != collect.ToolOokla {
		t.Errorf("the reused reading keeps its tool name, got %q", r.SpeedTool)
	}
	if r.SpeedWaitSeconds < 50 || r.SpeedWaitSeconds > 70 {
		t.Errorf("1 minute of a 2 minute hold is left, got %d seconds", r.SpeedWaitSeconds)
	}
	joined := strings.Join(r.Notes, " ")
	for _, want := range []string{"Too soon to test again", "1 minute ago", "Come back in"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the note should contain %q: %v", want, r.Notes)
		}
	}
	if !strings.Contains(joined, time.Now().Add(time.Minute).Format("15:04")) && !strings.Contains(joined, time.Now().Add(time.Minute+time.Minute).Format("15:04")) {
		t.Errorf("the note should name the time to come back (military time): %v", r.Notes)
	}
}

// ND-206: the saved sample remembers which tool measured it.
func TestSavedSampleRemembersTheTool(t *testing.T) {
	dir := t.TempDir()
	SaveLastSpeed(dir, SpeedSample{At: time.Now(), Down: 1, Up: 2, Tool: collect.ToolOokla})
	got := LoadLastSpeed(dir)
	if got == nil || got.Tool != collect.ToolOokla {
		t.Errorf("the tool name must survive the file: %+v", got)
	}
}
