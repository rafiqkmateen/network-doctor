package install

import (
	"bytes"
	"encoding/json"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/jobs"
)

func choicesIn(t *testing.T, dir string) map[string]bool {
	t.Helper()
	var c map[string]bool
	if err := json.Unmarshal([]byte(read(t, filepath.Join(dir, "choices.json"))), &c); err != nil {
		t.Fatal(err)
	}
	return c
}

// ND-273: each choice makes exactly its jobs, and only those.
func TestInstallInstallsOnlyTheChosenParts(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		for _, c := range []Choice{{}, {Weekly: true}, {Alerts: true}, {Weekly: true, Alerts: true}} {
			s := newScratch(t, goos)
			r := Install(s.env, c, io.Discard)
			if len(r.Problems) > 0 {
				t.Fatalf("%s %+v: %v", goos, c, r.Problems)
			}
			l := LayoutFor(s.env)
			if len(s.p.plans) != 1 {
				t.Fatalf("jobs asked %d times", len(s.p.plans))
			}
			pl := s.p.plans[0]
			if pl.Weekly != c.Weekly || pl.Alerts != c.Alerts || pl.Exe != l.Program || pl.Weekday != 1 || pl.Hour != 9 {
				t.Errorf("%s %+v: jobs plan %+v", goos, c, pl)
			}
			if exists(filepath.Join(l.ProgramDir, "ookla")) {
				t.Error("Ookla's tool came without both ticks")
			}
			if len(s.p.ran("curl")) > 0 || len(s.p.ran("wget")) > 0 {
				t.Error("something was fetched")
			}
		}
	}
}

// ND-275: choices.json sits beside the program in the Windows form.
func TestInstallChoicesFileForm(t *testing.T) {
	s := newScratch(t, "linux")
	Install(s.env, Choice{Weekly: true}, io.Discard)
	l := LayoutFor(s.env)
	got := read(t, filepath.Join(l.ProgramDir, "choices.json"))
	if got != `{"core":true,"scheduled":true,"proactive":false,"ookla":false}` {
		t.Errorf("choices.json: %s", got)
	}
	if !exists(filepath.Join(l.ProgramDir, "installed.marker")) || read(t, filepath.Join(l.ProgramDir, "GUIDE.md")) != "# Guide\n" {
		t.Error("marker or guide missing beside the program")
	}
}

// ND-274: nothing is written or removed outside home, and no administrator is asked.
func TestInstallWritesOnlyInTheHomeFolder(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		s := newScratch(t, goos)
		os.WriteFile(filepath.Join(s.root, "neighbour.txt"), []byte("keep"), 0o644)
		outside := func() map[string]string {
			all := tree(t, s.root)
			for p := range all {
				if p == s.home || strings.HasPrefix(p, s.home+string(filepath.Separator)) {
					delete(all, p)
				}
			}
			return all
		}
		before := outside()
		s.env.Apply = func(g, h string, pl jobs.Plan, r jobs.Runner) error {
			if h != s.home {
				t.Errorf("jobs given home %q", h)
			}
			return nil
		}
		Install(s.env, Choice{Weekly: true, Alerts: true}, io.Discard)
		Install(s.env, Choice{}, io.Discard)
		Uninstall(s.env, false, io.Discard)
		after := outside()
		if len(before) != len(after) {
			t.Errorf("%s: paths outside home changed: before %d, after %d", goos, len(before), len(after))
		}
		for p, v := range before {
			if after[p] != v {
				t.Errorf("%s: %s changed", goos, p)
			}
		}
		for _, c := range s.p.cmds {
			if c[0] == "sudo" || c[0] == "su" || c[0] == "pkexec" || strings.Contains(strings.Join(c, " "), "administrator privileges") {
				t.Errorf("asked for an administrator: %q", c)
			}
		}
	}
}

// savedFiles are the files the program writes in its results folder: results, the alert log,
// the known list with the device choices, the settings with the new-device switch, the speed
// notes, the open page's note, the fix notes, the page certificate, the draft email and run logs.
var savedFiles = []string{"weekly.csv", "alerts.log", "known-devices.txt", "config.json", "speed-hold.txt",
	"speed-last.json", "exit-place.json", "open-page.json", "fix-saved.json", "fix-log.txt",
	"fix-enabled-here.txt", "https-cert.pem", "https-key.pem", "complaint-draft.txt",
	filepath.Join("logs", "weekly-run.log"), filepath.Join("logs", "alerts-run.log")}

// makeResults puts every file the program saves in the results folder and returns the weekly one.
func makeResults(t *testing.T, s *scratch) string {
	t.Helper()
	for _, f := range savedFiles {
		p := filepath.Join(s.env.ResultsDir, f)
		os.MkdirAll(filepath.Dir(p), 0o755)
		text := "saved\n"
		if f == "fix-saved.json" {
			text = "{}" // undo notes with nothing recorded: every fix was put back
		}
		os.WriteFile(p, []byte(text), 0o644)
	}
	return filepath.Join(s.env.ResultsDir, "weekly.csv")
}

// ND-290, ND-291: uninstall takes away everything install made and everything the program
// saved, the whole results folder too, and nothing else.
func TestUninstallRemovesWhatInstallMade(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		s := newScratch(t, goos)
		makeResults(t, s)
		other := filepath.Join(s.home, "Desktop", "my notes.txt")
		os.MkdirAll(filepath.Dir(other), 0o755)
		os.WriteFile(other, []byte("mine"), 0o644)
		Install(s.env, Choice{Weekly: true, Alerts: true}, io.Discard)
		l := LayoutFor(s.env)
		os.MkdirAll(filepath.Join(l.ProgramDir, "ookla"), 0o755) // as if Ookla's tool came too
		os.WriteFile(filepath.Join(l.ProgramDir, "ookla", "speedtest"), []byte("x"), 0o755)
		if err := Uninstall(s.env, false, io.Discard); err != nil {
			t.Fatalf("%s: %v", goos, err)
		}
		if exists(s.env.ResultsDir) {
			t.Errorf("%s: the results folder is still there: %v", goos, tree(t, s.env.ResultsDir))
		}
		if len(s.fixes.calls) != 0 {
			t.Errorf("%s: with nothing recorded no setting may be read or changed: %v", goos, s.fixes.calls)
		}
		if !exists(filepath.Dir(s.env.ResultsDir)) {
			t.Errorf("%s: the folder that holds the results folder was removed", goos)
		}
		if s.p.removes != 1 {
			t.Errorf("%s: jobs removed %d times", goos, s.p.removes)
		}
		if s.p.stops < 1 {
			t.Errorf("%s: the helper was not told to stop", goos)
		}
		for _, p := range l.Made() {
			if exists(p) {
				t.Errorf("%s: %s is still there", goos, p)
			}
		}
		for _, p := range []string{l.Program, filepath.Join(l.ProgramDir, "choices.json"), filepath.Join(l.ProgramDir, "ookla")} {
			if exists(p) {
				t.Errorf("%s: %s is still there", goos, p)
			}
		}
		if read(t, other) != "mine" {
			t.Errorf("%s: something else in home changed", goos)
		}
	}
}

// ND-290: on a Mac the jobs' run logs go too, even when the results setting names another
// NetworkDoctor folder; with keep they stay.
func TestUninstallRemovesMacRunLogs(t *testing.T) {
	for _, keep := range []bool{false, true} {
		s := newScratch(t, "darwin")
		s.env.ResultsDir = filepath.Join(s.home, "elsewhere", "NetworkDoctor")
		makeResults(t, s)
		logs := jobs.MacLogDir(s.home)
		os.MkdirAll(logs, 0o755)
		os.WriteFile(filepath.Join(logs, "weekly-run.log"), []byte("ran\n"), 0o644)
		Install(s.env, Choice{Weekly: true}, io.Discard)
		if err := Uninstall(s.env, keep, io.Discard); err != nil {
			t.Fatalf("keep %v: %v", keep, err)
		}
		if exists(logs) != keep || exists(s.env.ResultsDir) != keep {
			t.Errorf("keep %v: run logs there %v, results there %v", keep, exists(logs), exists(s.env.ResultsDir))
		}
		if !keep && exists(filepath.Dir(logs)) {
			t.Error("the emptied NetworkDoctor folder that held the run logs is still there")
		}
	}
}

// ND-290: a folder of the person's own with an icon's name is never removed.
func TestUninstallLeavesAFolderOfTheSameName(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		s := newScratch(t, goos)
		Install(s.env, Choice{}, io.Discard)
		l := LayoutFor(s.env)
		os.Remove(l.DeskIcon)
		os.MkdirAll(l.DeskIcon, 0o755)
		mine := filepath.Join(l.DeskIcon, "mine.txt")
		os.WriteFile(mine, []byte("mine"), 0o644)
		Uninstall(s.env, false, io.Discard)
		if !exists(mine) {
			t.Errorf("%s: the person's own folder was removed", goos)
		}
		if exists(l.Program) {
			t.Errorf("%s: the program is still there", goos)
		}
	}
}

// ND-291: the removal page asks no keep-or-delete question; it says everything goes, and its one
// button, sent with an empty answer, deletes the results folder.
func TestUninstallPageAsksNoQuestion(t *testing.T) {
	tx := uninstallTexts("linux", "/r")
	all := strings.Join([]string{tx.Ask, tx.Note, tx.Button, tx.Working}, " ")
	for _, want := range []string{"everything", "results", "alert log", "devices", "settings", "starts fresh",
		"tries to put back any setting a fix changed"} {
		if !strings.Contains(all, want) {
			t.Errorf("the removal page does not say %q: %+v", want, tx)
		}
	}
	if strings.Contains(strings.ToLower(all), "keep") {
		t.Errorf("the removal page still talks of keeping: %+v", tx)
	}
	page := string(pageHTML)
	for _, old := range []string{`name="keep"`, `id="keep"`, `id="drop"`, "keepText", "dropText", "u.keep", "u.delete"} {
		if strings.Contains(page, old) {
			t.Errorf("the page still holds the keep choice: %s", old)
		}
	}
	if !strings.Contains(page, "askNote") {
		t.Error("the page does not show the removal note")
	}
	s := newScratch(t, "linux")
	res := makeResults(t, s)
	Install(s.env, Choice{}, io.Discard)
	base, _, _ := started(t, s, ModeUninstall)
	u, _ := url.Parse(base)
	r := send(t, base, req{method: "POST", path: u.Path + "api/uninstall", body: `{}`, header: true})
	var a answer
	if err := json.NewDecoder(r.Body).Decode(&a); err != nil || r.StatusCode != 200 || !a.OK {
		t.Fatalf("remove: %d %+v %v", r.StatusCode, a, err)
	}
	if exists(res) || exists(s.env.ResultsDir) || exists(LayoutFor(s.env).Program) {
		t.Error("the page's removal left the results or the program")
	}
	if g := strings.Join(a.Lines, " "); !strings.Contains(g, unDeleted) || strings.Contains(strings.ToLower(g), "still in") {
		t.Errorf("finish lines: %q", g)
	}
}

// ND-291: uninstall --silent deletes the results; --keep-results keeps them (also with the old
// --delete-results, which alone only repeats the default); --keep-results without --silent is
// refused and removes nothing.
func TestUninstallSwitches(t *testing.T) {
	for _, tc := range []struct {
		args []string
		code int
		keep bool
	}{
		{[]string{"--silent"}, 0, false},
		{[]string{"--silent", "--keep-results"}, 0, true},
		{[]string{"--silent", "--delete-results"}, 0, false},
		{[]string{"--silent", "--keep-results", "--delete-results"}, 0, true},
		{[]string{"--keep-results"}, 2, true},
	} {
		s := newScratch(t, "linux")
		res := makeResults(t, s)
		Install(s.env, Choice{}, io.Discard)
		served := false
		s.env.OpenURL = func(string) error { served = true; return nil }
		var out bytes.Buffer
		if code := Main(t.Context(), "uninstall", tc.args, s.env, &out); code != tc.code {
			t.Fatalf("%v: exit %d, want %d: %s", tc.args, code, tc.code, out.String())
		}
		if served || exists(res) != tc.keep {
			t.Errorf("%v: page %v, results kept %v", tc.args, served, exists(res))
		}
		if exists(LayoutFor(s.env).Program) != (tc.code != 0) {
			t.Errorf("%v: program still there %v", tc.args, exists(LayoutFor(s.env).Program))
		}
		if tc.code != 0 && strings.Count(strings.TrimSpace(out.String()), "\n") != 0 {
			t.Errorf("%v: want one plain line, got %q", tc.args, out.String())
		}
		if tc.code == 0 && tc.keep && !strings.Contains(out.String(), s.env.ResultsDir) {
			t.Errorf("%v: the kept folder is not named: %q", tc.args, out.String())
		}
	}
}

// ND-292: install again puts back what is chosen and takes away what is not, results kept.
func TestInstallAgainRepairs(t *testing.T) {
	s := newScratch(t, "linux")
	res := makeResults(t, s)
	Install(s.env, Choice{Weekly: true, Alerts: true}, io.Discard)
	l := LayoutFor(s.env)
	os.Remove(l.Launcher)
	r := Install(s.env, Choice{Weekly: true}, io.Discard)
	if len(r.Problems) > 0 {
		t.Fatal(r.Problems)
	}
	if !exists(l.Launcher) {
		t.Error("a removed launcher was not put back")
	}
	last := s.p.plans[len(s.p.plans)-1]
	if !last.Weekly || last.Alerts {
		t.Errorf("second plan %+v: the alerts job must go", last)
	}
	if s.p.stops < 2 {
		t.Error("the running helper was not stopped before the program was replaced")
	}
	if read(t, res) != "saved\n" {
		t.Error("the results changed")
	}
	if c := choicesIn(t, l.ProgramDir); !c["scheduled"] || c["proactive"] {
		t.Errorf("choices after repair: %v", c)
	}
}

// ND-292: running the installed copy's own install does not wipe the program it runs from.
func TestInstallFromTheInstalledCopy(t *testing.T) {
	s := newScratch(t, "linux")
	Install(s.env, Choice{}, io.Discard)
	l := LayoutFor(s.env)
	s.env.Exe = l.Program
	if r := Install(s.env, Choice{}, io.Discard); len(r.Problems) > 0 {
		t.Fatal(r.Problems)
	}
	if read(t, l.Program) != "pretend program" {
		t.Error("the program was damaged")
	}
}

// ND-293: install --silent takes its parts from --parts, never fetches Ookla, and says one plain
// line on a failure.
func TestSilentInstall(t *testing.T) {
	for _, tc := range []struct {
		parts        string
		weekly, alrt bool
	}{{"", false, false}, {"--parts=scheduled", true, false}, {"--parts=proactive", false, true},
		{"--parts=scheduled,proactive", true, true}} {
		s := newScratch(t, "linux")
		served := false
		s.env.OpenURL = func(string) error { served = true; return nil }
		s.env.HTTP = failingClient(t)
		args := []string{"--silent"}
		if tc.parts != "" {
			args = append(args, tc.parts)
		}
		var out bytes.Buffer
		if code := Main(t.Context(), "install", args, s.env, &out); code != 0 {
			t.Fatalf("%v: exit %d: %s", args, code, out.String())
		}
		if served {
			t.Error("a page was opened")
		}
		pl := s.p.plans[0]
		if pl.Weekly != tc.weekly || pl.Alerts != tc.alrt {
			t.Errorf("%v: plan %+v", args, pl)
		}
		if c := choicesIn(t, LayoutFor(s.env).ProgramDir); c["ookla"] {
			t.Error("Ookla in a silent install")
		}
	}
	s := newScratch(t, "linux")
	s.p.applyErr = os.ErrPermission
	var out bytes.Buffer
	code := Main(t.Context(), "install", []string{"--silent", "--parts=scheduled"}, s.env, &out)
	lines := strings.Split(strings.TrimSpace(out.String()), "\n")
	if code == 0 || len(lines) != 1 || banned(lines[0], true) != "" {
		t.Errorf("failure: exit %d, %q", code, out.String())
	}
	out.Reset()
	if code := Main(t.Context(), "install", []string{"--silent", "--parts=everything"}, s.env, &out); code == 0 {
		t.Error("an unknown part was taken")
	}
}

// Windows has its own setup file: install and uninstall say so and change nothing.
func TestInstallRefusedOnWindows(t *testing.T) {
	s := newScratch(t, "windows")
	for _, cmd := range []string{"install", "uninstall"} {
		var out bytes.Buffer
		if code := Main(t.Context(), cmd, []string{"--silent"}, s.env, &out); code == 0 || !strings.Contains(out.String(), "Windows") {
			t.Errorf("%s on Windows: %d %q", cmd, code, out.String())
		}
	}
	if len(s.p.plans) > 0 || len(s.p.cmds) > 0 {
		t.Error("something ran on Windows")
	}
	var out bytes.Buffer
	dir := t.TempDir()
	if code := Main(t.Context(), "install", []string{"--write-setup-files", dir, "--system", "linux"}, s.env, &out); code != 0 {
		t.Errorf("write-setup-files: %d %s", code, out.String())
	}
	if !exists(filepath.Join(dir, "Install Network Doctor.desktop")) {
		t.Error("the setup files were not written")
	}
}
