package collect

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

// The two tools that can measure the speed.
const (
	ToolOokla      = "Ookla Speedtest"
	ToolCloudflare = "Cloudflare"
)

// ooklaFile is the name of Ookla's command-line program on this system.
var ooklaFile = func() string {
	if runtime.GOOS == "windows" {
		return "speedtest.exe"
	}
	return "speedtest"
}()

// ooklaArgs asks Ookla's tool for one machine-readable result and nothing else.
// It never carries the license switches: only the person accepts Ookla's license,
// by running the tool once themselves. Without that acceptance the tool stops and
// the Cloudflare test is used instead.
var ooklaArgs = []string{"-f", "json", "--progress=no"}

// findOokla returns the path of Ookla's tool, or "" when the person has not put it
// anywhere we look. Order: the NETDOCTOR_OOKLA setting, an ookla folder beside this
// program, then the program path. The tool is never shipped with Network Doctor.
func findOokla(self string) string {
	if named := strings.TrimSpace(os.Getenv("NETDOCTOR_OOKLA")); named != "" {
		if isFile(named) {
			return named
		}
		return ""
	}
	if self != "" {
		beside := filepath.Join(filepath.Dir(self), "ookla", ooklaFile)
		if isFile(beside) {
			return beside
		}
	}
	if p, err := exec.LookPath("speedtest"); err == nil {
		return p
	}
	return ""
}

func isFile(p string) bool {
	st, err := os.Stat(p)
	return err == nil && !st.IsDir()
}

// runOokla runs Ookla's tool once and returns what it printed. It has no input, so
// a license question it cannot ask ends the run with an error.
func runOokla(ctx context.Context, exe string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, exe, ooklaArgs...)
	hide(cmd)
	return cmd.Output()
}

// parseOoklaResult reads the result line from Ookla's output and gives download and
// upload in megabits per second. Ookla reports bytes per second.
func parseOoklaResult(out []byte) (down, up float64, err error) {
	type band struct {
		Bandwidth float64 `json:"bandwidth"`
	}
	sc := bufio.NewScanner(bytes.NewReader(out))
	sc.Buffer(make([]byte, 0, 64*1024), 1<<20)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var r struct {
			Type     string `json:"type"`
			Download band   `json:"download"`
			Upload   band   `json:"upload"`
		}
		if json.Unmarshal([]byte(line), &r) != nil || r.Type != "result" {
			continue
		}
		if r.Download.Bandwidth > 0 && r.Upload.Bandwidth > 0 {
			return r.Download.Bandwidth * 8 / 1e6, r.Upload.Bandwidth * 8 / 1e6, nil
		}
	}
	return 0, 0, errors.New("no usable result from Ookla's tool")
}

// measureSpeed uses Ookla's tool when it is there and works, and the Cloudflare
// test otherwise. It says which one measured.
func measureSpeed(ctx context.Context, ooklaExe string, run func(context.Context, string) ([]byte, error), base string, plan speedPlan) (down, up float64, tool string, err error) {
	if ooklaExe != "" {
		if out, rerr := run(ctx, ooklaExe); rerr == nil {
			if d, u, perr := parseOoklaResult(out); perr == nil {
				return d, u, ToolOokla, nil
			}
		}
	}
	down, up, err = runSpeed(ctx, base, plan)
	return down, up, ToolCloudflare, err
}

type lightKey struct{}

// LightSpeed marks a context so the speed test stays light: Ookla's tool moves
// gigabytes a test, so the repeating alerts helper uses the small Cloudflare test.
func LightSpeed(ctx context.Context) context.Context {
	return context.WithValue(ctx, lightKey{}, true)
}

func isLight(ctx context.Context) bool {
	v, _ := ctx.Value(lightKey{}).(bool)
	return v
}

var lastTool atomic.Value // string

// SpeedToolUsed names the tool that measured the most recent speed test in this run.
func SpeedToolUsed() string {
	s, _ := lastTool.Load().(string)
	return s
}
