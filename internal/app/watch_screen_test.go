package app

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/notify"
	"github.com/rafiqkmateen/network-doctor/internal/store"
)

// ND-288: with the screen helper missing, then failing, and the file place on, the watcher keeps
// going, every alert reaches the file, and the log says the plain sentence once, not each time.
func TestMissingNotifierKeepsWatching(t *testing.T) {
	old := screenAlert
	t.Cleanup(func() { screenAlert = old })
	for name, fail := range map[string]error{
		"missing": fmt.Errorf("%w (notify-send)", notify.ErrNoHelper),
		"failing": errors.New("osascript did not show the alert: exit status 1"),
	} {
		calls := 0
		screenAlert = func(title, msg string) error { calls++; return fail }
		dir := t.TempDir()
		src := healthy()
		src.devices = []core.Device{
			{Name: "a", MAC: "AA:AA:AA:AA:AA:01", IP: "192.168.1.31"},
			{Name: "b", MAC: "AA:AA:AA:AA:AA:02", IP: "192.168.1.32"},
			{Name: "c", MAC: "AA:AA:AA:AA:AA:03", IP: "192.168.1.33"},
		}
		cfg := store.Config{Sinks: []string{"file", "screen"}, SinksSaved: true}
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // one full round of checks, then the watcher ends as if stopped
		var out bytes.Buffer
		if code := watch(ctx, src, dir, cfg, 0, 60, nil, &out); code != 0 {
			t.Fatalf("%s: the watcher must keep going and end normally, got %d", name, code)
		}
		if calls != 3 {
			t.Errorf("%s: the screen was tried %d times, want 3", name, calls)
		}
		b, err := os.ReadFile(filepath.Join(dir, "alerts.log"))
		if err != nil || strings.Count(string(b), "\n") != 3 {
			t.Errorf("%s: three alerts must be in the file: %q %v", name, b, err)
		}
		if n := strings.Count(out.String(), screenFailSaved); n != 1 {
			t.Errorf("%s: the plain sentence must be in the log once, found %d:\n%s", name, n, out.String())
		}
	}
	// Without the file place on, the sentence does not promise a file.
	screenAlert = func(string, string) error { return notify.ErrNoHelper }
	if p := Emit(Alert{Message: "x"}, []core.Sink{core.SinkScreen}, t.TempDir(), &bytes.Buffer{}); len(p) != 1 || p[0] != screenFailNoFile {
		t.Errorf("screen only: %q", p)
	}
}

// ND-05: the two sentences read at grade 8 or lower and hold no system or terminal words (ND-280).
func TestScreenFailSentencesArePlain(t *testing.T) {
	for _, s := range []string{screenFailSaved, screenFailNoFile, testAlertMessage} {
		if g := core.GradeLevel(s); g > 8.0 {
			t.Errorf("grade %.1f too high: %q", g, s)
		}
		low := strings.ToLower(s)
		for _, w := range []string{"type", "terminal", "command", "cron", "sudo", "systemd", "notify-send", "osascript"} {
			if strings.Contains(low, w) {
				t.Errorf("%q holds %q", s, w)
			}
		}
	}
}
