// Package install sets Network Doctor up on a Mac or a Linux computer for the one person who runs
// it, and takes it away again (requirements ND-269 to ND-280, ND-290 to ND-294, ND-301, ND-302).
// It shows its choices as a page in the web browser on this computer only, writes nothing outside
// the person's home folder and never asks to act as an administrator. Windows has its own setup
// file and is refused here.
//
// Every outside program runs through Env.Run and every job through Env.Apply and Env.Remove, so
// tests use a pretend computer in a scratch folder and run nothing real.
//
// Owner: Rafiq. Version 0.3.0 (0.3.0, 2026-11-23: uninstall first puts back every fix still
// applied, and keeps only the undo notes when one cannot be put back; 0.2.0, 2026-11-23: uninstall
// removes everything, the results folder too, unless asked to keep it; 0.1.0, 2026-11-20: first build, Mac and Linux match Windows).
package install

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/fix"
	"github.com/rafiqkmateen/network-doctor/internal/jobs"
)

// Env is the computer the install works on. Zero function fields take the real ones.
type Env struct {
	GOOS, GOARCH  string
	Home          string // the person's home folder
	Exe           string // the running program, copied into place
	ResultsDir    string // where results are saved (the same folder the program reads)
	Version       string
	Getenv        func(string) string
	Run           jobs.Runner
	RunContext    func(ctx context.Context, name string, args ...string) (string, error) // Run, stopped when ctx ends
	Apply         func(goos, home string, p jobs.Plan, run jobs.Runner) error
	Remove        func(goos, home string, run jobs.Runner) error
	StopHelper    func(goos string, run jobs.Runner) error
	HelperRunning func(goos string, run jobs.Runner) bool
	StartHelper   func(goos, home string, run jobs.Runner) error
	RetireWait    time.Duration                        // how long a retired open page may take to go before it is stopped (5 seconds)
	OpenURL       func(url string) error               // opens the page in the default browser
	NoBrowser     func(url string, out io.Writer) bool // tells the address when no browser starts; true ends the page
	HTTP          *http.Client                         // fetches Ookla's file; nil takes a client that stays on Ookla
	OoklaFiles    map[string]OoklaFile                 // nil takes the fixed table
	Quiet         time.Duration                        // the page quits after this long with no request (10 minutes)
	Grace         time.Duration                        // and this long after the finish screen is shown (2 seconds)
	AliasWait     time.Duration                        // how long the Finder alias may take before a link is made (20 seconds)
	Silent        bool                                 // no page: the Desktop alias is a plain link, Finder is never asked
	Symlink       func(target, link string) error      // makes the Desktop link on a Mac (os.Symlink)
	Fixes         fix.System                           // the computer the fixes act on; nil takes the real one
}

func (e *Env) fill() {
	if e.RunContext == nil {
		if e.Run != nil {
			run := e.Run // a pretend runner stays pretend
			e.RunContext = func(_ context.Context, name string, args ...string) (string, error) { return run(name, args...) }
		} else {
			e.RunContext = execRunContext
		}
	}
	if e.Run == nil {
		e.Run = jobs.ExecRunner
	}
	if e.Apply == nil {
		e.Apply = jobs.Apply
	}
	if e.Remove == nil {
		e.Remove = jobs.Remove
	}
	if e.StopHelper == nil {
		e.StopHelper = jobs.StopHelper
	}
	if e.HelperRunning == nil {
		e.HelperRunning = jobs.HelperRunning
	}
	if e.StartHelper == nil {
		e.StartHelper = jobs.StartHelper
	}
	if e.RetireWait <= 0 {
		e.RetireWait = 12 * time.Second
	}
	if e.Symlink == nil {
		e.Symlink = os.Symlink
	}
	if e.Getenv == nil {
		e.Getenv = os.Getenv
	}
	if e.Quiet <= 0 {
		e.Quiet = 10 * time.Minute
	}
	if e.Grace <= 0 {
		e.Grace = 2 * time.Second
	}
	if e.AliasWait <= 0 {
		e.AliasWait = 20 * time.Second
	}
	if e.OoklaFiles == nil {
		e.OoklaFiles = defaultOoklaFiles
	}
	if e.HTTP == nil {
		e.HTTP = &http.Client{Timeout: 5 * time.Minute, CheckRedirect: ooklaRedirect}
	}
	if e.Version == "" {
		e.Version = "0.0.0"
	}
	if e.Fixes == nil {
		e.Fixes = fix.Real()
	}
}

// Choice is what the person picked: the two add-ons and Ookla's two ticks.
type Choice struct {
	Weekly     bool `json:"weekly"`
	Alerts     bool `json:"alerts"`
	OoklaOffer bool `json:"ookla_offer"`
	OoklaAgree bool `json:"ookla_agree"`
}

// Result says how the install went. Problems are plain sentences for the person.
type Result struct {
	Problems   []string
	OoklaTried bool // both ticks were given, so the fetch was tried
	OoklaAdded bool // the file matched, was unpacked and the tool ran once
}

// Install puts Network Doctor in place for the person and makes exactly the jobs chosen, removing
// the ones not chosen, so a second run repairs (ND-292). System errors go to log only.
func Install(e Env, c Choice, log io.Writer) Result {
	e.fill()
	var r Result
	problem := func(p string, err error) {
		fmt.Fprintf(log, "%s %v\n", p, err)
		for _, q := range r.Problems {
			if q == p {
				return
			}
		}
		r.Problems = append(r.Problems, p)
	}
	l := LayoutFor(e)
	if err := mkdirIn(e.Home, l.ProgramDir); err != nil {
		problem(probFolder, err)
		return r
	}
	// A running helper holds the old program; stop it first, as the Windows setup does, and start
	// it again afterwards when alerts are still chosen, so a repair never leaves it stopped.
	wasRunning := e.HelperRunning(e.GOOS, e.Run)
	if err := e.StopHelper(e.GOOS, e.Run); err != nil && !errors.Is(err, jobs.ErrUnsupported) {
		fmt.Fprintln(log, "Note: the helper could not be stopped:", err)
	}
	if err := copyProgram(e.Home, e.Exe, l.Program); err != nil {
		problem(probProgram, err)
		return r
	}
	if g := findGuide(e.Exe); g != "" {
		if err := copyFile(e.Home, g, filepath.Join(l.ProgramDir, "GUIDE.md"), 0o644); err != nil {
			fmt.Fprintln(log, "Note: the guide could not be copied:", err)
		}
	}
	if err := writeIn(e.Home, filepath.Join(l.ProgramDir, "installed.marker"), "Installed by Network Doctor setup.\n", 0o644); err != nil {
		problem(probProgram, err)
	}
	if err := makeIcons(e, l, log); err != nil {
		problem(probIcons, err)
	}
	// Ookla's tool from an earlier run counts while it is still there.
	if err := writeChoices(e.Home, l.ProgramDir, c, ooklaThere(l.ProgramDir)); err != nil {
		problem(probChoices, err)
	}
	if err := e.Apply(e.GOOS, e.Home, jobs.Plan{Exe: l.Program, Weekly: c.Weekly, Alerts: c.Alerts, Weekday: 1, Hour: 9}, e.Run); err != nil {
		problem(probJobs, err)
	} else if wasRunning && c.Alerts {
		if err := e.StartHelper(e.GOOS, e.Home, e.Run); err != nil {
			fmt.Fprintln(log, "Note: the alerts helper could not be started again now; it starts at the next log-in:", err)
		}
	}
	if c.OoklaOffer && c.OoklaAgree && !e.Silent {
		r.OoklaTried = true
		if err := addOokla(e, l.ProgramDir); err != nil {
			fmt.Fprintln(log, "Note: Ookla's tool was not added:", err)
		} else {
			r.OoklaAdded = true
			if err := writeChoices(e.Home, l.ProgramDir, c, true); err != nil {
				problem(probChoices, err)
			}
		}
	}
	return r
}

// NotUndoneError says that removal could not put back every fix still applied (ND-316); the
// results folder then keeps only the undo notes, in Notes.
type NotUndoneError struct {
	Failures []fix.UndoFailure
	Notes    string
}

func (e *NotUndoneError) Error() string { return nuHead }

// undoNotes are the files of the fixes' undo notes, the only ones kept when a fix was not put back.
var undoNotes = map[string]bool{"fix-saved.json": true, "fix-log.txt": true, "fix-enabled-here.txt": true,
	"fix-not-undone.txt": true}

// putBackFixes puts back every fix still recorded as applied, with the program's own undo, before
// anything is removed (ND-316). On a Mac and Linux a fix whose tool may ask for administrator rights
// is not tried, since removal never asks for them (ND-274).
func putBackFixes(e Env, log io.Writer) (failures []fix.UndoFailure, settled []string) {
	if e.ResultsDir == "" {
		return nil, nil
	}
	m := fix.NewManager(e.Fixes, e.ResultsDir, fix.Options{})
	skip := func(name string) bool { return (e.GOOS == "darwin" || e.GOOS == "linux") && m.NeedsAdmin(name) }
	failures, settled = m.UndoAll(skip)
	for _, f := range failures {
		fmt.Fprintf(log, "Note: not put back: %q (%s) %v\n", f.Name, f.Reason, f.Err)
	}
	for _, n := range settled {
		fmt.Fprintf(log, "Note: left as the person set it: %q\n", n)
	}
	return failures, settled
}

// Report says what a removal settled without a fault: the fixes whose setting the person changed
// by hand since the fix, left as they set it (ND-316).
type Report struct {
	Settled []string
}

// Uninstall is UninstallReport without the report.
func Uninstall(e Env, keepResults bool, log io.Writer) error {
	_, err := UninstallReport(e, keepResults, log)
	return err
}

// UninstallReport first puts back every fix still applied (ND-316); then it stops the helper,
// removes both jobs, every icon, launcher and app the install made, the choices file, Ookla's
// folder, the program and everything the program saved: the whole results folder and, on a Mac,
// the jobs' run logs, so a new install starts fresh. Only keepResults keeps those two (ND-290,
// ND-291). A fix whose setting was changed by hand is settled and named in the report. When a fix
// is not put back, the results folder keeps only its undo notes and a *NotUndoneError is returned.
func UninstallReport(e Env, keepResults bool, log io.Writer) (Report, error) {
	e.fill()
	l := LayoutFor(e)
	var errs []error
	note := func(p string, err error) {
		fmt.Fprintf(log, "%s %v\n", p, err)
		errs = append(errs, errors.New(p))
	}
	failures, settled := putBackFixes(e, log)
	rep := Report{Settled: settled}
	if len(failures) > 0 {
		errs = append(errs, &NotUndoneError{Failures: failures, Notes: e.ResultsDir})
	}
	notUndone := len(errs) > 0
	// No page of this program stays open with old numbers: it is told it was removed.
	retireOpenPage(e, l, log)
	if err := e.StopHelper(e.GOOS, e.Run); err != nil && !errors.Is(err, jobs.ErrUnsupported) {
		fmt.Fprintln(log, "Note: the helper could not be stopped:", err)
	}
	if err := e.Remove(e.GOOS, e.Home, e.Run); err != nil {
		note(probJobs, err)
	}
	paths := l.Made()
	for _, p := range readRecord(l.ProgramDir) {
		paths = append(paths, p)
	}
	paths = append(paths, filepath.Join(l.ProgramDir, "choices.json"), filepath.Join(l.ProgramDir, "ookla"), l.Program)
	// Only these are folders of ours; any other path that turns out to be a folder (a person's own
	// "Network Doctor" folder on the Desktop, say) is left alone.
	ours := map[string]bool{l.AppBundle: true, l.UninstApp: true, l.ProgramDir: true,
		filepath.Join(l.ProgramDir, "ookla"): true}
	for _, p := range paths {
		if fi, err := os.Lstat(p); err == nil && fi.IsDir() && !ours[p] {
			fmt.Fprintln(log, "Note: left alone, since it is a folder:", p)
			continue
		}
		if err := removeIn(e.Home, p); err != nil {
			note(probRemove, err)
		}
	}
	if keepResults {
		return rep, errors.Join(errs...)
	}
	if l.Results != "" {
		// Only a folder of the program's own name is ever deleted, whatever the results setting names.
		if filepath.Base(filepath.Clean(l.Results)) != resultsFolderName {
			note(probResultsName, fmt.Errorf("results folder %s is not named %s", l.Results, resultsFolderName))
		} else if notUndone {
			if err := keepOnlyUndoNotes(e.Home, l.Results); err != nil {
				note(probResults, err)
			}
		} else if err := removeIn(e.Home, l.Results); err != nil {
			note(probResults, err)
		}
	}
	if e.GOOS == "darwin" {
		// The Mac jobs log into a fixed place, the default results folder's logs; when the results
		// setting names another folder, that logs folder, and its NetworkDoctor folder once empty, go too.
		logs := jobs.MacLogDir(e.Home)
		if err := removeIn(e.Home, logs); err != nil {
			note(probResults, err)
		} else if parent := filepath.Dir(logs); filepath.Base(parent) == resultsFolderName && insideReal(e.Home, parent) {
			os.Remove(parent) // only when empty; one that still holds files is left
		}
	}
	return rep, errors.Join(errs...)
}

// keepOnlyUndoNotes deletes everything in the results folder but the undo notes, so a fix not put
// back can still be put back later (ND-316). Like every removal it stays inside home.
func keepOnlyUndoNotes(home, dir string) error {
	if !insideReal(home, dir) {
		return errOutsideHome
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	var errs []error
	for _, en := range entries {
		if en.Type().IsRegular() && undoNotes[en.Name()] {
			continue
		}
		errs = append(errs, removeIn(home, filepath.Join(dir, en.Name())))
	}
	return errors.Join(errs...)
}

// installed says whether an install for this person is there to remove.
func installed(l Layout) bool {
	_, err := os.Stat(l.Program)
	return err == nil
}

// makeIcons makes the app or launchers and the Desktop icon, and notes them in the record file.
func makeIcons(e Env, l Layout, log io.Writer) error {
	var errs []error
	if e.GOOS == "darwin" {
		contents := filepath.Join(l.AppBundle, "Contents")
		errs = append(errs,
			writeIn(e.Home, filepath.Join(contents, "Info.plist"),
				MacInfoPlist(PlistInfo{Name: appName, ID: bundleID, Executable: appName, Version: e.Version}), 0o644),
			writeIn(e.Home, l.Launcher, MacLauncherBeside("open"), 0o755))
		uc := filepath.Join(l.UninstApp, "Contents")
		errs = append(errs,
			writeIn(e.Home, filepath.Join(uc, "Info.plist"),
				MacInfoPlist(PlistInfo{Name: uninstallName, ID: uninstallID, Executable: uninstallName, Version: e.Version}), 0o644),
			writeIn(e.Home, filepath.Join(uc, "MacOS", uninstallName), MacLauncherAt(l.Program, "uninstall"), 0o755))
		errs = append(errs, macDesktopAlias(e, l, log))
	} else {
		prog := filepath.ToSlash(l.Program)
		launcher := DesktopEntry(appName, "Check your internet connection", prog, "open")
		errs = append(errs,
			writeIn(e.Home, l.Launcher, launcher, 0o755),
			writeIn(e.Home, l.UninstEnt, DesktopEntry(uninstallName, "Remove Network Doctor", prog, "uninstall"), 0o755))
		if err := mkdirIn(e.Home, l.Desktop); err != nil {
			errs = append(errs, err)
		} else if err := writeIn(e.Home, l.DeskIcon, launcher, 0o755); err != nil {
			errs = append(errs, err)
		} else if _, err := e.Run("gio", "set", l.DeskIcon, "metadata::trusted", "true"); err != nil {
			fmt.Fprintln(log, "Note: the Desktop icon could not be marked as trusted:", err)
		}
	}
	errs = append(errs, writeIn(e.Home, filepath.Join(l.ProgramDir, recordName), strings.Join(l.Made(), "\n")+"\n", 0o644))
	return errors.Join(errs...)
}

// macDesktopAlias puts an alias of the app on the Desktop through Finder, its paths given as
// arguments; when Finder does not answer in time, or in a silent install, a plain link instead.
func macDesktopAlias(e Env, l Layout, log io.Writer) error {
	if err := mkdirIn(e.Home, l.Desktop); err != nil {
		return err
	}
	if err := clearDeskIcon(e.Home, l.DeskIcon); err != nil {
		return err
	}
	if !e.Silent {
		// Finder gets AliasWait; then the request is stopped before a link is made, so the two never clash.
		ctx, cancel := context.WithTimeout(context.Background(), e.AliasWait)
		_, err := e.RunContext(ctx, "osascript",
			"-e", "on run argv",
			"-e", `tell application "Finder" to make new alias file at (POSIX file (item 2 of argv) as alias) `+
				`to (POSIX file (item 1 of argv) as alias) with properties {name:(item 3 of argv)}`,
			"-e", "end run",
			l.AppBundle, l.Desktop, appName)
		late := ctx.Err() != nil
		cancel()
		if err == nil && !late {
			return nil
		}
		fmt.Fprintln(log, "Note: Finder did not make the Desktop alias, so a link is made instead:", err)
		if err := clearDeskIcon(e.Home, l.DeskIcon); err != nil {
			return err
		}
	}
	return e.Symlink(l.AppBundle, l.DeskIcon)
}

// execRunContext runs a real command that is stopped when ctx ends.
func execRunContext(ctx context.Context, name string, args ...string) (string, error) {
	b, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(b)), err
}

// ooklaThere says whether Ookla's tool itself is in the ookla folder beside the program; an empty
// leftover folder does not count.
func ooklaThere(dir string) bool {
	for _, name := range []string{"speedtest", "speedtest.exe"} {
		if fi, err := os.Stat(filepath.Join(dir, "ookla", name)); err == nil && fi.Mode().IsRegular() {
			return true
		}
	}
	return false
}

// clearDeskIcon removes an old Desktop alias or link of this name, never a folder.
func clearDeskIcon(home, p string) error {
	fi, err := os.Lstat(p)
	if err != nil {
		return nil
	}
	if fi.IsDir() {
		return fmt.Errorf("%s is a folder", p)
	}
	return removeIn(home, p)
}

func exists(p string) bool { _, err := os.Lstat(p); return err == nil }

// writeChoices writes choices.json beside the program in the Windows installer's form (ND-275).
func writeChoices(home, dir string, c Choice, ookla bool) error {
	return writeIn(home, filepath.Join(dir, "choices.json"),
		fmt.Sprintf(`{"core":true,"scheduled":%t,"proactive":%t,"ookla":%t}`, c.Weekly, c.Alerts, ookla), 0o644)
}

// findGuide looks for GUIDE.md beside the setup program, and in a Mac app's Resources folder.
func findGuide(exe string) string {
	dir := filepath.Dir(exe)
	for _, p := range []string{filepath.Join(dir, "GUIDE.md"), filepath.Join(dir, "..", "Resources", "GUIDE.md")} {
		if fi, err := os.Stat(p); err == nil && fi.Mode().IsRegular() {
			return p
		}
	}
	return ""
}

// readRecord reads the list of what an earlier install made.
func readRecord(dir string) []string {
	b, err := os.ReadFile(filepath.Join(dir, recordName))
	if err != nil {
		return nil
	}
	var out []string
	for _, l := range strings.Split(string(b), "\n") {
		if l = strings.TrimSpace(l); l != "" {
			out = append(out, l)
		}
	}
	return out
}

// copyProgram puts the program in place: a new file beside it, then a rename, so a running copy
// is never written over. A run from the installed copy itself copies nothing.
func copyProgram(home, src, dst string) error {
	if a, err := os.Stat(src); err == nil {
		if b, err := os.Stat(dst); err == nil && os.SameFile(a, b) {
			return nil
		}
	}
	return copyFile(home, src, dst, 0o755)
}

func copyFile(home, src, dst string, mode os.FileMode) error {
	b, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return writeBytesIn(home, dst, b, mode)
}

func writeIn(home, p, text string, mode os.FileMode) error {
	return writeBytesIn(home, p, []byte(text), mode)
}

// writeBytesIn writes a whole file at once under home: a temporary file in the same folder, then
// a rename. It refuses any place outside home (ND-274).
func writeBytesIn(home, p string, b []byte, mode os.FileMode) error {
	if !insideReal(home, p) {
		return errOutsideHome
	}
	if err := mkdirIn(home, filepath.Dir(p)); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(p), ".netdoctor-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := io.Copy(f, bytes.NewReader(b))
	cerr := f.Close()
	if err := errors.Join(werr, cerr, os.Chmod(tmp, mode)); err != nil {
		os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

func mkdirIn(home, p string) error {
	if !insideReal(home, p) || !insideReal(home, filepath.Join(p, "x")) {
		return errOutsideHome
	}
	return os.MkdirAll(p, 0o755)
}

// removeIn removes a file or folder under home; a missing one is fine.
func removeIn(home, p string) error {
	if p == "" {
		return nil
	}
	if !insideReal(home, p) {
		return errOutsideHome
	}
	if err := os.RemoveAll(p); err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}

// openStateName is the note the open command keeps in the results folder while its page runs.
const openStateName = "open-page.json"

// retireOpenPage asks a running open page to retire, the way a second press does, saying it was
// removed so its page says so; it then waits up to RetireWait for it to go, and if it is still
// there stops only this person's installed program running open, never this process.
func retireOpenPage(e Env, l Layout, log io.Writer) {
	state := filepath.Join(e.ResultsDir, openStateName)
	b, err := os.ReadFile(state)
	if err != nil {
		return
	}
	var st struct {
		Port  string `json:"port"`
		Token string `json:"token"`
	}
	if json.Unmarshal(b, &st) != nil || st.Token == "" {
		return
	}
	if n, err := strconv.Atoi(st.Port); err != nil || n < 1 || n > 65535 {
		return
	}
	addr := "127.0.0.1:" + st.Port
	req, err := http.NewRequest(http.MethodPost, "http://"+addr+"/api/retire", nil)
	if err != nil {
		return
	}
	req.Header.Set("X-NetDoctor-Retire", st.Token)
	req.Header.Set("X-NetDoctor-Retire-Reason", "removed")
	c := &http.Client{Timeout: 2 * time.Second}
	res, err := c.Do(req)
	if err != nil {
		return // nothing answers: no page is running
	}
	res.Body.Close()
	end := time.Now().Add(e.RetireWait)
	for time.Now().Before(end) {
		if !exists(state) {
			return
		}
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err != nil {
			return
		}
		conn.Close()
		time.Sleep(50 * time.Millisecond)
	}
	uid := strconv.Itoa(os.Getuid())
	if out, err := e.Run("id", "-u"); err == nil {
		if s := strings.TrimSpace(out); s != "" {
			if _, err := strconv.Atoi(s); err == nil {
				uid = s
			}
		}
	}
	// pkill answers 1 when nothing matched, which is fine.
	if out, err := e.Run("pkill", "-u", uid, "-f", openPattern(l.Program)); err != nil {
		fmt.Fprintln(log, "Note: the open page was told to stop:", strings.TrimSpace(out), err)
	}
}

// openPattern matches only the installed program at its own place running open.
func openPattern(program string) string {
	return "^" + regexp.QuoteMeta(filepath.ToSlash(program)) + " open( |$)"
}
