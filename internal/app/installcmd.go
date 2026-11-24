package app

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rafiqkmateen/network-doctor/internal/install"
)

// installCmd runs `install` or `uninstall` (Mac and Linux, ND-271, ND-290). Results stay in dir,
// the folder the rest of the program uses, and the page opens the same way the open page does.
func installCmd(ctx context.Context, cmd string, args []string, dir string, out io.Writer) int {
	exe, err := os.Executable()
	if err == nil {
		if real, rerr := filepath.EvalSymlinks(exe); rerr == nil {
			exe = real
		}
	}
	home, _ := os.UserHomeDir()
	e := install.Env{GOOS: runtime.GOOS, GOARCH: runtime.GOARCH, Home: home, Exe: exe, ResultsDir: dir,
		Version: Version, OpenURL: openBrowser, NoBrowser: noBrowser, Fixes: newFixSystem()}
	return install.Main(ctx, cmd, args, e, out)
}
