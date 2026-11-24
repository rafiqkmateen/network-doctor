package core

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

// dropsAt makes one drop of a minute at each given hour on a different day before now.
func dropsAt(now time.Time, hours ...int) []Event {
	var ev []Event
	for i, h := range hours {
		day := now.AddDate(0, 0, -(i + 1))
		start := time.Date(day.Year(), day.Month(), day.Day(), h, 10, 0, 0, now.Location())
		ev = append(ev, Event{Time: start, Up: false}, Event{Time: start.Add(time.Minute), Up: true})
	}
	return ev
}

// ND-262: the drops pattern also goes to the page as a number, so the page can say the hour in the
// viewer's own time format. The sentence for the terminal stays as it was.
func TestPatternHourIsSentWithThePattern(t *testing.T) {
	now := time.Date(2026, 3, 10, 12, 0, 0, 0, time.Local)
	rep := AnalyzeDrops(dropsAt(now, 4, 4, 4), now)
	if rep.PatternHour == nil || *rep.PatternHour != 4 {
		t.Fatalf("pattern hour should be 4, got %v", rep.PatternHour)
	}
	if !strings.HasPrefix(rep.Pattern, "Drops often start around 0400.") {
		t.Errorf("the terminal sentence must not change: %q", rep.Pattern)
	}
	b, _ := json.Marshal(rep)
	if !strings.Contains(string(b), `"pattern_hour":4`) {
		t.Errorf("the JSON must carry pattern_hour: %s", b)
	}

	// Midnight is a real hour, so it must still be sent.
	mid := AnalyzeDrops(dropsAt(now, 0, 0, 0), now)
	b, _ = json.Marshal(mid)
	if mid.PatternHour == nil || *mid.PatternHour != 0 || !strings.Contains(string(b), `"pattern_hour":0`) {
		t.Errorf("hour 0 must be sent: %v %s", mid.PatternHour, b)
	}

	// No pattern: no hour at all, so the page never makes one up.
	none := AnalyzeDrops(dropsAt(now, 4, 9), now)
	b, _ = json.Marshal(none)
	if none.PatternHour != nil || strings.Contains(string(b), "pattern_hour") {
		t.Errorf("no pattern must mean no pattern_hour: %s", b)
	}
}
