package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/store"
)

// The proof switch: with NETDOCTOR_TEST_ALERT=1, watch raises exactly one alert of kind "test"
// with the plain test message, through the normal places, once, right after it starts. Unset, or
// set to anything else, it raises none.
func TestWatchRaisesOneTestAlertWhenAsked(t *testing.T) {
	old := screenAlert
	t.Cleanup(func() { screenAlert = old })
	for _, c := range []struct {
		env  string
		want int
	}{{"1", 1}, {"", 0}, {"yes", 0}} {
		t.Setenv("NETDOCTOR_TEST_ALERT", c.env)
		var shown []string
		screenAlert = func(title, msg string) error { shown = append(shown, msg); return nil }
		dir := t.TempDir()
		src := healthy()
		src.devices = nil // no new device, so only the test alert can be raised
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		var out bytes.Buffer
		cfg := store.Config{Sinks: []string{"console", "file", "screen"}, SinksSaved: true}
		if code := watch(ctx, src, dir, cfg, 0, 60, nil, &out); code != 0 {
			t.Fatalf("env %q: code %d", c.env, code)
		}
		b, _ := os.ReadFile(filepath.Join(dir, "alerts.log"))
		lines := strings.Count(string(b), "\ttest\t"+testAlertMessage+"\n")
		if lines != c.want || strings.Count(string(b), "\n") != c.want {
			t.Errorf("env %q: alerts file %q, want %d test line", c.env, b, c.want)
		}
		if strings.Count(out.String(), testAlertMessage) != c.want || len(shown) != c.want {
			t.Errorf("env %q: console %q screen %v", c.env, out.String(), shown)
		}
	}
	if testAlertMessage != "This is a test alert from Network Doctor." {
		t.Errorf("wording: %q", testAlertMessage)
	}
}
