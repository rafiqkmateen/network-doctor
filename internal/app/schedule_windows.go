//go:build windows

package app

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/rafiqkmateen/network-doctor/internal/store"
)

const taskName = windowsWeeklyTask

var days = map[int]string{0: "SUN", 1: "MON", 2: "TUE", 3: "WED", 4: "THU", 5: "FRI", 6: "SAT"}

// scheduleCmd turns the weekly check on or off. The user asks for it by name;
// the tool never schedules anything by itself.
func scheduleCmd(args []string, cfg store.Config, out io.Writer) int {
	if len(args) != 1 {
		fmt.Fprintln(out, "Use: schedule enable | disable | status")
		return 2
	}
	run := func(a ...string) (string, error) {
		c := exec.Command("schtasks", a...)
		c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		b, err := c.CombinedOutput()
		return strings.TrimSpace(string(b)), err
	}
	switch args[0] {
	case "enable":
		exe, err := os.Executable()
		if err != nil {
			fmt.Fprintln(out, "Could not find this program:", err)
			return 2
		}
		tr := fmt.Sprintf(`"%s" weekly`, exe)
		o, err := run("/Create", "/TN", taskName, "/TR", tr, "/SC", "WEEKLY", "/D", days[int(cfg.WeeklyDay)], "/ST", fmt.Sprintf("%02d:00", cfg.WeeklyHour), "/F")
		if err != nil {
			fmt.Fprintln(out, "Could not turn it on:", o)
			return 2
		}
		// schtasks has no switch for "run as soon as possible after a missed start" (ND-289).
		ps := exec.Command("powershell.exe", "-NoProfile", "-NonInteractive", "-Command", windowsTaskSettingsScript(windowsWeeklyTask, false))
		ps.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
		if ps.Run() != nil {
			fmt.Fprintln(out, "Note: a weekly check missed while the computer was off may wait until the next week.")
		}
		syncScheduled(true)
		fmt.Fprintf(out, "Weekly check is on: %s at %02d00.\n", days[int(cfg.WeeklyDay)], cfg.WeeklyHour)
	case "disable":
		if o, err := run("/Delete", "/TN", taskName, "/F"); err != nil {
			fmt.Fprintln(out, "Could not turn it off:", o)
			return 2
		}
		syncScheduled(false)
		fmt.Fprintln(out, "Weekly check is off.")
	case "status":
		if o, err := run("/Query", "/TN", taskName); err != nil {
			fmt.Fprintln(out, "Weekly check is off.")
		} else {
			fmt.Fprintln(out, "Weekly check is on.")
			_ = o
		}
	default:
		fmt.Fprintln(out, "Use: schedule enable | disable | status")
		return 2
	}
	return 0
}
