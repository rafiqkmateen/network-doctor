package app

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/install"
	"github.com/rafiqkmateen/network-doctor/internal/jobs"
)

// ND-275: the Mac and Linux install writes choices.json beside the program, and the existing
// reader reads back the choice made.
func TestInstallWritesTheChoicesFile(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		for _, c := range []install.Choice{{}, {Weekly: true}, {Alerts: true}, {Weekly: true, Alerts: true}} {
			root := t.TempDir()
			home := filepath.Join(root, "home")
			exe := filepath.Join(root, "setup", "netdoctor")
			os.MkdirAll(filepath.Dir(exe), 0o755)
			os.WriteFile(exe, []byte("simulated"), 0o755)
			e := install.Env{GOOS: goos, GOARCH: "amd64", Home: home, Exe: exe, ResultsDir: filepath.Join(home, "r"),
				Getenv:     func(string) string { return "" },
				Run:        func(string, ...string) (string, error) { return "", os.ErrNotExist },
				Apply:      func(string, string, jobs.Plan, jobs.Runner) error { return nil },
				Remove:     func(string, string, jobs.Runner) error { return nil },
				StopHelper: func(string, jobs.Runner) error { return nil },
				Symlink:    func(string, string) error { return nil },
			}
			if r := install.Install(e, c, io.Discard); len(r.Problems) > 0 {
				t.Fatalf("%s %+v: %v", goos, c, r.Problems)
			}
			l := install.LayoutFor(e)
			if filepath.Dir(l.Program) != l.ProgramDir {
				t.Fatal("the program is not in the program folder")
			}
			got, ok := LoadChoices(l.ProgramDir)
			want := Choices{Core: true, Scheduled: c.Weekly, Proactive: c.Alerts}
			if !ok || got != want {
				t.Errorf("%s %+v: read %+v, %v", goos, c, got, ok)
			}
		}
	}
}

// On Windows the install and uninstall commands point to the Windows setup and change nothing.
func TestInstallCommandOnWindows(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("only on Windows: elsewhere these commands install for real")
	}
	for _, cmd := range []string{"install", "uninstall"} {
		var out bytes.Buffer
		if code := Run(context.Background(), []string{cmd, "--silent"}, healthy(), t.TempDir(), &out); code == 0 ||
			!strings.Contains(out.String(), "Windows") {
			t.Errorf("%s: %d %q", cmd, code, out.String())
		}
	}
}
