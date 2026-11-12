package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/store"
)

func loadSinks(t *testing.T, dir string) []string {
	t.Helper()
	cfg, err := store.LoadConfig(filepath.Join(dir, "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	return cfg.Sinks
}

// ND-150: one command switches the alerts file on or off and leaves the other places alone.
func TestAlertsFileSwitch(t *testing.T) {
	dir := t.TempDir()
	var out bytes.Buffer
	run := func(args ...string) int {
		out.Reset()
		return Run(context.Background(), args, healthy(), dir, &out)
	}
	if code := run("config", "sinks", "console,screen"); code != 0 {
		t.Fatal(out.String())
	}
	if code := run("config", "alerts-file", "on"); code != 0 {
		t.Fatal(out.String())
	}
	if got := strings.Join(loadSinks(t, dir), ","); got != "console,screen,file" {
		t.Fatalf("after on: %s", got)
	}
	run("config", "alerts-file", "on") // twice must not add it twice
	if got := strings.Join(loadSinks(t, dir), ","); got != "console,screen,file" {
		t.Fatalf("after second on: %s", got)
	}
	if code := run("config", "alerts-file", "off"); code != 0 {
		t.Fatal(out.String())
	}
	if got := strings.Join(loadSinks(t, dir), ","); got != "console,screen" {
		t.Fatalf("after off: %s", got)
	}
	if code := run("config", "alerts-file", "maybe"); code == 0 {
		t.Fatal("a word that is not on or off should be refused")
	}
}

// ND-151 and ND-152: one line per alert (time, kind, message), beside the other results;
// nothing is written when the file place is off.
func TestAlertsFileLines(t *testing.T) {
	dir := t.TempDir()
	at := time.Date(2026, 11, 11, 20, 0, 0, 0, time.UTC)
	a := Alert{Kind: core.AlertDrop, Message: "The connection dropped.\nSecond\tline", Time: at}

	var console bytes.Buffer
	Emit(a, []core.Sink{core.SinkConsole}, dir, &console)
	if _, err := os.Stat(filepath.Join(dir, "alerts.log")); err == nil {
		t.Fatal("the file must not exist when its place is off")
	}

	Emit(a, []core.Sink{core.SinkFile}, dir, &console)
	Emit(Alert{Kind: core.AlertSlow, Message: "Slow.", Time: at.Add(time.Hour)}, []core.Sink{core.SinkFile}, dir, &console)
	b, err := os.ReadFile(filepath.Join(dir, "alerts.log"))
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(b)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want one line per alert, got %d: %q", len(lines), b)
	}
	f := strings.Split(lines[0], "\t")
	if len(f) != 3 || f[0] != "2026-11-11T20:00:00Z" || f[1] != string(core.AlertDrop) || f[2] != "The connection dropped. Second line" {
		t.Fatalf("line should be time, kind, message with no stray tabs or breaks: %q", lines[0])
	}
}
