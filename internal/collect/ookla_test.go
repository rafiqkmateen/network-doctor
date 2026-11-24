package collect

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

// A result line as Ookla's command-line tool prints it with -f json (bandwidth is bytes per second).
const ooklaResultLine = `{"type":"result","timestamp":"2026-11-12T14:35:00Z","ping":{"jitter":0.1,"latency":7.9},"download":{"bandwidth":110000000,"bytes":1600000000,"elapsed":15000},"upload":{"bandwidth":100000000,"bytes":850000000,"elapsed":8000},"packetLoss":0,"isp":"Example","server":{"id":76723,"name":"Example"},"result":{"id":"abc","url":"https://www.speedtest.net/result/c/abc"}}`

// ND-200: Ookla's result becomes megabits per second, read from the result line even among other lines.
func TestParseOoklaResult(t *testing.T) {
	out := []byte(`{"type":"log","message":"hello"}` + "\n" + ooklaResultLine + "\n")
	down, up, err := parseOoklaResult(out)
	if err != nil {
		t.Fatal(err)
	}
	if down < 879 || down > 881 || up < 799 || up > 801 {
		t.Errorf("110000000 and 100000000 bytes a second are 880 and 800 megabits, got %.1f and %.1f", down, up)
	}
	for _, bad := range []string{"", "not json", `{"type":"log","message":"no result here"}`, `{"type":"result","download":{"bandwidth":0},"upload":{"bandwidth":0}}`} {
		if _, _, err := parseOoklaResult([]byte(bad)); err == nil {
			t.Errorf("%q has no usable result and must be an error", bad)
		}
	}
}

// ND-201: Ookla's tool is found only where the person put it: the setting, then a folder beside the program, then the path.
func TestFindOoklaOnlyWherePutByThePerson(t *testing.T) {
	dir := t.TempDir()
	exe := filepath.Join(dir, "speedtest.exe")
	if err := os.WriteFile(exe, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("NETDOCTOR_OOKLA", exe)
	if got := findOokla(""); got != exe {
		t.Errorf("the setting names the tool, got %q", got)
	}
	t.Setenv("NETDOCTOR_OOKLA", filepath.Join(dir, "missing.exe"))
	if got := findOokla(""); got != "" {
		t.Errorf("a setting that points nowhere finds nothing, got %q", got)
	}
	t.Setenv("NETDOCTOR_OOKLA", "")
	beside := t.TempDir()
	if err := os.MkdirAll(filepath.Join(beside, "ookla"), 0o755); err != nil {
		t.Fatal(err)
	}
	inFolder := filepath.Join(beside, "ookla", ooklaFile)
	if err := os.WriteFile(inFolder, []byte("x"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findOokla(filepath.Join(beside, "netdoctor.exe")); got != inFolder {
		t.Errorf("an ookla folder beside the program is found, got %q", got)
	}
	t.Setenv("PATH", "")
	if got := findOokla(filepath.Join(t.TempDir(), "netdoctor.exe")); got != "" {
		t.Errorf("nothing anywhere means no Ookla tool, got %q", got)
	}
}

// ND-202: when Ookla's tool is there and works its numbers are used and named; if it fails, the Cloudflare test runs and is named instead.
func TestSpeedUsesOoklaThenFallsBackToCloudflare(t *testing.T) {
	url, _, _ := fakeSpeedServer(t)
	plan := speedPlan{Streams: 2, Warmup: 50 * time.Millisecond, Measure: 300 * time.Millisecond}
	works := func(ctx context.Context, exe string) ([]byte, error) { return []byte(ooklaResultLine), nil }
	fails := func(ctx context.Context, exe string) ([]byte, error) { return nil, errors.New("could not start") }

	down, up, tool, err := measureSpeed(context.Background(), "tool.exe", works, url, plan)
	if err != nil || tool != ToolOokla || down < 879 || up < 799 {
		t.Errorf("Ookla should answer: %.0f %.0f %q %v", down, up, tool, err)
	}
	down, up, tool, err = measureSpeed(context.Background(), "tool.exe", fails, url, plan)
	if err != nil || tool != ToolCloudflare || down <= 0 || up <= 0 {
		t.Errorf("a failing Ookla tool falls back to Cloudflare: %.0f %.0f %q %v", down, up, tool, err)
	}
	_, _, tool, err = measureSpeed(context.Background(), "", fails, url, plan)
	if err != nil || tool != ToolCloudflare {
		t.Errorf("no Ookla tool means Cloudflare: %q %v", tool, err)
	}
}

// ND-203: Ookla's tool is never asked to accept its license for the person; without a stored acceptance it fails and Cloudflare is used.
func TestOoklaCommandNeverAcceptsTheLicense(t *testing.T) {
	for _, a := range ooklaArgs {
		if a == "--accept-license" || a == "--accept-gdpr" {
			t.Errorf("argument %s would accept Ookla's license for the person", a)
		}
	}
}
