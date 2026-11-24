package app

import (
	"fmt"
	"io"

	"github.com/rafiqkmateen/network-doctor/internal/jobs"
	"github.com/rafiqkmateen/network-doctor/internal/store"
)

// scheduleJobs is `schedule enable | disable | status` on Mac and Linux (ND-284). It makes and
// removes the same weekly job the install makes, through the jobs package, and keeps the choices
// file in step (ND-208). The words name no system tool. It has no build tag, so the tests run it
// as a pretend Mac and a pretend Linux on any computer.
func scheduleJobs(goos, home, exe string, args []string, cfg store.Config, run jobs.Runner, out io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(out, "Use: schedule enable | disable | status")
		return 2
	}
	p := jobs.Plan{Exe: exe, Weekday: int(cfg.WeeklyDay), Hour: cfg.WeeklyHour, Custom: true}
	switch args[0] {
	case "enable":
		p.Weekly = true
		if err := jobs.SetWeekly(goos, home, p, run); err != nil {
			fmt.Fprintln(out, "Could not turn on the weekly check.")
			return 2
		}
		syncScheduledIn(programDir(), true)
		fmt.Fprintf(out, "Weekly check is on: %s at %02d00.\n", cfg.WeeklyDay, cfg.WeeklyHour)
	case "disable":
		if err := jobs.SetWeekly(goos, home, p, run); err != nil {
			fmt.Fprintln(out, "Could not turn off the weekly check.")
			return 2
		}
		syncScheduledIn(programDir(), false)
		fmt.Fprintln(out, "Weekly check is off.")
	case "status":
		if jobs.WeeklyStatus(goos, home, run) {
			fmt.Fprintln(out, "Weekly check is on.")
		} else {
			fmt.Fprintln(out, "Weekly check is off.")
		}
	default:
		fmt.Fprintln(out, "Use: schedule enable | disable | status")
		return 2
	}
	return 0
}
