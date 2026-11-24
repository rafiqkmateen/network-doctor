package jobs

import (
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// simulated is a stand-in for the system's own tools. It records every call and answers like a
// Mac or a Linux computer would, so no real job is ever made by a test.
type simulated struct {
	calls     [][]string
	systemd   bool   // user systemd answers
	crontab   string // the person's crontab now ("" means none)
	noCrontab bool   // the crontab program is missing
	loaded    map[string]bool
	bootFails int  // the next bootstraps that fail, as launchd does while an old job is going
	linger    int  // print calls that still find a job after bootout
	active    bool // the user alerts service runs
	pgrep     bool // a helper started directly runs
}

func newSimulated() *simulated { return &simulated{loaded: map[string]bool{}} }

// noWait makes the short waits between tries instant for one test, and counts them.
func noWait(t *testing.T) *int {
	n := 0
	old := sleep
	sleep = func(time.Duration) { n++ }
	t.Cleanup(func() { sleep = old })
	return &n
}

func (p *simulated) run(name string, args ...string) (string, error) {
	p.calls = append(p.calls, append([]string{name}, args...))
	fail := errors.New("exit status 1")
	switch name {
	case "id":
		return "501", nil
	case "launchctl":
		switch args[0] {
		case "print":
			if p.loaded[args[1]] {
				return "state = running", nil
			}
			if p.linger > 0 {
				p.linger--
				return "state = not running", nil
			}
			return "Could not find service", fail
		case "bootstrap":
			target := args[1] + "/" + strings.TrimSuffix(filepath.Base(args[2]), ".plist")
			if p.bootFails > 0 || p.loaded[target] || p.linger > 0 {
				if p.bootFails > 0 {
					p.bootFails--
				}
				return "Bootstrap failed: 5: Input/output error", fail
			}
			p.loaded[target] = true
		case "bootout":
			if !p.loaded[args[1]] {
				return "No such process", fail
			}
			delete(p.loaded, args[1])
		}
		return "", nil
	case "systemctl":
		if !p.systemd {
			return "Failed to connect to bus: No medium found", fail
		}
		if args[1] == "is-active" && !p.active {
			return "inactive", fail
		}
		if args[1] == "start" && args[2] == "network-doctor-alerts.service" {
			p.active = true
		}
		return "", nil
	case "pgrep":
		if !p.pgrep {
			return "", fail
		}
		return "4242", nil
	case "sh":
		p.pgrep = true // the helper was started directly
		return "", nil
	case "crontab":
		if p.noCrontab {
			return "crontab: not found", fail
		}
		if args[0] == "-l" {
			if p.crontab == "" {
				return "no crontab for tester", fail
			}
			return p.crontab, nil
		}
		b, err := os.ReadFile(args[0]) // read while the call runs: the file is deleted after
		if err != nil {
			return err.Error(), fail
		}
		p.crontab = strings.TrimRight(string(b), "\n")
		return "", nil
	case "pkill":
		return "", fail // nothing was running
	}
	return "unexpected " + name, fail
}

func (p *simulated) asked(words ...string) bool {
	want := strings.Join(words, " ")
	for _, c := range p.calls {
		if strings.Contains(strings.Join(c, " "), want) {
			return true
		}
	}
	return false
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("expected %s: %v", path, err)
	}
	return string(b)
}

func gone(t *testing.T, path string) {
	t.Helper()
	if _, err := os.Stat(path); err == nil {
		t.Errorf("%s should not exist", path)
	}
}

const exe = "/Users/tester/Applications/Network Doctor.app/Contents/MacOS/netdoctor"

// ND-282: the Mac weekly LaunchAgent, Monday 0900, loaded at once; gone when not chosen.
func TestMacWeeklyAgent(t *testing.T) {
	home, p := t.TempDir(), newSimulated()
	if err := Apply("darwin", home, Plan{Exe: exe, Weekly: true, Weekday: 1, Hour: 9}, p.run); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "Library", "LaunchAgents", "com.networkdoctor.weekly.plist")
	s := read(t, path)
	for _, want := range []string{
		"<key>Label</key>", "<string>com.networkdoctor.weekly</string>",
		"<string>" + exe + "</string>", "<string>weekly</string>",
		"<key>StartCalendarInterval</key>", "<key>Weekday</key>", "<integer>1</integer>",
		"<key>Hour</key>", "<integer>9</integer>", "<key>Minute</key>", "<integer>0</integer>",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("weekly plist lacks %q:\n%s", want, s)
		}
	}
	if !p.asked("launchctl", "bootstrap", "gui/501", path) {
		t.Errorf("the job must be loaded at once: %v", p.calls)
	}
	if w, a := Status("darwin", home, p.run); !w || a {
		t.Errorf("status = %v %v, want weekly only", w, a)
	}
	// Running it again repairs: the old job is taken out first, so loading never fails (ND-292).
	if err := Apply("darwin", home, Plan{Exe: exe, Weekly: true, Weekday: 1, Hour: 9}, p.run); err != nil {
		t.Fatalf("second run: %v", err)
	}
	if !p.asked("launchctl", "bootout", "gui/501/com.networkdoctor.weekly") {
		t.Error("a second run must unload the old job before loading the new one")
	}
	if err := Apply("darwin", home, Plan{Exe: exe}, p.run); err != nil {
		t.Fatal(err)
	}
	gone(t, path)
	if p.loaded["gui/501/com.networkdoctor.weekly"] {
		t.Error("the job must be unloaded when not chosen")
	}
}

// ND-285: the Mac alerts LaunchAgent starts at log-in and is started again if it stops.
func TestMacAlertsAgent(t *testing.T) {
	home, p := t.TempDir(), newSimulated()
	if err := Apply("darwin", home, Plan{Exe: exe, Alerts: true}, p.run); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(home, "Library", "LaunchAgents", "com.networkdoctor.alerts.plist")
	s := read(t, path)
	for _, want := range []string{"<string>com.networkdoctor.alerts</string>", "<string>watch</string>",
		"<key>RunAtLoad</key>\n\t<true/>", "<key>KeepAlive</key>\n\t<true/>"} {
		if !strings.Contains(s, want) {
			t.Errorf("alerts plist lacks %q:\n%s", want, s)
		}
	}
	if _, err := os.Stat(filepath.Join(home, "Library", "LaunchAgents", "com.networkdoctor.weekly.plist")); err == nil {
		t.Error("the weekly job was not chosen")
	}
	if w, a := Status("darwin", home, p.run); w || !a {
		t.Errorf("status = %v %v, want alerts only", w, a)
	}
	p.loaded["gui/501/com.networkdoctor.alerts"] = true // as after the next log-in
	if err := Apply("darwin", home, Plan{Exe: exe}, p.run); err != nil {
		t.Fatal(err)
	}
	gone(t, path)
	if p.loaded["gui/501/com.networkdoctor.alerts"] {
		t.Error("a running helper must be stopped when alerts are not chosen")
	}
}

// ND-283: with user systemd, a service and a timer, Monday 0900, catching up after a missed run.
func TestLinuxWeeklyTimer(t *testing.T) {
	home, p := t.TempDir(), newSimulated()
	p.systemd = true
	p.crontab = "MAILTO=me\n30 2 * * * /usr/bin/backup # network-doctor-weekly-like but not ours\n0 9 * * 1 '/old/netdoctor' weekly # network-doctor-weekly"
	if err := Apply("linux", home, Plan{Exe: "/home/tester/.local/share/NetworkDoctor/netdoctor", Weekly: true, Weekday: 1, Hour: 9}, p.run); err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(home, ".config", "systemd", "user")
	svc := read(t, filepath.Join(dir, "network-doctor-weekly.service"))
	tmr := read(t, filepath.Join(dir, "network-doctor-weekly.timer"))
	if !strings.Contains(svc, `ExecStart="/home/tester/.local/share/NetworkDoctor/netdoctor" weekly`) || !strings.Contains(svc, "Type=oneshot") {
		t.Errorf("service:\n%s", svc)
	}
	for _, want := range []string{"OnCalendar=Mon *-*-* 09:00:00", "Persistent=true", "WantedBy=timers.target"} {
		if !strings.Contains(tmr, want) {
			t.Errorf("timer lacks %q:\n%s", want, tmr)
		}
	}
	if !p.asked("systemctl", "--user", "daemon-reload") || !p.asked("systemctl", "--user", "enable", "--now", "network-doctor-weekly.timer") {
		t.Errorf("the timer must be enabled: %v", p.calls)
	}
	// An old marked line from a computer that had no timer before is taken out; the others stay.
	if strings.Contains(p.crontab, "/old/netdoctor") || !strings.Contains(p.crontab, "MAILTO=me") || !strings.Contains(p.crontab, "/usr/bin/backup") {
		t.Errorf("crontab after:\n%s", p.crontab)
	}
	if w, _ := Status("linux", home, p.run); !w {
		t.Error("status should say the weekly check is on")
	}
	if err := Apply("linux", home, Plan{Exe: "/x"}, p.run); err != nil {
		t.Fatal(err)
	}
	gone(t, filepath.Join(dir, "network-doctor-weekly.service"))
	gone(t, filepath.Join(dir, "network-doctor-weekly.timer"))
	if !p.asked("systemctl", "--user", "disable", "--now", "network-doctor-weekly.timer") {
		t.Error("the timer must be stopped and disabled")
	}
	if w, _ := Status("linux", home, p.run); w {
		t.Error("status should say the weekly check is off")
	}
}

// ND-283: without user systemd, one marked crontab line; every other line kept as it was.
func TestLinuxWeeklyCronFallback(t *testing.T) {
	home, p := t.TempDir(), newSimulated()
	p.crontab = "# my own note\n15 * * * * /usr/bin/thing --flag"
	if err := Apply("linux", home, Plan{Exe: "/opt/net doctor/netdoctor", Weekly: true, Weekday: 1, Hour: 9}, p.run); err != nil {
		t.Fatal(err)
	}
	want := "# my own note\n15 * * * * /usr/bin/thing --flag\n0 9 * * 1 '/opt/net doctor/netdoctor' weekly # network-doctor-weekly"
	if p.crontab != want {
		t.Errorf("crontab:\n%s\nwant:\n%s", p.crontab, want)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "systemd", "user", "network-doctor-weekly.timer")); err == nil {
		t.Error("no timer without user systemd")
	}
	if files, _ := filepath.Glob(filepath.Join(home, ".network-doctor-crontab*")); len(files) != 0 {
		t.Errorf("the scratch crontab file must be removed: %v", files)
	}
	// Twice is still one line.
	if err := Apply("linux", home, Plan{Exe: "/opt/net doctor/netdoctor", Weekly: true}, p.run); err != nil {
		t.Fatal(err)
	}
	if strings.Count(p.crontab, "# network-doctor-weekly") != 1 {
		t.Errorf("one marked line expected:\n%s", p.crontab)
	}
	if w, _ := Status("linux", home, p.run); !w {
		t.Error("status should read the marked line")
	}
	if err := Apply("linux", home, Plan{}, p.run); err != nil {
		t.Fatal(err)
	}
	if p.crontab != "# my own note\n15 * * * * /usr/bin/thing --flag" {
		t.Errorf("only the marked line may go:\n%s", p.crontab)
	}
	// A person with no crontab at all gets one line, and nothing is written when there is nothing to take out.
	home2, p2 := t.TempDir(), newSimulated()
	if err := Apply("linux", home2, Plan{}, p2.run); err != nil {
		t.Fatal(err)
	}
	for _, c := range p2.calls {
		if c[0] == "crontab" && c[1] != "-l" {
			t.Errorf("nothing to remove, yet the crontab was written: %v", c)
		}
	}
	p2.noCrontab = true
	if err := Apply("linux", home2, Plan{Exe: "/x", Weekly: true}, p2.run); err == nil {
		t.Error("with no timer and no crontab the weekly check cannot be set up, and must say so")
	}
}

// ND-286: the Linux alerts autostart entry, through a user service that restarts, or direct.
func TestLinuxAlertsAutostart(t *testing.T) {
	const lx = "/home/tester/.local/share/NetworkDoctor/netdoctor"
	desk := func(h string) string {
		return filepath.Join(h, ".config", "autostart", "network-doctor-alerts.desktop")
	}
	unit := func(h string) string {
		return filepath.Join(h, ".config", "systemd", "user", "network-doctor-alerts.service")
	}

	home, p := t.TempDir(), newSimulated()
	p.systemd = true
	if err := Apply("linux", home, Plan{Exe: lx, Alerts: true}, p.run); err != nil {
		t.Fatal(err)
	}
	d, u := read(t, desk(home)), read(t, unit(home))
	if !strings.Contains(d, "systemctl --user start network-doctor-alerts.service") || strings.Contains(d, lx) {
		t.Errorf("with systemd the entry starts the service:\n%s", d)
	}
	for _, want := range []string{`ExecStart="` + lx + `" watch`, "Restart=on-failure", "RestartSec=60"} {
		if !strings.Contains(u, want) {
			t.Errorf("service lacks %q:\n%s", want, u)
		}
	}
	for _, want := range []string{"[Desktop Entry]", "Type=Application", "Terminal=false", "X-GNOME-Autostart-enabled=true"} {
		if !strings.Contains(d, want) {
			t.Errorf("entry lacks %q:\n%s", want, d)
		}
	}
	if _, a := Status("linux", home, p.run); !a {
		t.Error("status should say alerts are on")
	}
	if err := Apply("linux", home, Plan{Exe: lx}, p.run); err != nil {
		t.Fatal(err)
	}
	gone(t, desk(home))
	gone(t, unit(home))
	if !p.asked("systemctl", "--user", "stop", "network-doctor-alerts.service") {
		t.Error("the helper must be stopped when alerts are not chosen")
	}

	home2, p2 := t.TempDir(), newSimulated()
	if err := Apply("linux", home2, Plan{Exe: lx, Alerts: true}, p2.run); err != nil {
		t.Fatal(err)
	}
	d2 := read(t, desk(home2))
	if !strings.Contains(d2, `Exec="`+lx+`" watch`) {
		t.Errorf("without systemd the entry starts the helper directly:\n%s", d2)
	}
	gone(t, unit(home2))
	if err := Apply("linux", home2, Plan{}, p2.run); err != nil {
		t.Fatal(err)
	}
	gone(t, desk(home2))
}

// Paths with spaces, quotes and other special signs are written so each system reads back the
// same path, and nothing in them can start another program.
func TestExePathsAreEscaped(t *testing.T) {
	odd := `/home/t e/a"b'c$d%e` + "`f" + `\g/netdoctor`
	if got := plistString(odd); got != `/home/t e/a&#34;b&#39;c$d%e`+"`f"+`\g/netdoctor` {
		t.Errorf("plist: %s", got)
	}
	if got := systemdQuote(odd); got != `"/home/t e/a\"b'c$$d%%e`+"`f"+`\\g/netdoctor"` {
		t.Errorf("systemd: %s", got)
	}
	// Desktop entry: quoted with " ` $ \ escaped by a backslash, then every backslash doubled
	// for the string level, and % doubled.
	if got := desktopQuote(odd); got != `"/home/t e/a\\"b'c\\$d%%e\\`+"`f"+`\\\\g/netdoctor"` {
		t.Errorf("desktop: %s", got)
	}
	if got := cronQuote(odd); got != `'/home/t e/a"b'\''c$d\%e`+"`f"+`\g/netdoctor'` {
		t.Errorf("cron: %s", got)
	}
	// A line break would end the entry and start a new one: refused everywhere.
	for _, goos := range []string{"darwin", "linux"} {
		if err := Apply(goos, t.TempDir(), Plan{Exe: "/a\n/b", Weekly: true}, newSimulated().run); err == nil {
			t.Errorf("%s: a path with a line break must be refused", goos)
		}
	}
	// The odd path goes into every file whole.
	home, p := t.TempDir(), newSimulated()
	p.systemd = true
	if err := Apply("linux", home, Plan{Exe: odd, Weekly: true, Alerts: true}, p.run); err != nil {
		t.Fatal(err)
	}
	if s := read(t, filepath.Join(home, ".config", "systemd", "user", "network-doctor-weekly.service")); !strings.Contains(s, systemdQuote(odd)) {
		t.Errorf("service:\n%s", s)
	}
	p2 := newSimulated()
	if err := Apply("linux", t.TempDir(), Plan{Exe: odd, Weekly: true}, p2.run); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p2.crontab, cronQuote(odd)+" weekly # network-doctor-weekly") {
		t.Errorf("crontab:\n%s", p2.crontab)
	}
}

// Without Custom the time is the default, Monday 0900; with Custom the day and hour are used as given.
func TestWeeklyTimeDefaultsAndChoices(t *testing.T) {
	home, p := t.TempDir(), newSimulated()
	if err := Apply("linux", home, Plan{Exe: "/x", Weekly: true}, p.run); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.crontab, "0 9 * * 1 ") {
		t.Errorf("default should be Monday 0900: %s", p.crontab)
	}
	p.systemd = true
	if err := Apply("linux", home, Plan{Exe: "/x", Weekly: true, Weekday: 5, Hour: 18, Custom: true}, p.run); err != nil {
		t.Fatal(err)
	}
	if s := read(t, filepath.Join(home, ".config", "systemd", "user", "network-doctor-weekly.timer")); !strings.Contains(s, "OnCalendar=Fri *-*-* 18:00:00") {
		t.Errorf("timer:\n%s", s)
	}
	if err := Apply("darwin", home, Plan{Exe: "/x", Weekly: true, Weekday: 0, Hour: 7, Custom: true}, p.run); err != nil {
		t.Fatal(err)
	}
	if s := read(t, filepath.Join(home, "Library", "LaunchAgents", "com.networkdoctor.weekly.plist")); !strings.Contains(s, "<key>Weekday</key>\n\t\t<integer>0</integer>") ||
		!strings.Contains(s, "<integer>7</integer>") {
		t.Errorf("Sunday 0700 expected:\n%s", s)
	}
	if err := Apply("linux", home, Plan{Exe: "/x", Weekly: true, Weekday: 7, Hour: 9, Custom: true}, p.run); err == nil {
		t.Error("a day out of range must be refused")
	}
	if err := Apply("linux", home, Plan{Weekly: true}, p.run); err == nil {
		t.Error("a job with no program must be refused")
	}
}

// Remove takes both jobs away and stops the helper; StopHelper touches only this program's helper.
func TestRemoveAndStopHelper(t *testing.T) {
	home, p := t.TempDir(), newSimulated()
	if err := Apply("darwin", home, Plan{Exe: exe, Weekly: true, Alerts: true}, p.run); err != nil {
		t.Fatal(err)
	}
	p.loaded["gui/501/com.networkdoctor.alerts"] = true
	if err := Remove("darwin", home, p.run); err != nil {
		t.Fatal(err)
	}
	if w, a := Status("darwin", home, p.run); w || a {
		t.Error("both jobs must be gone")
	}
	if len(p.loaded) != 0 {
		t.Errorf("nothing may stay loaded: %v", p.loaded)
	}

	lp := newSimulated()
	lp.systemd = true
	if err := StopHelper("linux", lp.run); err != nil {
		t.Fatalf("nothing running is not a failure: %v", err)
	}
	if !lp.asked("systemctl", "--user", "stop", "network-doctor-alerts.service") {
		t.Error("the service must be stopped")
	}
	for _, c := range lp.calls {
		if c[0] == "pkill" {
			if !strings.Contains(strings.Join(c, " "), "-u 501") || !strings.Contains(c[len(c)-1], "NetworkDoctor/") || !strings.HasSuffix(c[len(c)-1], " watch$") {
				t.Errorf("pkill must match only this person's helper in the program folder: %v", c)
			}
		}
	}
	if !lp.asked("pkill") {
		t.Error("a helper started without systemd must be stopped too")
	}
	for _, goos := range []string{"windows", "plan9"} {
		if Apply(goos, home, Plan{}, p.run) != ErrUnsupported || Remove(goos, home, p.run) != ErrUnsupported || StopHelper(goos, p.run) != ErrUnsupported {
			t.Errorf("%s is not served here", goos)
		}
	}
}

// Custom: an old caller that never sets it gets Monday 0900 whatever the day and hour; with it,
// Sunday midnight can be chosen.
func TestCustomWeeklyTime(t *testing.T) {
	home, p := t.TempDir(), newSimulated()
	p.systemd = true
	timer := filepath.Join(home, ".config", "systemd", "user", "network-doctor-weekly.timer")
	if err := Apply("linux", home, Plan{Exe: "/x", Weekly: true, Weekday: 5, Hour: 18}, p.run); err != nil {
		t.Fatal(err)
	}
	if s := read(t, timer); !strings.Contains(s, "OnCalendar=Mon *-*-* 09:00:00") {
		t.Errorf("no Custom means Monday 0900:\n%s", s)
	}
	if err := Apply("linux", home, Plan{Exe: "/x", Weekly: true, Weekday: 0, Hour: 0, Custom: true}, p.run); err != nil {
		t.Fatal(err)
	}
	if s := read(t, timer); !strings.Contains(s, "OnCalendar=Sun *-*-* 00:00:00") {
		t.Errorf("Custom Sunday midnight:\n%s", s)
	}
}

// After bootout the old job is waited for, and a bootstrap that fails at first is tried again,
// about five times over five seconds, before it fails.
func TestMacBootstrapWaitsAndRetries(t *testing.T) {
	waits := noWait(t)
	home, p := t.TempDir(), newSimulated()
	if err := Apply("darwin", home, Plan{Exe: exe, Weekly: true}, p.run); err != nil {
		t.Fatal(err)
	}
	p.bootFails, p.linger = 2, 2
	*waits = 0
	if err := Apply("darwin", home, Plan{Exe: exe, Weekly: true}, p.run); err != nil {
		t.Fatalf("two failed tries must be retried: %v", err)
	}
	if !p.loaded["gui/501/com.networkdoctor.weekly"] || *waits < 4 {
		t.Errorf("loaded=%v waits=%d", p.loaded, *waits)
	}
	before := len(p.calls)
	p.bootFails = 99
	if err := Apply("darwin", home, Plan{Exe: exe, Weekly: true}, p.run); err == nil {
		t.Error("a bootstrap that keeps failing must fail in the end")
	}
	n := 0
	for _, c := range p.calls[before:] {
		if c[0] == "launchctl" && c[1] == "bootstrap" {
			n++
		}
	}
	if n != 5 {
		t.Errorf("a failing bootstrap is tried 5 times, got %d", n)
	}
}

// HelperRunning and StartHelper: a repair can find a running helper and start it again.
func TestHelperRunningAndStartHelper(t *testing.T) {
	noWait(t)
	home, p := t.TempDir(), newSimulated()
	if HelperRunning("darwin", p.run) {
		t.Error("nothing runs yet")
	}
	if err := StartHelper("darwin", home, p.run); err == nil {
		t.Error("with no alerts job there is nothing to start, and it must say so")
	}
	if err := Apply("darwin", home, Plan{Exe: exe, Alerts: true}, p.run); err != nil {
		t.Fatal(err)
	}
	if err := StartHelper("darwin", home, p.run); err != nil || !HelperRunning("darwin", p.run) {
		t.Errorf("mac start: %v %v", err, p.calls)
	}

	// Linux with user systemd: the service is started.
	lh, lp := t.TempDir(), newSimulated()
	lp.systemd = true
	if err := Apply("linux", lh, Plan{Exe: "/x/NetworkDoctor/netdoctor", Alerts: true}, lp.run); err != nil {
		t.Fatal(err)
	}
	if HelperRunning("linux", lp.run) {
		t.Error("linux: nothing runs yet")
	}
	if err := StartHelper("linux", lh, lp.run); err != nil || !lp.asked("systemctl", "--user", "start", "network-doctor-alerts.service") || !HelperRunning("linux", lp.run) {
		t.Errorf("linux systemd start: %v %v", err, lp.calls)
	}

	// Linux without user systemd: the entry's own program is started on its own, the odd path whole.
	odd := `/home/t e/a"b'c$d%e` + "`f" + `\g/NetworkDoctor/netdoctor`
	nh, np := t.TempDir(), newSimulated()
	if err := Apply("linux", nh, Plan{Exe: odd, Alerts: true}, np.run); err != nil {
		t.Fatal(err)
	}
	if err := StartHelper("linux", nh, np.run); err != nil {
		t.Fatal(err)
	}
	last := np.calls[len(np.calls)-1]
	if last[0] != "sh" || last[1] != "-c" || !strings.Contains(last[2], "nohup") || last[3] != odd {
		t.Errorf("direct start: %q", last)
	}
	if !HelperRunning("linux", np.run) {
		t.Error("linux: the started helper must be found")
	}
	if err := StartHelper("linux", t.TempDir(), newSimulated().run); err == nil {
		t.Error("linux: no entry means nothing to start")
	}
	if HelperRunning("windows", np.run) || StartHelper("windows", nh, np.run) != ErrUnsupported {
		t.Error("windows is not served here")
	}
}

// The Mac jobs write what they print into the results folder's logs area, and a removal leaves
// that folder alone (the results, and their logs, go only when the person deletes results).
func TestMacJobsWriteRunLogs(t *testing.T) {
	home, p := t.TempDir(), newSimulated()
	if err := Apply("darwin", home, Plan{Exe: exe, Weekly: true, Alerts: true}, p.run); err != nil {
		t.Fatal(err)
	}
	logs := filepath.Join(home, "Library", "Application Support", "NetworkDoctor", "logs")
	for label, name := range map[string]string{"com.networkdoctor.weekly": "weekly-run.log", "com.networkdoctor.alerts": "alerts-run.log"} {
		s := read(t, filepath.Join(home, "Library", "LaunchAgents", label+".plist"))
		want := "<string>" + plistString(filepath.Join(logs, name)) + "</string>"
		for _, key := range []string{"<key>StandardOutPath</key>\n\t" + want, "<key>StandardErrorPath</key>\n\t" + want} {
			if !strings.Contains(s, key) {
				t.Errorf("%s lacks %q:\n%s", label, key, s)
			}
		}
	}
	if fi, err := os.Stat(logs); err != nil || !fi.IsDir() {
		t.Fatalf("the logs folder must exist before launchd writes to it: %v", err)
	}
	if err := os.WriteFile(filepath.Join(logs, "weekly-run.log"), []byte("run\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Remove("darwin", home, p.run); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(logs, "weekly-run.log")); err != nil {
		t.Error("removing the jobs must keep the run logs with the saved results")
	}
}

// ND-310: the weekly command exits with the finding's own code (10 to 14) after it saved a row,
// as the check command does. systemd must count those as a finished run, not a failed unit, so a
// slow week does not leave the weekly service marked failed. Run 44701230444 on GitHub's Mac saw
// a saved row with exit 12 (router).
func TestLinuxWeeklyServiceFindingCodesAreSuccess(t *testing.T) {
	svc := LinuxWeeklyService("/home/tester/.local/share/NetworkDoctor/netdoctor")
	var codes []string
	for _, c := range []core.Cause{core.CauseComputer, core.CauseCableOrSignal, core.CauseRouter, core.CauseProvider, core.CauseSignIn} {
		codes = append(codes, strconv.Itoa(core.ExitCode(core.Finding{Cause: c})))
	}
	want := "SuccessExitStatus=" + strings.Join(codes, " ")
	if !strings.Contains(svc, "\n"+want+"\n") {
		t.Errorf("the weekly service lacks %q:\n%s", want, svc)
	}
	if strings.Contains(svc, "SuccessExitStatus=0 ") || strings.Contains(svc, " 1 ") || strings.Contains(svc, " 2\n") {
		t.Errorf("only the finding codes count as success; a real error (1 or 2) must still fail:\n%s", svc)
	}
}
