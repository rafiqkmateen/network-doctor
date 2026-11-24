package app

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// ND-300: with no browser, a pretend Mac shows an osascript dialog, a pretend Linux shows zenity,
// else kdialog, else only prints. The address is its own argument (never script text), and the
// program ends when the person presses OK.
func TestNoBrowserDialogPerSystem(t *testing.T) {
	const addr = `http://127.0.0.1:50123/?t=a"b'c&d`
	for _, c := range []struct {
		name, goos string
		have       map[string]bool
		fails      map[string]bool
		want       string // the program that showed it, or "" for printed only
	}{
		{"mac", "darwin", map[string]bool{"osascript": true}, nil, "osascript"},
		{"mac without osascript", "darwin", map[string]bool{}, nil, ""},
		{"linux zenity", "linux", map[string]bool{"zenity": true, "kdialog": true}, nil, "zenity"},
		{"linux only kdialog", "linux", map[string]bool{"kdialog": true}, nil, "kdialog"},
		{"linux zenity fails", "linux", map[string]bool{"zenity": true, "kdialog": true}, map[string]bool{"zenity": true}, "kdialog"},
		{"linux neither", "linux", map[string]bool{}, nil, ""},
	} {
		var ran [][]string
		look := func(n string) (string, error) {
			if c.have[n] {
				return "/usr/bin/" + n, nil
			}
			return "", errors.New("not found")
		}
		run := func(n string, args ...string) error {
			ran = append(ran, append([]string{n}, args...))
			if c.fails[n] {
				return errors.New("no display")
			}
			return nil
		}
		var out bytes.Buffer
		ended := dialogNoBrowser(c.goos, look, run, addr, &out)
		if !strings.Contains(out.String(), addr) {
			t.Errorf("%s: the address must always be printed too: %q", c.name, out.String())
		}
		if c.want == "" {
			if ended || len(ran) != 0 {
				t.Errorf("%s: with no dialog program it only prints and keeps waiting: ended=%v ran=%v", c.name, ended, ran)
			}
			continue
		}
		if !ended {
			t.Errorf("%s: pressing OK must end the program", c.name)
		}
		last := ran[len(ran)-1]
		if last[0] != c.want {
			t.Errorf("%s: shown by %q, want %q", c.name, last[0], c.want)
		}
		switch c.want {
		case "osascript":
			if last[len(last)-1] != addr || !strings.Contains(strings.Join(last, " "), "item 1 of argv") {
				t.Errorf("mac: the address must be its own last argument: %q", last)
			}
			for _, a := range last[:len(last)-1] {
				if strings.Contains(a, "127.0.0.1") {
					t.Errorf("mac: the address leaked into the script: %q", a)
				}
			}
		case "zenity":
			if !contains(last, "--no-markup") || !contains(last, "--text="+noBrowserText(addr)) {
				t.Errorf("zenity call: %q", last)
			}
		case "kdialog":
			if last[len(last)-1] != noBrowserText(addr) {
				t.Errorf("kdialog call: %q", last)
			}
		}
	}
}

// closedBox is how a box program ends when the person closes it with its X (exit code 1).
type closedBox struct{}

func (closedBox) Error() string { return "exit status 1" }
func (closedBox) ExitCode() int { return 1 }

// ND-300: closing the zenity box with its X ends it like OK; no second box opens. Only a zenity
// that is missing or cannot run passes on to kdialog.
func TestClosingTheZenityBoxOpensNoSecondBox(t *testing.T) {
	var ran []string
	look := func(n string) (string, error) { return "/usr/bin/" + n, nil }
	run := func(n string, args ...string) error {
		ran = append(ran, n)
		if n == "zenity" {
			return closedBox{}
		}
		return nil
	}
	if !dialogNoBrowser("linux", look, run, "http://127.0.0.1:1/", &bytes.Buffer{}) || strings.Join(ran, ",") != "zenity" {
		t.Errorf("closing zenity must end it with no second box: %v", ran)
	}
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

// ND-05 and ND-280: the dialog's words are plain and hold no terminal words.
func TestNoBrowserWordsArePlain(t *testing.T) {
	for _, s := range []string{noBrowserLead, noBrowserOpen, noBrowserDone} {
		if g := core.GradeLevel(s); g > 8.0 {
			t.Errorf("grade %.1f too high: %q", g, s)
		}
	}
	low := strings.ToLower(noBrowserText("x") + strings.Join(macDialogScript, " "))
	for _, w := range []string{"type", "typing", "terminal", "command", "cron", "sudo", "systemd"} {
		for _, f := range strings.FieldsFunc(low, func(r rune) bool { return !('a' <= r && r <= 'z') }) {
			if f == w {
				t.Errorf("the dialog holds the word %q", w)
			}
		}
	}
}
