package install

import (
	"io"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// ND-269: the Mac app has no Dock icon, is named Network Doctor and opens the page.
func TestMacAppBundleLayout(t *testing.T) {
	s := newScratch(t, "darwin")
	r := Install(s.env, Choice{}, io.Discard)
	if len(r.Problems) > 0 {
		t.Fatalf("install problems: %v", r.Problems)
	}
	app := filepath.Join(s.home, "Applications", "Network Doctor.app")
	plist := read(t, filepath.Join(app, "Contents", "Info.plist"))
	for _, want := range []string{
		"<key>LSUIElement</key>\n\t<true/>",
		"<key>CFBundleName</key>\n\t<string>Network Doctor</string>",
		"<key>CFBundleExecutable</key>\n\t<string>Network Doctor</string>",
		"<key>CFBundlePackageType</key>\n\t<string>APPL</string>",
	} {
		if !strings.Contains(plist, want) {
			t.Errorf("Info.plist lacks %q:\n%s", want, plist)
		}
	}
	start := filepath.Join(app, "Contents", "MacOS", "Network Doctor")
	launcher := read(t, start)
	if !strings.HasPrefix(launcher, "#!/bin/sh\n") || !strings.Contains(launcher, `exec "$(dirname "$0")/netdoctor" open`) {
		t.Errorf("start file does not run the program with open:\n%s", launcher)
	}
	if strings.Contains(strings.ToLower(launcher), "terminal") || !runnable(t, start) {
		t.Errorf("start file must run by itself with no terminal:\n%s", launcher)
	}
	if read(t, filepath.Join(app, "Contents", "MacOS", "netdoctor")) != "simulated program" {
		t.Error("the program was not copied into the app")
	}
	// The Finder alias failed (simulated), so the Desktop has a link to the app.
	desk := filepath.Join(s.home, "Desktop", "Network Doctor")
	if got := read(t, desk); got != "link to "+app {
		t.Errorf("Desktop link: %q", got)
	}
	if len(s.p.ran("osascript")) == 0 {
		t.Error("the Finder alias was not tried first")
	}
	uninst := filepath.Join(s.home, "Applications", "Uninstall Network Doctor.app")
	ul := read(t, filepath.Join(uninst, "Contents", "MacOS", "Uninstall Network Doctor"))
	if !strings.Contains(ul, filepath.ToSlash(filepath.Join(app, "Contents", "MacOS", "netdoctor"))+"' uninstall") {
		t.Errorf("uninstall app does not run uninstall:\n%s", ul)
	}
	if !strings.Contains(read(t, filepath.Join(uninst, "Contents", "Info.plist")), "<key>LSUIElement</key>\n\t<true/>") {
		t.Error("uninstall app shows a Dock icon")
	}
}

// ND-269: the Finder alias, when it works, is made from arguments, never from script text.
func TestMacAliasPathsAreArguments(t *testing.T) {
	s := newScratch(t, "darwin")
	delete(s.p.fails, "osascript")
	Install(s.env, Choice{}, io.Discard)
	calls := s.p.ran("osascript")
	if len(calls) != 1 {
		t.Fatalf("osascript calls: %v", calls)
	}
	c := calls[0]
	app := filepath.Join(s.home, "Applications", "Network Doctor.app")
	n := len(c)
	if c[n-3] != app || c[n-2] != filepath.Join(s.home, "Desktop") || c[n-1] != "Network Doctor" {
		t.Errorf("alias arguments: %q", c)
	}
	for _, a := range c[:n-3] {
		if strings.Contains(a, s.home) {
			t.Errorf("a path is written into the script: %q", a)
		}
	}
}

// ND-270: the Linux launcher, its Desktop copy and the uninstall entry.
func TestLinuxLauncherFiles(t *testing.T) {
	s := newScratch(t, "linux")
	r := Install(s.env, Choice{}, io.Discard)
	if len(r.Problems) > 0 {
		t.Fatalf("install problems: %v", r.Problems)
	}
	prog := filepath.Join(s.home, ".local", "share", "NetworkDoctor", "netdoctor")
	if read(t, prog) != "simulated program" || !runnable(t, prog) {
		t.Error("the program is not in ~/.local/share/NetworkDoctor")
	}
	apps := filepath.Join(s.home, ".local", "share", "applications")
	l := read(t, filepath.Join(apps, "network-doctor.desktop"))
	if entry(l, "Name") != "Network Doctor" || entry(l, "Terminal") != "false" || entry(l, "Type") != "Application" {
		t.Errorf("launcher:\n%s", l)
	}
	args := execArgs(t, entry(l, "Exec"))
	if len(args) != 2 || args[0] != filepath.ToSlash(prog) || args[1] != "open" || !strings.HasSuffix(entry(l, "Exec"), " open") {
		t.Errorf("Exec runs %q", args)
	}
	desk := filepath.Join(s.home, "Desktop", "network-doctor.desktop")
	if read(t, desk) != l || !runnable(t, desk) {
		t.Error("the Desktop copy is not the same runnable launcher")
	}
	if g := s.p.ran("gio"); len(g) != 1 || strings.Join(g[0][1:], " ") != "set "+desk+" metadata::trusted true" {
		t.Errorf("gio trusted flag: %v", g)
	}
	u := read(t, filepath.Join(apps, "network-doctor-uninstall.desktop"))
	ua := execArgs(t, entry(u, "Exec"))
	if entry(u, "Name") != "Uninstall Network Doctor" || entry(u, "Terminal") != "false" || len(ua) != 2 || ua[1] != "uninstall" {
		t.Errorf("uninstall entry:\n%s", u)
	}
}

// ND-270: the Desktop folder is the one xdg-user-dir names, but never one outside home.
func TestLinuxDesktopFolder(t *testing.T) {
	s := newScratch(t, "linux")
	delete(s.p.fails, "xdg-user-dir")
	s.p.replies["xdg-user-dir"] = filepath.Join(s.home, "Schreibtisch")
	Install(s.env, Choice{}, io.Discard)
	if !exists(filepath.Join(s.home, "Schreibtisch", "network-doctor.desktop")) {
		t.Error("the named Desktop folder was not used")
	}
	s2 := newScratch(t, "linux")
	delete(s2.p.fails, "xdg-user-dir")
	s2.p.replies["xdg-user-dir"] = filepath.Join(s2.root, "elsewhere")
	Install(s2.env, Choice{}, io.Discard)
	if exists(filepath.Join(s2.root, "elsewhere")) || !exists(filepath.Join(s2.home, "Desktop", "network-doctor.desktop")) {
		t.Error("a Desktop folder outside home was used")
	}
}

// ND-271: both setup files start the install with no terminal window.
func TestSetupFilesStartInstall(t *testing.T) {
	mac := setupFile(t, "mac", "Network Doctor Setup.app/Contents/MacOS/Network Doctor Setup")
	if !strings.Contains(mac.Text, `exec "$(dirname "$0")/netdoctor" install`) || mac.Mode&0o111 == 0 {
		t.Errorf("Mac setup start file:\n%s", mac.Text)
	}
	plist := setupFile(t, "mac", "Network Doctor Setup.app/Contents/Info.plist").Text
	if !strings.Contains(plist, "<key>LSUIElement</key>\n\t<true/>") ||
		!strings.Contains(plist, "<key>CFBundleExecutable</key>\n\t<string>Network Doctor Setup</string>") {
		t.Errorf("Mac setup Info.plist:\n%s", plist)
	}
	lin := setupFile(t, "linux", "Install Network Doctor.desktop")
	if entry(lin.Text, "Terminal") != "false" || !strings.Contains(entry(lin.Text, "Exec"), "install") {
		t.Errorf("Linux setup launcher:\n%s", lin.Text)
	}
}

func setupFile(t *testing.T, system, path string) SetupFile {
	t.Helper()
	files, err := SetupFiles(system)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range files {
		if f.Path == path {
			return f
		}
	}
	t.Fatalf("%s setup files lack %s", system, path)
	return SetupFile{}
}

// ND-278: the Linux setup launcher runs the program beside it with install, wherever it is unpacked.
func TestLinuxSetupLauncher(t *testing.T) {
	f := setupFile(t, "linux", "Install Network Doctor.desktop")
	if entry(f.Text, "Name") != "Install Network Doctor" || entry(f.Text, "Type") != "Application" ||
		entry(f.Text, "Terminal") != "false" || f.Mode&0o111 == 0 {
		t.Errorf("launcher:\n%s", f.Text)
	}
	args := execArgs(t, entry(f.Text, "Exec"))
	// sh -c SCRIPT sh %k: the launcher's own place comes in as $1, never inside the script text.
	if len(args) != 5 || args[0] != "sh" || args[1] != "-c" || args[3] != "sh" || args[4] != "%k" {
		t.Fatalf("Exec arguments: %q", args)
	}
	if args[2] != `exec "$(dirname "$1")/netdoctor" install` {
		t.Errorf("script: %q", args[2])
	}
	// The packaging script puts the program and the guide beside it.
	files, _ := SetupFiles("linux")
	if len(files) != 1 {
		t.Errorf("Linux setup files: %v", files)
	}
	if p := SetupProgramPath("linux"); p != "netdoctor" {
		t.Errorf("program path %q", p)
	}
	if p := SetupGuidePath("linux"); p != "GUIDE.md" {
		t.Errorf("guide path %q", p)
	}
}

// ND-276 layout: the Mac zip holds the setup app and the page; the program goes inside the app.
func TestMacSetupLayout(t *testing.T) {
	files, err := SetupFiles("mac")
	if err != nil {
		t.Fatal(err)
	}
	var paths []string
	for _, f := range files {
		paths = append(paths, f.Path)
	}
	want := []string{"Network Doctor Setup.app/Contents/Info.plist",
		"Network Doctor Setup.app/Contents/MacOS/Network Doctor Setup", "How to open Network Doctor Setup.html"}
	if strings.Join(paths, "|") != strings.Join(want, "|") {
		t.Errorf("Mac setup files: %q", paths)
	}
	if SetupProgramPath("mac") != "Network Doctor Setup.app/Contents/MacOS/netdoctor" ||
		SetupGuidePath("mac") != "Network Doctor Setup.app/Contents/Resources/GUIDE.md" {
		t.Error("program or guide place in the Mac setup app")
	}
	if _, err := SetupFiles("windows"); err == nil {
		t.Error("Windows has its own setup file")
	}
}

// The packaging step writes the files to disk with one call.
func TestWriteSetupFiles(t *testing.T) {
	dir := t.TempDir()
	for _, sys := range []string{"mac", "linux"} {
		if err := WriteSetupFiles(dir, sys); err != nil {
			t.Fatal(err)
		}
	}
	for _, p := range []string{"Network Doctor Setup.app/Contents/Info.plist", "How to open Network Doctor Setup.html",
		"Install Network Doctor.desktop"} {
		if !exists(filepath.Join(dir, filepath.FromSlash(p))) {
			t.Errorf("%s not written", p)
		}
	}
	if !runnable(t, filepath.Join(dir, "Install Network Doctor.desktop")) {
		t.Error("the Linux launcher is not runnable")
	}
}

// ND-277: the Mac open page gives the four steps in order and loads nothing from outside.
func TestMacOpenPageWords(t *testing.T) {
	page := MacOpenPage()
	text := visibleText(page)
	at := -1
	for _, w := range []string{"System Settings", "Privacy and Security", "Open Anyway", "Mac password", "USB stick", "shared network folder"} {
		i := strings.Index(text, w)
		if i < 0 {
			t.Fatalf("missing %q:\n%s", w, text)
		}
		if i < at {
			t.Errorf("%q is out of order", w)
		}
		at = i
	}
	if i, j := strings.Index(text, "open the setup file once"), strings.Index(text, "System Settings"); i < 0 || i > j {
		t.Errorf("the first step must be to try to open the setup file once:\n%s", text)
	}
	if strings.Contains(text, "AirDrop") {
		t.Error("AirDrop files are checked too, so the page must not say otherwise")
	}
	low := strings.ToLower(text)
	if strings.Contains(low, "right-click") || strings.Contains(low, "control-click") || banned(text, true) != "" {
		t.Errorf("forbidden words: %s", banned(text, true))
	}
	if regexp.MustCompile(`(?i)(src|href)\s*=|https?:|url\(|@import`).MatchString(page) {
		t.Error("the page refers to something outside the file")
	}
	for _, p := range paragraphs(text) {
		if g := core.GradeLevel(p); g > 8 {
			t.Errorf("grade %.1f: %q", g, p)
		}
	}
}

// ND-279: the finish screen says what to do next with the system's own names.
func TestFinishPageSaysWhatToDoNextOnMacAndLinux(t *testing.T) {
	for _, goos := range []string{"darwin", "linux"} {
		for _, c := range []Choice{{}, {Weekly: true}, {Alerts: true}, {Weekly: true, Alerts: true}} {
			prog, res := "/h/prog folder", "/h/results folder"
			lines := FinishLines(goos, prog, res, c, Result{})
			all := strings.Join(lines, "\n")
			want := []string{"Network Doctor is installed.",
				"A page opens in your web browser, runs the check and shows the answer. Close the browser tab when you are done.",
				"The program is in " + prog + ".", "Your results are saved in " + res + ".",
				"It takes away everything, your saved results and settings too, so a new install starts fresh. " +
					"It first tries to put back any setting a fix changed."}
			if goos == "darwin" {
				want = append(want, "To run a check, press the Network Doctor icon on your Desktop or in your Applications folder.",
					"To remove it, open Uninstall Network Doctor in your Applications folder.")
			} else {
				want = append(want, "To run a check, press the Network Doctor icon on your Desktop or in your applications menu.",
					"To remove it, open Uninstall Network Doctor in your applications menu.",
					`right-click it and choose "Allow Launching"`, "The menu launcher needs nothing.")
			}
			if c.Weekly {
				want = append(want, "Weekly check: on.")
			} else {
				want = append(want, "Weekly check: not chosen.")
			}
			if c.Alerts {
				want = append(want, "Alerts: on. They start the next time you log in.")
			} else {
				want = append(want, "Alerts: not chosen.")
			}
			for _, w := range want {
				if !strings.Contains(all, w) {
					t.Errorf("%s %+v lacks %q:\n%s", goos, c, w, all)
				}
			}
			if strings.Contains(all, "GUIDE.md") {
				t.Errorf("%s: the finish screen names GUIDE.md; the page's guide button replaces it:\n%s", goos, all)
			}
			if strings.Contains(strings.ToLower(all), "keep your saved results") {
				t.Errorf("%s: the finish screen still says removal asks to keep the results:\n%s", goos, all)
			}
			if goos == "darwin" && strings.Contains(all, "Allow Launching") {
				t.Error("the Mac has no Allow Launching step")
			}
		}
	}
	lines := strings.Join(FinishLines("linux", "/p", "/r", Choice{OoklaOffer: true, OoklaAgree: true},
		Result{OoklaTried: true}), "\n")
	if !strings.Contains(lines, "The speed tool was not added.") {
		t.Errorf("a failed speed tool is not told:\n%s", lines)
	}
}

// visibleText is the words a browser shows from an HTML text: no tags, no script or style.
func visibleText(html string) string {
	html = regexp.MustCompile(`(?is)<(script|style)\b.*?</(script|style)>`).ReplaceAllString(html, " ")
	html = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(html, " ")
	html = regexp.MustCompile(`(?s)<[^>]*>`).ReplaceAllString(html, "\n")
	r := strings.NewReplacer("&amp;", "&", "&quot;", `"`, "&#39;", "'", "&lt;", "<", "&gt;", ">", "&nbsp;", " ")
	return r.Replace(html)
}

func paragraphs(text string) []string {
	var out []string
	for _, p := range strings.Split(text, "\n") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
