package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/store"
)

// jobRunner stands in for launchctl, systemctl and crontab, so a test never makes a real job.
type jobRunner struct {
	calls   []string
	systemd bool
	crontab string
}

func (r *jobRunner) run(name string, args ...string) (string, error) {
	r.calls = append(r.calls, name+" "+strings.Join(args, " "))
	fail := errors.New("exit status 1")
	switch name {
	case "id":
		return "501", nil
	case "systemctl":
		if !r.systemd {
			return "Failed to connect to bus", fail
		}
		return "", nil
	case "crontab":
		if args[0] == "-l" {
			if r.crontab == "" {
				return "no crontab for tester", fail
			}
			return r.crontab, nil
		}
		b, _ := os.ReadFile(args[0])
		r.crontab = strings.TrimRight(string(b), "\n")
		return "", nil
	case "launchctl":
		if args[0] == "print" || args[0] == "bootout" {
			return "", fail
		}
		return "", nil
	}
	return "", fail
}

// ND-284: schedule enable, status, disable and status on a pretend Mac and a pretend Linux, with
// and without a choices file. The job is made and removed, the words are plain, and the choices
// file follows only when it exists.
func TestScheduleWorksOnMacAndLinux(t *testing.T) {
	cfg := store.Config{WeeklyDay: 1, WeeklyHour: 9}
	banned := regexp.MustCompile(`(?i)\b(cron|crontab|systemd|systemctl|launchctl|terminal|type)\b`)
	for _, c := range []struct {
		goos    string
		systemd bool
		choices bool
	}{{"darwin", false, true}, {"darwin", false, false}, {"linux", true, true}, {"linux", false, false}} {
		home, prog := t.TempDir(), t.TempDir()
		old := programDir
		programDir = func() string { return prog }
		if c.choices {
			if err := SaveChoices(prog, Choices{Core: true, Proactive: true}); err != nil {
				t.Fatal(err)
			}
		}
		r := &jobRunner{systemd: c.systemd}
		exe := filepath.Join(prog, "netdoctor")
		do := func(cmd string) (string, int) {
			var b bytes.Buffer
			code := scheduleJobs(c.goos, home, exe, []string{cmd}, cfg, r.run, &b)
			if banned.MatchString(b.String()) {
				t.Errorf("%s %s: message names a system tool or terminal word: %q", c.goos, cmd, b.String())
			}
			return b.String(), code
		}
		weeklyThere := func() bool {
			switch {
			case c.goos == "darwin":
				_, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", "com.networkdoctor.weekly.plist"))
				return err == nil
			case c.systemd:
				_, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", "network-doctor-weekly.timer"))
				return err == nil
			}
			return strings.Contains(r.crontab, "# network-doctor-weekly")
		}

		if s, code := do("enable"); code != 0 || s != "Weekly check is on: Monday at 0900.\n" || !weeklyThere() {
			t.Errorf("%+v enable: %d %q there=%v", c, code, s, weeklyThere())
		}
		if ch, found := LoadChoices(prog); found != c.choices || (found && (!ch.Scheduled || !ch.Proactive)) {
			t.Errorf("%+v: choices after enable %+v found=%v", c, ch, found)
		}
		if s, _ := do("status"); s != "Weekly check is on.\n" {
			t.Errorf("%+v status: %q", c, s)
		}
		if s, code := do("disable"); code != 0 || s != "Weekly check is off.\n" || weeklyThere() {
			t.Errorf("%+v disable: %d %q", c, code, s)
		}
		if ch, found := LoadChoices(prog); found && (ch.Scheduled || !ch.Proactive) {
			t.Errorf("%+v: choices after disable %+v", c, ch)
		}
		if s, _ := do("status"); s != "Weekly check is off.\n" {
			t.Errorf("%+v status after: %q", c, s)
		}
		if _, code := do("sometimes"); code != 2 {
			t.Errorf("%+v: an unknown word must be refused", c)
		}
		// The alerts job is left alone by schedule.
		for _, call := range r.calls {
			if strings.Contains(call, "alerts") {
				t.Errorf("%+v: schedule touched the alerts job: %s", c, call)
			}
		}
		programDir = old
	}
}
