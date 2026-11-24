//go:build darwin || linux

package app

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/rafiqkmateen/network-doctor/internal/jobs"
	"github.com/rafiqkmateen/network-doctor/internal/store"
)

// scheduleCmd turns the weekly check on or off on a Mac or Linux computer (ND-284). The person
// asks for it by name; the tool never schedules anything by itself.
func scheduleCmd(args []string, cfg store.Config, out io.Writer) int {
	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Fprintln(out, "Could not find your home folder.")
		return 2
	}
	exe, err := os.Executable()
	if err != nil {
		fmt.Fprintln(out, "Could not find this program:", err)
		return 2
	}
	if real, err := filepath.EvalSymlinks(exe); err == nil {
		exe = real
	}
	return scheduleJobs(runtime.GOOS, home, exe, args, cfg, jobs.ExecRunner, out)
}
