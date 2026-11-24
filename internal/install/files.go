package install

import (
	_ "embed"
	"errors"
	"fmt"
	"html"
	"os"
	"path/filepath"
	"strings"
)

// Names of what the install makes. The program keeps its plain name, netdoctor, everywhere.
const (
	appName           = "Network Doctor"
	programName       = "netdoctor"
	bundleID          = "com.networkdoctor.app"
	uninstallID       = "com.networkdoctor.uninstall"
	setupID           = "com.networkdoctor.setup"
	uninstallName     = "Uninstall Network Doctor"
	setupAppName      = "Network Doctor Setup"
	linuxLauncher     = "network-doctor.desktop"
	linuxUninst       = "network-doctor-uninstall.desktop"
	linuxSetup        = "Install Network Doctor.desktop"
	macOpenPage       = "How to open Network Doctor Setup.html"
	recordName        = "install-record.txt"
	resultsFolderName = "NetworkDoctor" // the only results folder name uninstall ever removes
	minMacVersion     = "12.0"          // the oldest macOS the Go version in go.mod builds for
	linuxIconName     = "network-wired"
)

// PlistInfo is what a Mac app's Info.plist says about it.
type PlistInfo struct {
	Name, ID, Executable, Version string
}

// MacInfoPlist is the Info.plist text of a Mac app with no Dock icon (LSUIElement true, ND-269).
func MacInfoPlist(p PlistInfo) string {
	esc := html.EscapeString
	return `<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>CFBundleName</key>
	<string>` + esc(p.Name) + `</string>
	<key>CFBundleDisplayName</key>
	<string>` + esc(p.Name) + `</string>
	<key>CFBundleIdentifier</key>
	<string>` + esc(p.ID) + `</string>
	<key>CFBundleExecutable</key>
	<string>` + esc(p.Executable) + `</string>
	<key>CFBundlePackageType</key>
	<string>APPL</string>
	<key>CFBundleVersion</key>
	<string>` + esc(p.Version) + `</string>
	<key>CFBundleShortVersionString</key>
	<string>` + esc(p.Version) + `</string>
	<key>CFBundleInfoDictionaryVersion</key>
	<string>6.0</string>
	<key>LSMinimumSystemVersion</key>
	<string>` + minMacVersion + `</string>
	<key>LSUIElement</key>
	<true/>
</dict>
</plist>
`
}

// shQuote quotes one word for the POSIX shell: inside single quotes nothing is special.
func shQuote(s string) string { return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'" }

// MacLauncherBeside is a Mac app's start file that runs the program in the same folder with one
// word (open for the icon, install for the setup app). It shows no window of its own.
func MacLauncherBeside(arg string) string {
	return "#!/bin/sh\n# Network Doctor: starts the program beside this file. No window opens for this file.\n" +
		`exec "$(dirname "$0")/` + programName + `" ` + arg + "\n"
}

// MacLauncherAt is a Mac app's start file that runs the program at a fixed place with one word.
func MacLauncherAt(program, arg string) string {
	return "#!/bin/sh\n# Network Doctor: starts the installed program. No window opens for this file.\n" +
		"exec " + shQuote(filepath.ToSlash(program)) + " " + arg + "\n"
}

// desktopArg writes one argument of a desktop entry's Exec line: a plain word as it is, anything
// else in double quotes with " ` $ \ escaped, then the string escape for \ on top; % is doubled.
func desktopArg(s string) string {
	s = strings.ReplaceAll(s, "%", "%%")
	if s != "" && !strings.ContainsAny(s, " \t\n\"'\\><~|&;$*?#()`=") {
		return s
	}
	var b strings.Builder
	b.WriteByte('"')
	for _, r := range s {
		switch r {
		case '"', '`', '$':
			b.WriteString(`\` + string(r))
		case '\\':
			b.WriteString(`\\`)
		default:
			b.WriteRune(r)
		}
	}
	b.WriteByte('"')
	return strings.ReplaceAll(b.String(), `\`, `\\`)
}

// DesktopEntry is a Linux launcher with no terminal window. exec is the program and its words.
func DesktopEntry(name, comment string, exec ...string) string {
	args := make([]string, len(exec))
	for i, a := range exec {
		args[i] = desktopArg(a)
	}
	return "[Desktop Entry]\nType=Application\nVersion=1.0\nName=" + name + "\nComment=" + comment +
		"\nExec=" + strings.Join(args, " ") + "\nIcon=" + linuxIconName + "\nTerminal=false\nCategories=Network;Utility;\n"
}

// linuxSetupEntry is the Linux setup launcher (ND-278). A desktop entry has no folder of its own,
// so it asks the shell for the folder of the launcher file (%k) and runs the program there; the
// place arrives as an argument and is never part of the script.
func linuxSetupEntry() string {
	return "[Desktop Entry]\nType=Application\nVersion=1.0\nName=Install Network Doctor\n" +
		"Comment=Set up Network Doctor for you\n" +
		`Exec=sh -c "exec \\"\\$(dirname \\"\\$1\\")/netdoctor\\" install" sh %k` +
		"\nIcon=" + linuxIconName + "\nTerminal=false\nCategories=Network;Utility;\n"
}

// macOpenHTML is the page that comes with the Mac setup file (ND-277), kept as its own file.
//
//go:embed macopen.html
var macOpenHTML string

// MacOpenPage is the page that comes with the Mac setup file (ND-277). It loads nothing.
func MacOpenPage() string { return macOpenHTML }

// SetupFile is one file of a setup download, with its path inside the download (with /).
type SetupFile struct {
	Path string
	Mode os.FileMode
	Text string
}

// SetupFiles are the text files of the Mac or Linux setup download (ND-276 to ND-278). The
// packaging step adds the program at SetupProgramPath and the guide at SetupGuidePath.
func SetupFiles(system string) ([]SetupFile, error) {
	switch system {
	case "mac", "darwin":
		app := setupAppName + ".app/Contents/"
		return []SetupFile{
			{Path: app + "Info.plist", Mode: 0o644,
				Text: MacInfoPlist(PlistInfo{Name: setupAppName, ID: setupID, Executable: setupAppName, Version: "1.0"})},
			{Path: app + "MacOS/" + setupAppName, Mode: 0o755, Text: MacLauncherBeside("install")},
			{Path: macOpenPage, Mode: 0o644, Text: MacOpenPage()},
		}, nil
	case "linux":
		return []SetupFile{{Path: linuxSetup, Mode: 0o755, Text: linuxSetupEntry()}}, nil
	}
	return nil, fmt.Errorf("no setup files for %q: use mac or linux", system)
}

// SetupProgramPath is where the packaging step puts the program inside the download.
func SetupProgramPath(system string) string {
	if system == "mac" || system == "darwin" {
		return setupAppName + ".app/Contents/MacOS/" + programName
	}
	return programName
}

// SetupGuidePath is where the packaging step puts GUIDE.md inside the download. On a Mac it sits
// in the setup app, since the zip holds only the app and the open page.
func SetupGuidePath(system string) string {
	if system == "mac" || system == "darwin" {
		return setupAppName + ".app/Contents/Resources/GUIDE.md"
	}
	return "GUIDE.md"
}

// WriteSetupFiles writes the setup download's text files under dir.
func WriteSetupFiles(dir, system string) error {
	files, err := SetupFiles(system)
	if err != nil {
		return err
	}
	for _, f := range files {
		p := filepath.Join(dir, filepath.FromSlash(f.Path))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(p, []byte(f.Text), f.Mode); err != nil {
			return err
		}
		if err := os.Chmod(p, f.Mode); err != nil {
			return err
		}
	}
	return nil
}

// Layout is where everything goes for one person on one system.
type Layout struct {
	System     string // darwin or linux
	Home       string
	ProgramDir string // the folder of the program, choices.json, the marker, the guide and ookla
	Program    string
	AppBundle  string // Mac: ~/Applications/Network Doctor.app
	UninstApp  string // Mac: ~/Applications/Uninstall Network Doctor.app
	Launcher   string // Mac: the app's start file; Linux: the menu launcher
	UninstEnt  string // Linux: the uninstall entry in the menu
	Desktop    string // the Desktop folder
	DeskIcon   string // the Desktop alias, link or launcher copy
	Results    string
}

// Made lists the icons, launchers and apps the install makes, which uninstall removes.
func (l Layout) Made() []string {
	if l.System == "darwin" {
		return []string{l.AppBundle, l.UninstApp, l.DeskIcon}
	}
	return []string{l.ProgramDir, l.Launcher, l.UninstEnt, l.DeskIcon}
}

// LayoutFor works out the places for e. On Linux the Desktop folder is the one xdg-user-dir names,
// when it is inside home, else ~/Desktop (ND-270, ND-274).
func LayoutFor(e Env) Layout {
	l := Layout{System: e.GOOS, Home: e.Home, Results: e.ResultsDir, Desktop: filepath.Join(e.Home, "Desktop")}
	if e.GOOS == "darwin" {
		l.AppBundle = filepath.Join(e.Home, "Applications", appName+".app")
		l.UninstApp = filepath.Join(e.Home, "Applications", uninstallName+".app")
		l.ProgramDir = filepath.Join(l.AppBundle, "Contents", "MacOS")
		l.Launcher = filepath.Join(l.ProgramDir, appName)
		l.DeskIcon = filepath.Join(l.Desktop, appName)
	} else {
		l.ProgramDir = filepath.Join(e.Home, ".local", "share", "NetworkDoctor")
		apps := filepath.Join(e.Home, ".local", "share", "applications")
		l.Launcher = filepath.Join(apps, linuxLauncher)
		l.UninstEnt = filepath.Join(apps, linuxUninst)
		if e.Run != nil {
			if out, err := e.Run("xdg-user-dir", "DESKTOP"); err == nil {
				d := filepath.Clean(strings.TrimSpace(out))
				if filepath.IsAbs(d) && inside(e.Home, d) {
					l.Desktop = d
				}
			}
		}
		l.DeskIcon = filepath.Join(l.Desktop, linuxLauncher)
	}
	l.Program = filepath.Join(l.ProgramDir, programName)
	return l
}

// inside says whether p is strictly inside dir.
func inside(dir, p string) bool {
	rel, err := filepath.Rel(filepath.Clean(dir), filepath.Clean(p))
	return err == nil && rel != "." && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		!filepath.IsAbs(rel)
}

// realPath follows links in the part of p that exists, then adds the rest, so a folder that is a
// link to somewhere else is judged by where it really is.
func realPath(p string) string {
	p = filepath.Clean(p)
	rest := ""
	for {
		if r, err := filepath.EvalSymlinks(p); err == nil {
			return filepath.Join(r, rest)
		}
		parent := filepath.Dir(p)
		if parent == p {
			return filepath.Join(p, rest)
		}
		rest = filepath.Join(filepath.Base(p), rest)
		p = parent
	}
}

// insideReal says whether the entry p lands inside home: its folder is judged with links followed;
// p itself may be a link, and only the link is ever written over or removed, never its target.
func insideReal(home, p string) bool {
	if !inside(home, p) {
		return false
	}
	return inside(realPath(home), filepath.Join(realPath(filepath.Dir(p)), filepath.Base(p)))
}

// errOutsideHome stops any write or removal outside the person's home folder (ND-274).
var errOutsideHome = errors.New("refused: outside the home folder")
