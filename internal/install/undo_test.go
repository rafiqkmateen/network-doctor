package install

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/fix"
)

// proxyNote is the undo note the proxy fix leaves: the old setting, on with a server.
const proxyNote = `{"proxy":"{\"enabled\":true,\"server\":\"10.0.0.9:8080\"}"}`

// leftByFix is the proxy as the fix left it: off, with the same server.
var leftByFix = fix.ProxyState{Enabled: false, Server: "10.0.0.9:8080"}

// leftIn lists the files left in the results folder, with / between folder names.
func leftIn(t *testing.T, dir string) []string {
	t.Helper()
	var out []string
	filepath.Walk(dir, func(p string, fi os.FileInfo, err error) error {
		if err == nil && !fi.IsDir() {
			rel, _ := filepath.Rel(dir, p)
			out = append(out, filepath.ToSlash(rel))
		}
		return nil
	})
	sort.Strings(out)
	return out
}

// ND-316: a fix still applied is put back before anything is deleted, then removal goes on as
// before; with --keep-results it is put back too and everything is kept.
func TestUninstallPutsBackFixesFirst(t *testing.T) {
	for _, keep := range []bool{false, true} {
		s := newScratch(t, "linux") // the Linux web proxy is the person's own setting: no administrator
		makeResults(t, s)
		writeNotes(t, s, proxyNote)
		s.fixes.proxy = leftByFix
		Install(s.env, Choice{Weekly: true}, io.Discard)
		s.fixes.program = LayoutFor(s.env).Program
		if err := Uninstall(s.env, keep, io.Discard); err != nil {
			t.Fatalf("keep %v: %v", keep, err)
		}
		if !s.fixes.proxy.Enabled || s.fixes.proxy.Server != "10.0.0.9:8080" {
			t.Errorf("keep %v: the proxy was not put back: %+v", keep, s.fixes.proxy)
		}
		if len(s.fixes.whenSet) != 1 || !s.fixes.whenSet[0] {
			t.Errorf("keep %v: the fix must be put back while the program is still there: %v", keep, s.fixes.whenSet)
		}
		if exists(s.env.ResultsDir) != keep {
			t.Errorf("keep %v: results folder there %v", keep, exists(s.env.ResultsDir))
		}
		if exists(LayoutFor(s.env).Program) {
			t.Errorf("keep %v: the program is still there", keep)
		}
	}
}

// ND-316: when a fix cannot be put back, the program, icons and jobs still go, the results folder
// keeps only the undo notes, and the person is told which change, that its notes are kept and
// what to do next: by the function, by --silent (exit 1) and by the page.
func TestUninstallKeepsUndoNotesWhenAnUndoFails(t *testing.T) {
	cases := []struct {
		name, notes string
		proxy       fix.ProxyState
		setErr      error
		reason      string
	}{
		{"refused", proxyNote, leftByFix, errors.New("the computer said no"), fix.ReasonFailed},
		{"damaged notes", "not notes", leftByFix, nil, fix.ReasonNotes},
	}
	for _, c := range cases {
		for _, how := range []string{"function", "silent", "page"} {
			s := newScratch(t, "linux")
			makeResults(t, s)
			writeNotes(t, s, c.notes)
			s.fixes.proxy, s.fixes.setErr = c.proxy, c.setErr
			Install(s.env, Choice{Weekly: true, Alerts: true}, io.Discard)
			l := LayoutFor(s.env)
			var lines string
			switch how {
			case "function":
				err := Uninstall(s.env, false, io.Discard)
				var nu *NotUndoneError
				if !errors.As(err, &nu) || len(nu.Failures) != 1 || nu.Failures[0].Reason != c.reason {
					t.Fatalf("%s %s: %v", c.name, how, err)
				}
				lines = strings.Join(NotUndoneLines(nu.Failures, s.env.ResultsDir, false), " ")
			case "silent":
				var out bytes.Buffer
				if code := Main(t.Context(), "uninstall", []string{"--silent"}, s.env, &out); code != 1 {
					t.Errorf("%s %s: exit %d, want 1: %s", c.name, how, code, out.String())
				}
				lines = out.String()
			case "page":
				base, _, _ := started(t, s, ModeUninstall)
				u, _ := url.Parse(base)
				r := send(t, base, req{method: "POST", path: u.Path + "api/uninstall", body: `{}`, header: true})
				var a answer
				json.NewDecoder(r.Body).Decode(&a)
				if a.OK {
					t.Errorf("%s %s: the page said all went well", c.name, how)
				}
				lines = strings.Join(a.Lines, " ")
			}
			if got := leftIn(t, s.env.ResultsDir); strings.Join(got, ",") != "fix-enabled-here.txt,fix-log.txt,fix-saved.json" {
				t.Errorf("%s %s: left in the results folder: %v", c.name, how, got)
			}
			if exists(l.Program) || exists(l.Launcher) || s.p.removes != 1 {
				t.Errorf("%s %s: the program, a launcher or the jobs were left", c.name, how)
			}
			for _, want := range []string{"could not put back", s.env.ResultsDir, "install Network Doctor again", "by hand"} {
				if !strings.Contains(lines, want) {
					t.Errorf("%s %s: the person is not told %q: %s", c.name, how, want, lines)
				}
			}
			if c.reason != fix.ReasonNotes && !strings.Contains(lines, fix.UndoTitle("proxy")) {
				t.Errorf("%s %s: the change is not named: %s", c.name, how, lines)
			}
		}
	}
}

// ND-316: a setting the person changed by hand since the fix is settled at removal: left as they
// set it, its note dropped, nothing kept for it, exit 0 and the page says all went well, and the
// person is told it was left as they set it. Mixed with a fix that fails, only that one's note stays.
func TestUninstallSettlesAChangedSetting(t *testing.T) {
	changed := fix.ProxyState{Enabled: true, Server: "changed.example:1"}
	proxyOld := `{"enabled":true,"server":"10.0.0.9:8080"}`
	me := fix.CurrentAccount() // the notes were written by the account the removal runs as
	for _, mixed := range []bool{false, true} {
		for _, how := range []string{"function", "silent", "page"} {
			s := newScratch(t, "linux")
			makeResults(t, s)
			if mixed {
				// power saving reads "off" after the undo, so it fails
				writeNotes(t, s, notesFor(t, me, map[string]string{"proxy": proxyOld, "power-saving": "on"}))
			} else {
				writeNotes(t, s, notesFor(t, me, map[string]string{"proxy": proxyOld}))
			}
			s.fixes.proxy = changed
			Install(s.env, Choice{Weekly: true}, io.Discard)
			var lines string
			switch how {
			case "function":
				rep, err := UninstallReport(s.env, false, io.Discard)
				var nu *NotUndoneError
				if mixed != errors.As(err, &nu) || (mixed && (len(nu.Failures) != 1 || nu.Failures[0].Name != "power-saving")) {
					t.Fatalf("mixed %v %s: %v", mixed, how, err)
				}
				if len(rep.Settled) != 1 || rep.Settled[0] != "proxy" {
					t.Fatalf("mixed %v %s: settled %v", mixed, how, rep.Settled)
				}
				lines = strings.Join(SettledLines(rep.Settled), " ")
			case "silent":
				var out bytes.Buffer
				want := 0
				if mixed {
					want = 1
				}
				if code := Main(t.Context(), "uninstall", []string{"--silent"}, s.env, &out); code != want {
					t.Errorf("mixed %v %s: exit %d, want %d: %s", mixed, how, code, want, out.String())
				}
				lines = out.String()
			case "page":
				base, _, _ := started(t, s, ModeUninstall)
				u, _ := url.Parse(base)
				r := send(t, base, req{method: "POST", path: u.Path + "api/uninstall", body: `{}`, header: true})
				var a answer
				json.NewDecoder(r.Body).Decode(&a)
				if a.OK == mixed {
					t.Errorf("mixed %v %s: ok %v", mixed, how, a.OK)
				}
				lines = strings.Join(a.Lines, " ")
			}
			if s.fixes.proxy != changed || len(s.fixes.whenSet) != 0 {
				t.Errorf("mixed %v %s: the person's proxy was changed: %+v", mixed, how, s.fixes.proxy)
			}
			if !strings.Contains(lines, "left as you set it") {
				t.Errorf("mixed %v %s: not told it was left as set: %s", mixed, how, lines)
			}
			if mixed {
				notes := read(t, filepath.Join(s.env.ResultsDir, "fix-saved.json"))
				if strings.Contains(notes, "proxy") || !strings.Contains(notes, "power-saving") {
					t.Errorf("mixed %s: kept notes %s", how, notes)
				}
			} else if exists(s.env.ResultsDir) {
				t.Errorf("%s: a settled change must keep nothing: %v", how, leftIn(t, s.env.ResultsDir))
			}
			if !mixed && strings.Contains(lines, "could not put back") {
				t.Errorf("%s: a settled change is not a failure: %s", how, lines)
			}
		}
	}
	if g := core.GradeLevel(strings.Join(SettledLines([]string{"proxy", "power-saving", "public-dns"}), " ")); g > 8 {
		t.Errorf("settled lines grade %.1f", g)
	}
	if w := banned(strings.Join(SettledLines([]string{"proxy", "power-saving", "public-dns"}), " "), true); w != "" {
		t.Errorf("settled lines hold %q", w)
	}
}

// ND-316: a proxy that cannot be read, a note written by another account, or an old note with no
// account is never settled as changed by hand: a real failure, the note kept, the person told and
// nothing changed, by the function, by --silent and by the page.
func TestUninstallReadErrorOrOtherAccountIsAFailure(t *testing.T) {
	proxyOld := `{"enabled":true,"server":"10.0.0.9:8080"}`
	changed := fix.ProxyState{Enabled: true, Server: "changed.example:1"}
	for _, c := range []struct {
		name, account string
		readErr       error
	}{
		{"read error", fix.CurrentAccount(), errors.New("could not read the proxy")},
		{"another account", "PC\\someone-else (S-1-5-21-9)", nil},
		{"no account", "", nil},
	} {
		for _, how := range []string{"function", "silent", "page"} {
			s := newScratch(t, "linux")
			makeResults(t, s)
			writeNotes(t, s, notesFor(t, c.account, map[string]string{"proxy": proxyOld}))
			s.fixes.proxy, s.fixes.readErr = changed, c.readErr
			Install(s.env, Choice{}, io.Discard)
			var lines string
			failed := false
			switch how {
			case "function":
				rep, err := UninstallReport(s.env, false, io.Discard)
				var nu *NotUndoneError
				failed = errors.As(err, &nu) && len(rep.Settled) == 0
				if nu != nil {
					lines = strings.Join(NotUndoneLines(nu.Failures, nu.Notes, false), " ")
				}
			case "silent":
				var out bytes.Buffer
				failed = Main(t.Context(), "uninstall", []string{"--silent"}, s.env, &out) == 1
				lines = out.String()
			case "page":
				base, _, _ := started(t, s, ModeUninstall)
				u, _ := url.Parse(base)
				r := send(t, base, req{method: "POST", path: u.Path + "api/uninstall", body: `{}`, header: true})
				var a answer
				json.NewDecoder(r.Body).Decode(&a)
				failed = !a.OK
				lines = strings.Join(a.Lines, " ")
			}
			if !failed || strings.Contains(lines, "left as you set it") || !strings.Contains(lines, "could not put back") {
				t.Errorf("%s %s: must be a failure: %s", c.name, how, lines)
			}
			if !strings.Contains(read(t, filepath.Join(s.env.ResultsDir, "fix-saved.json")), `"proxy"`) {
				t.Errorf("%s %s: the note must stay", c.name, how)
			}
			if s.fixes.proxy != changed || len(s.fixes.whenSet) != 0 {
				t.Errorf("%s %s: the proxy was changed", c.name, how)
			}
		}
	}
}

// ND-316: the name servers count as changed by hand only when the adapter the fix changed is still
// listed and its servers differ; an adapter that is gone (offline, or another one in use) stays a
// real failure, its note kept and the person told, by the function, by --silent and by the page.
func TestUninstallMissingAdapterIsAFailure(t *testing.T) {
	note := notesFor(t, fix.CurrentAccount(), map[string]string{
		"public-dns": `[{"adapter":"Wi-Fi","if_index":12,"servers":["192.168.1.1"],"automatic":true,"in_use":true}]`})
	for _, gone := range []bool{true, false} {
		for _, how := range []string{"function", "silent", "page"} {
			s := newScratch(t, "linux")
			d := &simulatedDNS{simulatedFixes: s.fixes}
			if gone {
				d.dns = []fix.DNSState{{Adapter: "Other", IfIndex: 99, Servers: []string{"10.9.9.9"}, InUse: true}}
			} else {
				d.dns = []fix.DNSState{{Adapter: "Wi-Fi", IfIndex: 12, Servers: []string{"9.9.9.9"}, InUse: true}}
			}
			s.env.Fixes = d
			makeResults(t, s)
			writeNotes(t, s, note)
			Install(s.env, Choice{}, io.Discard)
			var lines string
			failed := false
			switch how {
			case "function":
				rep, err := UninstallReport(s.env, false, io.Discard)
				var nu *NotUndoneError
				failed = errors.As(err, &nu)
				lines = strings.Join(SettledLines(rep.Settled), " ")
				if failed {
					lines += strings.Join(NotUndoneLines(nu.Failures, nu.Notes, false), " ")
				}
			case "silent":
				var out bytes.Buffer
				failed = Main(t.Context(), "uninstall", []string{"--silent"}, s.env, &out) == 1
				lines = out.String()
			case "page":
				base, _, _ := started(t, s, ModeUninstall)
				u, _ := url.Parse(base)
				r := send(t, base, req{method: "POST", path: u.Path + "api/uninstall", body: `{}`, header: true})
				var a answer
				json.NewDecoder(r.Body).Decode(&a)
				failed = !a.OK
				lines = strings.Join(a.Lines, " ")
			}
			if d.sets != 0 {
				t.Errorf("gone %v %s: the name servers were changed", gone, how)
			}
			if failed != gone {
				t.Errorf("gone %v %s: failed %v: %s", gone, how, failed, lines)
			}
			if gone {
				if !strings.Contains(lines, "could not put back") || !strings.Contains(read(t, filepath.Join(s.env.ResultsDir, "fix-saved.json")), "public-dns") {
					t.Errorf("%s: an adapter that is gone must keep its note and be told: %s", how, lines)
				}
			} else if !strings.Contains(lines, "left as you set it") || exists(s.env.ResultsDir) {
				t.Errorf("%s: a present adapter changed by hand is settled: %s", how, lines)
			}
		}
	}
}

// ND-316, ND-274: on a Mac and on Linux removal never asks for administrator rights: a fix whose
// tool may ask for them there is not tried, and counts as not put back, with a plain reason.
func TestUninstallNeverAsksForAdminRights(t *testing.T) {
	for goos, notes := range map[string]string{
		"darwin": proxyNote,
		"linux":  `{"public-dns":"[]"}`,
	} {
		for _, keep := range []bool{false, true} {
			s := newScratch(t, goos)
			makeResults(t, s)
			writeNotes(t, s, notes)
			s.fixes.proxy = leftByFix
			Install(s.env, Choice{}, io.Discard)
			err := Uninstall(s.env, keep, io.Discard)
			var nu *NotUndoneError
			if !errors.As(err, &nu) || len(nu.Failures) != 1 || nu.Failures[0].Reason != fix.ReasonAdmin {
				t.Fatalf("%s keep %v: %v", goos, keep, err)
			}
			if len(s.fixes.calls) != 0 {
				t.Errorf("%s keep %v: a setting was read or changed: %v", goos, keep, s.fixes.calls)
			}
			for _, c := range s.p.cmds {
				if c[0] == "sudo" || c[0] == "pkexec" || c[0] == "networksetup" || c[0] == "nmcli" || c[0] == "osascript" && strings.Contains(strings.Join(c, " "), "administrator") {
					t.Errorf("%s: asked for administrator rights: %q", goos, c)
				}
			}
			if !strings.Contains(strings.Join(NotUndoneLines(nu.Failures, s.env.ResultsDir, keep), " "), "administrator rights") {
				t.Errorf("%s: the reason is not said", goos)
			}
			if keep && !exists(filepath.Join(s.env.ResultsDir, "weekly.csv")) {
				t.Errorf("%s: with keep the results must stay", goos)
			}
		}
	}
}

// ND-316, ND-05, ND-280: the words for a change not put back read at grade 8 or lower and hold no
// terminal word, for every reason.
func TestNotUndoneWordsArePlain(t *testing.T) {
	all := []fix.UndoFailure{{Name: "proxy", Reason: fix.ReasonAdmin}, {Name: "public-dns", Reason: fix.ReasonFailed},
		{Name: "power-saving", Reason: fix.ReasonFailed}, {Name: "proxy", Reason: fix.ReasonAccount}, {Reason: fix.ReasonNotes}}
	for _, kept := range []bool{false, true} {
		text := strings.Join(NotUndoneLines(all, "/r", kept), " ")
		if g := core.GradeLevel(text); g > 8 {
			t.Errorf("grade %.1f: %q", g, text)
		}
		if w := banned(text, true); w != "" {
			t.Errorf("%q in %q", w, text)
		}
	}
}
