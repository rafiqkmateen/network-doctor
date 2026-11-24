// Package jobs makes and removes the weekly check and the alerts helper on Mac and Linux
// (requirements ND-282 to ND-286, ND-292). Every function takes the system name, the person's
// home folder and a command runner, so the file texts and the commands can be tested on any system
// with a pretend runner and a scratch home folder. Nothing here needs administrator rights.
//
// Mac: two LaunchAgents in ~/Library/LaunchAgents, loaded into the person's own session with
// `launchctl bootstrap gui/<uid>` and taken out with `launchctl bootout`. Linux: a user service and
// timer in ~/.config/systemd/user for the weekly check (or one marked crontab line when the person
// has no user systemd), and an autostart entry in ~/.config/autostart for the alerts helper.
//
// Owner: Rafiq. Version 0.2.0 (2026-11-20: Mac and Linux built; 0.1.0, 2026-11-20: the shared
// surface, written first so the installer and the job builders work against one set of signatures).
package jobs

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

// Runner runs one command and returns its output. Tests pass a pretend one.
type Runner func(name string, args ...string) (string, error)

// ExecRunner is the real Runner.
func ExecRunner(name string, args ...string) (string, error) {
	b, err := exec.Command(name, args...).CombinedOutput()
	return strings.TrimSpace(string(b)), err
}

// Plan says what to make. Exe is the full path of the installed program. The weekly check runs at
// the default time, Monday 0900 (the same as the Windows installer), unless Custom is true; then
// Weekday (0 for Sunday to 6 for Saturday) and Hour (0 to 23) are used as given, so Sunday at
// midnight can be chosen. A caller that never sets Custom always gets the default.
type Plan struct {
	Exe     string
	Weekly  bool
	Alerts  bool
	Weekday int
	Hour    int
	Custom  bool
}

// ErrUnsupported is returned for a system this package does not serve (Windows has its own setup).
var ErrUnsupported = errors.New("jobs: this system is not served here")

// The names of the jobs, as the requirements give them.
const (
	MacWeeklyLabel    = "com.networkdoctor.weekly"
	MacAlertsLabel    = "com.networkdoctor.alerts"
	LinuxWeeklyName   = "network-doctor-weekly" // .service and .timer
	LinuxAlertsUnit   = "network-doctor-alerts.service"
	LinuxAlertsEntry  = "network-doctor-alerts.desktop"
	CronMark          = "# network-doctor-weekly"
	helperPattern     = "/NetworkDoctor/[^/]+ watch$" // the installed program running as the helper
	cronScratchPrefix = ".network-doctor-crontab"
)

var dayNames = []string{"Sun", "Mon", "Tue", "Wed", "Thu", "Fri", "Sat"}

func served(goos string) bool { return goos == "darwin" || goos == "linux" }

// MacAgentPath is where a LaunchAgent of this program lives.
func MacAgentPath(home, label string) string {
	return filepath.Join(home, "Library", "LaunchAgents", label+".plist")
}

func unitDir(home string) string { return filepath.Join(home, ".config", "systemd", "user") }

// LinuxAutostartPath is the alerts helper's autostart entry.
func LinuxAutostartPath(home string) string {
	return filepath.Join(home, ".config", "autostart", LinuxAlertsEntry)
}

// Apply makes exactly the jobs the plan asks for and removes the ones it does not (so running the
// installer again repairs the machine to match the choices, ND-292).
func Apply(goos, home string, p Plan, run Runner) error {
	if !served(goos) {
		return ErrUnsupported
	}
	if run == nil {
		run = ExecRunner
	}
	if err := SetWeekly(goos, home, p, run); err != nil {
		return err
	}
	return setAlerts(goos, home, p, run)
}

// Remove takes away both jobs and stops the alerts helper if it is running.
func Remove(goos, home string, run Runner) error {
	if !served(goos) {
		return ErrUnsupported
	}
	if run == nil {
		run = ExecRunner
	}
	if err := Apply(goos, home, Plan{}, run); err != nil {
		return err
	}
	return StopHelper(goos, run)
}

// Status says which of the two jobs exist now, read from the files and the system.
func Status(goos, home string, run Runner) (weekly, alerts bool) {
	if !served(goos) {
		return false, false
	}
	if run == nil {
		run = ExecRunner
	}
	weekly = WeeklyStatus(goos, home, run)
	if goos == "darwin" {
		return weekly, exists(MacAgentPath(home, MacAlertsLabel))
	}
	return weekly, exists(LinuxAutostartPath(home))
}

// StopHelper stops a running alerts helper (the process the alerts job started) and nothing else.
func StopHelper(goos string, run Runner) error {
	if !served(goos) {
		return ErrUnsupported
	}
	if run == nil {
		run = ExecRunner
	}
	id := uid(run)
	if goos == "darwin" {
		// The job restarts a stopped helper (KeepAlive), so it is unloaded, which also stops it.
		bootout(run, "gui/"+id+"/"+MacAlertsLabel)
		return nil
	}
	run("systemctl", "--user", "stop", LinuxAlertsUnit)
	// A helper the autostart entry started directly: only this person's, only the installed
	// program running `watch`. pkill answers 1 when nothing matched, which is not a failure.
	if out, err := run("pkill", "-u", id, "-f", helperPattern); err != nil && exitCode(err) > 1 {
		return fmt.Errorf("the alerts helper could not be stopped: %s", out)
	}
	return nil
}

// SetWeekly turns only the weekly job on or off, leaving the alerts job alone (schedule enable
// and disable, ND-284). p.Alerts is not read.
func SetWeekly(goos, home string, p Plan, run Runner) error {
	if !served(goos) {
		return ErrUnsupported
	}
	if run == nil {
		run = ExecRunner
	}
	day, hour, err := when(p)
	if err != nil {
		return err
	}
	if p.Weekly {
		if err := checkExe(p.Exe); err != nil {
			return err
		}
	}
	if goos == "darwin" {
		return macWeekly(home, p.Exe, p.Weekly, day, hour, run)
	}
	return linuxWeekly(home, p.Exe, p.Weekly, day, hour, run)
}

// WeeklyStatus says whether the weekly job is there now.
func WeeklyStatus(goos, home string, run Runner) bool {
	switch goos {
	case "darwin":
		return exists(MacAgentPath(home, MacWeeklyLabel))
	case "linux":
		if run == nil {
			run = ExecRunner
		}
		if exists(filepath.Join(unitDir(home), LinuxWeeklyName+".timer")) {
			return true
		}
		lines, err := readCrontab(run)
		return err == nil && hasMark(lines)
	}
	return false
}

// when gives the day and hour of the weekly run.
func when(p Plan) (int, int, error) {
	if !p.Custom {
		return 1, 9, nil
	}
	if p.Weekday < 0 || p.Weekday > 6 || p.Hour < 0 || p.Hour > 23 {
		return 0, 0, fmt.Errorf("the weekly check needs a day from 0 to 6 and an hour from 0 to 23, not %d and %d", p.Weekday, p.Hour)
	}
	return p.Weekday, p.Hour, nil
}

// checkExe refuses an empty path, and a path with a line break or other control sign, which
// would end a line of the job file and start another.
func checkExe(exe string) error {
	if exe == "" {
		return errors.New("the program's place is missing")
	}
	for _, r := range exe {
		if r < 0x20 || r == 0x7f {
			return errors.New("the program's place holds a line break or another sign that cannot go in a job file")
		}
	}
	return nil
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func writeFile(path, text string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(text), 0o644)
}

func removeFile(path string) error {
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// uid is the person's user number, asked of the system through the runner.
func uid(run Runner) string {
	if out, err := run("id", "-u"); err == nil {
		if s := strings.TrimSpace(out); s != "" {
			if _, err := strconv.Atoi(s); err == nil {
				return s
			}
		}
	}
	return strconv.Itoa(os.Getuid())
}

func exitCode(err error) int {
	var ee *exec.ExitError
	if errors.As(err, &ee) {
		return ee.ExitCode()
	}
	if err != nil {
		return 1 // a pretend runner's failure, or a missing program: treated as "nothing matched"
	}
	return 0
}

// ---------------------------------------------------------------------------------------------
// Mac

// MacLogDir is where the Mac jobs write what they print: the results folder's own logs area, so the
// logs stay with the saved results; removing the jobs keeps them, and uninstall deletes them with
// the results unless asked to keep the results.
func MacLogDir(home string) string {
	return filepath.Join(home, "Library", "Application Support", "NetworkDoctor", "logs")
}

// MacWeeklyPlist is the weekly LaunchAgent (ND-282) with no run logs. launchd runs a calendar job
// that was missed while the Mac slept as soon as it wakes.
func MacWeeklyPlist(exe string, day, hour int) string { return macWeeklyPlist(exe, day, hour, "") }

// MacAlertsPlist is the alerts LaunchAgent (ND-285) with no run logs: started at each log-in and
// again if it stops.
func MacAlertsPlist(exe string) string { return macAlertsPlist(exe, "") }

func macWeeklyPlist(exe string, day, hour int, logDir string) string {
	return plistHead(MacWeeklyLabel, exe, "weekly") + fmt.Sprintf(`	<key>StartCalendarInterval</key>
	<dict>
		<key>Weekday</key>
		<integer>%d</integer>
		<key>Hour</key>
		<integer>%d</integer>
		<key>Minute</key>
		<integer>0</integer>
	</dict>
`, day, hour) + plistLogs(logDir, "weekly-run.log") + plistTail
}

func macAlertsPlist(exe, logDir string) string {
	return plistHead(MacAlertsLabel, exe, "watch") + `	<key>RunAtLoad</key>
	<true/>
	<key>KeepAlive</key>
	<true/>
` + plistLogs(logDir, "alerts-run.log") + plistTail
}

const plistTail = `	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
`

// plistLogs sends the job's output and errors to one file in logDir ("" means none).
func plistLogs(logDir, name string) string {
	if logDir == "" {
		return ""
	}
	p := plistString(filepath.Join(logDir, name))
	return "\t<key>StandardOutPath</key>\n\t<string>" + p + "</string>\n" +
		"\t<key>StandardErrorPath</key>\n\t<string>" + p + "</string>\n"
}

func plistHead(label, exe, arg string) string {
	return `<?xml version="1.0" encoding="UTF-8"?>
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + label + `</string>
	<key>ProgramArguments</key>
	<array>
		<string>` + plistString(exe) + `</string>
		<string>` + arg + `</string>
	</array>
`
}

// plistString escapes text for a plist <string>. The program and its word are separate array
// items, so no shell ever reads them.
func plistString(s string) string {
	var b bytes.Buffer
	_ = xml.EscapeText(&b, []byte(s))
	return b.String()
}

// sleep waits between tries. Tests make it instant.
var sleep = time.Sleep

// bootout unloads a job and waits, about five seconds at most, until launchd no longer has it, so
// loading it again does not fail.
func bootout(run Runner, target string) {
	if _, err := run("launchctl", "bootout", target); err != nil {
		if _, perr := run("launchctl", "print", target); perr != nil {
			return // it was not loaded
		}
	}
	for i := 0; i < 5; i++ {
		if _, err := run("launchctl", "print", target); err != nil {
			return
		}
		sleep(time.Second)
	}
}

// bootstrap loads a job, trying again about five times over five seconds while launchd is still
// letting go of the old one.
func bootstrap(run Runner, dom, path string) (string, error) {
	var out string
	var err error
	for i := 0; i < 5; i++ {
		if out, err = run("launchctl", "bootstrap", dom, path); err == nil {
			return out, nil
		}
		if i < 4 {
			sleep(time.Second)
		}
	}
	return out, err
}

func macWeekly(home, exe string, on bool, day, hour int, run Runner) error {
	path := MacAgentPath(home, MacWeeklyLabel)
	dom := "gui/" + uid(run)
	bootout(run, dom+"/"+MacWeeklyLabel) // a loaded old job would make bootstrap fail
	if !on {
		return removeFile(path)
	}
	if err := os.MkdirAll(MacLogDir(home), 0o755); err != nil {
		return err
	}
	if err := writeFile(path, macWeeklyPlist(exe, day, hour, MacLogDir(home))); err != nil {
		return err
	}
	if out, err := bootstrap(run, dom, path); err != nil {
		return fmt.Errorf("the weekly check could not be started: %s", strings.TrimSpace(out))
	}
	return nil
}

func setAlerts(goos, home string, p Plan, run Runner) error {
	if p.Alerts {
		if err := checkExe(p.Exe); err != nil {
			return err
		}
	}
	if goos == "darwin" {
		return macAlerts(home, p.Exe, p.Alerts, run)
	}
	return linuxAlerts(home, p.Exe, p.Alerts, run)
}

// macAlerts writes or removes the alerts agent. Like the Windows log-on task, a new agent starts
// at the next log-in; one that is running now is reloaded so it runs the new program.
func macAlerts(home, exe string, on bool, run Runner) error {
	path := MacAgentPath(home, MacAlertsLabel)
	dom := "gui/" + uid(run)
	target := dom + "/" + MacAlertsLabel
	_, err := run("launchctl", "print", target)
	running := err == nil
	if !on {
		if running {
			bootout(run, target)
		}
		return removeFile(path)
	}
	if err := os.MkdirAll(MacLogDir(home), 0o755); err != nil {
		return err
	}
	if err := writeFile(path, macAlertsPlist(exe, MacLogDir(home))); err != nil {
		return err
	}
	if running {
		bootout(run, target)
		if out, err := bootstrap(run, dom, path); err != nil {
			return fmt.Errorf("the alerts helper could not be started again: %s", strings.TrimSpace(out))
		}
	}
	return nil
}

// HelperRunning says whether the alerts helper runs now: on a Mac its job is loaded and running;
// on Linux its user service is active, or the installed program runs `watch` for this person.
func HelperRunning(goos string, run Runner) bool {
	if !served(goos) {
		return false
	}
	if run == nil {
		run = ExecRunner
	}
	id := uid(run)
	if goos == "darwin" {
		out, err := run("launchctl", "print", "gui/"+id+"/"+MacAlertsLabel)
		return err == nil && strings.Contains(out, "state = running")
	}
	if _, err := run("systemctl", "--user", "is-active", "--quiet", LinuxAlertsUnit); err == nil {
		return true
	}
	_, err := run("pgrep", "-u", id, "-f", helperPattern)
	return err == nil
}

// StartHelper starts the alerts helper now, the way its job starts it, so a repair never leaves a
// helper stopped that was running before. Mac: the alerts job is loaded (it starts at once). Linux:
// the user service is started when it is there; else the autostart entry's own program is started
// on its own, apart from the caller.
func StartHelper(goos, home string, run Runner) error {
	if !served(goos) {
		return ErrUnsupported
	}
	if run == nil {
		run = ExecRunner
	}
	if goos == "darwin" {
		path := MacAgentPath(home, MacAlertsLabel)
		if !exists(path) {
			return errors.New("alerts are not set up, so there is no helper to start")
		}
		dom := "gui/" + uid(run)
		bootout(run, dom+"/"+MacAlertsLabel)
		if out, err := bootstrap(run, dom, path); err != nil {
			return fmt.Errorf("the alerts helper could not be started: %s", strings.TrimSpace(out))
		}
		return nil
	}
	sd := hasUserSystemd(run)
	if sd && exists(filepath.Join(unitDir(home), LinuxAlertsUnit)) {
		if out, err := run("systemctl", "--user", "start", LinuxAlertsUnit); err != nil {
			return fmt.Errorf("the alerts helper could not be started: %s", strings.TrimSpace(out))
		}
		return nil
	}
	b, err := os.ReadFile(LinuxAutostartPath(home))
	if err != nil {
		return errors.New("alerts are not set up, so there is no helper to start")
	}
	exe, ok := entryProgram(string(b))
	if !ok {
		return errors.New("the alerts entry does not name the program, so the helper was not started")
	}
	if out, err := run("sh", "-c", `nohup "$0" watch >/dev/null 2>&1 &`, exe); err != nil {
		return fmt.Errorf("the alerts helper could not be started: %s", strings.TrimSpace(out))
	}
	return nil
}

// entryProgram reads the program back from an alerts entry this package wrote with desktopQuote:
// Exec="<program>" watch. It undoes the steps of desktopQuote in reverse order.
func entryProgram(entry string) (string, bool) {
	for _, l := range strings.Split(entry, "\n") {
		v, ok := strings.CutPrefix(strings.TrimSpace(l), "Exec=")
		if !ok {
			continue
		}
		v = strings.ReplaceAll(v, "%%", "%")
		v = strings.ReplaceAll(v, `\\`, `\`)
		if !strings.HasPrefix(v, `"`) {
			return "", false
		}
		var b strings.Builder
		for i := 1; i < len(v); i++ {
			switch c := v[i]; {
			case c == '\\' && i+1 < len(v):
				i++
				b.WriteByte(v[i])
			case c == '"':
				return b.String(), strings.TrimSpace(v[i+1:]) == "watch"
			default:
				b.WriteByte(c)
			}
		}
		return "", false
	}
	return "", false
}

// ---------------------------------------------------------------------------------------------
// Linux

// hasUserSystemd is true when the person's own systemd answers.
func hasUserSystemd(run Runner) bool {
	_, err := run("systemctl", "--user", "show-environment")
	return err == nil
}

// LinuxWeeklyService runs the weekly check once. After it saved a row, the weekly command exits
// with the finding's own code (10 to 14, as the check command does); those count as a finished
// run, so a slow week does not leave the service marked failed. A real error (1 or 2) still fails.
func LinuxWeeklyService(exe string) string {
	return `[Unit]
Description=Network Doctor weekly check

[Service]
Type=oneshot
ExecStart=` + systemdQuote(exe) + ` weekly
SuccessExitStatus=10 11 12 13 14
`
}

// LinuxWeeklyTimer starts the service each week; Persistent catches up a run missed while the
// computer was off (ND-283).
func LinuxWeeklyTimer(day, hour int) string {
	return fmt.Sprintf(`[Unit]
Description=Network Doctor weekly check

[Timer]
OnCalendar=%s *-*-* %02d:00:00
Persistent=true

[Install]
WantedBy=timers.target
`, dayNames[day], hour)
}

// LinuxAlertsService runs the helper and starts it again a minute after it stops with an error.
func LinuxAlertsService(exe string) string {
	return `[Unit]
Description=Network Doctor alerts

[Service]
ExecStart=` + systemdQuote(exe) + ` watch
Restart=on-failure
RestartSec=60
`
}

// startService is the autostart command when user systemd is there: hand the desktop's display
// to the user service so its pop-ups can show, then start it.
const startService = `sh -c "systemctl --user import-environment DISPLAY WAYLAND_DISPLAY XAUTHORITY DBUS_SESSION_BUS_ADDRESS; systemctl --user start ` + LinuxAlertsUnit + `"`

// LinuxAlertsEntryText is the autostart entry (ND-286). With user systemd it starts the service;
// without, it starts the helper directly.
func LinuxAlertsEntryText(exe string, viaSystemd bool) string {
	cmd := desktopQuote(exe) + " watch"
	if viaSystemd {
		cmd = startService
	}
	return `[Desktop Entry]
Type=Application
Name=Network Doctor alerts
Comment=Watches your connection and tells you when it drops or a new device shows up.
Exec=` + cmd + `
Terminal=false
NoDisplay=true
X-GNOME-Autostart-enabled=true
`
}

// systemdQuote writes one argument for a unit's ExecStart: in double quotes, with a backslash
// before a backslash or a quote, and % and $ doubled so systemd does not expand them.
func systemdQuote(s string) string {
	r := strings.NewReplacer(`\`, `\\`, `"`, `\"`, "%", "%%", "$", "$$")
	return `"` + r.Replace(s) + `"`
}

// desktopQuote writes one argument for a desktop entry's Exec, as the Desktop Entry
// Specification says: in double quotes with a backslash before " ` $ and \; then, because the
// value is itself a string, every backslash doubled again; and % doubled.
func desktopQuote(s string) string {
	r := strings.NewReplacer(`"`, `\"`, "`", "\\`", "$", `\$`, `\`, `\\`)
	q := `"` + r.Replace(s) + `"`
	q = strings.ReplaceAll(q, `\`, `\\`)
	return strings.ReplaceAll(q, "%", "%%")
}

// cronQuote writes one argument for a crontab line: in single quotes for the shell, an embedded
// quote closed, escaped and opened again, and % written as \% because a bare % ends the command
// in cron.
func cronQuote(s string) string {
	q := "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
	return strings.ReplaceAll(q, "%", `\%`)
}

// CronLine is the one crontab line of the fallback weekly job.
func CronLine(exe string, day, hour int) string {
	return fmt.Sprintf("0 %d * * %d %s weekly %s", hour, day, cronQuote(exe), CronMark)
}

func hasMark(lines []string) bool {
	for _, l := range lines {
		if isMarked(l) {
			return true
		}
	}
	return false
}

func isMarked(l string) bool { return strings.HasSuffix(strings.TrimSpace(l), CronMark) }

// readCrontab reads the person's crontab. "No crontab" is an empty one.
func readCrontab(run Runner) ([]string, error) {
	out, err := run("crontab", "-l")
	if err != nil {
		if strings.Contains(strings.ToLower(out), "no crontab") {
			return nil, nil
		}
		return nil, fmt.Errorf("the crontab could not be read: %s", strings.TrimSpace(out))
	}
	if strings.TrimSpace(out) == "" {
		return nil, nil
	}
	return strings.Split(strings.TrimRight(out, "\n"), "\n"), nil
}

// writeCrontab installs the lines from a scratch file in the home folder (the runner gives the
// command no input), then removes the file.
func writeCrontab(home string, lines []string, run Runner) error {
	f, err := os.CreateTemp(home, cronScratchPrefix)
	if err != nil {
		return err
	}
	name := f.Name()
	defer os.Remove(name)
	text := ""
	if len(lines) > 0 {
		text = strings.Join(lines, "\n") + "\n"
	}
	_, werr := f.WriteString(text)
	if cerr := f.Close(); werr != nil || cerr != nil {
		return errors.Join(werr, cerr)
	}
	if out, err := run("crontab", name); err != nil {
		return fmt.Errorf("the crontab could not be saved: %s", strings.TrimSpace(out))
	}
	return nil
}

// cronWeekly puts the one marked line in (line != "") or takes it out, keeping every other line.
// When there is nothing to change, the crontab is not written.
func cronWeekly(home, line string, run Runner) error {
	lines, err := readCrontab(run)
	if err != nil {
		if line == "" {
			return nil // no crontab program: there is no line to take out
		}
		return err
	}
	var keep []string
	for _, l := range lines {
		if !isMarked(l) {
			keep = append(keep, l)
		}
	}
	if line != "" {
		keep = append(keep, line)
	}
	if strings.Join(keep, "\n") == strings.Join(lines, "\n") {
		return nil
	}
	return writeCrontab(home, keep, run)
}

func linuxWeekly(home, exe string, on bool, day, hour int, run Runner) error {
	svc := filepath.Join(unitDir(home), LinuxWeeklyName+".service")
	tmr := filepath.Join(unitDir(home), LinuxWeeklyName+".timer")
	sd := hasUserSystemd(run)
	if sd {
		run("systemctl", "--user", "disable", "--now", LinuxWeeklyName+".timer")
	}
	if err := errors.Join(removeFile(svc), removeFile(tmr)); err != nil {
		return err
	}
	switch {
	case on && sd:
		if err := errors.Join(writeFile(svc, LinuxWeeklyService(exe)), writeFile(tmr, LinuxWeeklyTimer(day, hour))); err != nil {
			return err
		}
		run("systemctl", "--user", "daemon-reload")
		if out, err := run("systemctl", "--user", "enable", "--now", LinuxWeeklyName+".timer"); err != nil {
			return fmt.Errorf("the weekly check could not be started: %s", strings.TrimSpace(out))
		}
		return cronWeekly(home, "", run) // an old line from before the timer
	case on:
		if err := cronWeekly(home, CronLine(exe, day, hour), run); err != nil {
			return fmt.Errorf("the weekly check could not be set up: %w", err)
		}
		return nil
	}
	if sd {
		run("systemctl", "--user", "daemon-reload")
	}
	return cronWeekly(home, "", run)
}

func linuxAlerts(home, exe string, on bool, run Runner) error {
	unit := filepath.Join(unitDir(home), LinuxAlertsUnit)
	entry := LinuxAutostartPath(home)
	sd := hasUserSystemd(run)
	if !on {
		if sd {
			run("systemctl", "--user", "stop", LinuxAlertsUnit)
		}
		if err := errors.Join(removeFile(unit), removeFile(entry)); err != nil {
			return err
		}
		if sd {
			run("systemctl", "--user", "daemon-reload")
		}
		return nil
	}
	if sd {
		if err := writeFile(unit, LinuxAlertsService(exe)); err != nil {
			return err
		}
		run("systemctl", "--user", "daemon-reload")
		run("systemctl", "--user", "try-restart", LinuxAlertsUnit) // a running helper runs the new program
	} else if err := removeFile(unit); err != nil {
		return err
	}
	return writeFile(entry, LinuxAlertsEntryText(exe, sd))
}
