package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/rafiqkmateen/network-doctor/internal/fix"
	"github.com/rafiqkmateen/network-doctor/internal/install"
)

// newFixSystem makes the real computer for fixes. Tests swap it for a simulated one.
var newFixSystem = func() fix.System { return fix.Real() }

const fixUse = `Use: fix list | apply NAME [--yes] | undo NAME | undo-all [--dir FOLDER] [--kept] [--note FILE] | enable-here   (add --json for one line a program can read)
Without --yes, apply only shows the setting now and what it would become.`

// fixReply is the one line printed by a fix command given --json.
type fixReply struct {
	OK         bool      `json:"ok"`
	Name       string    `json:"name,omitempty"`
	Title      string    `json:"title,omitempty"`
	Before     string    `json:"before,omitempty"`
	After      string    `json:"after,omitempty"`
	NeedsAdmin bool      `json:"needs_admin"`
	Applied    bool      `json:"applied"`
	Undone     bool      `json:"undone"`
	Error      string    `json:"error,omitempty"`
	Fixes      []fixItem `json:"fixes,omitempty"`
}

type fixItem struct {
	Name       string `json:"name"`
	Title      string `json:"title"`
	Now        string `json:"now"`
	NeedsAdmin bool   `json:"needs_admin"`
	Undoable   bool   `json:"undoable"`
}

func fixCmd(args []string, dir string, out io.Writer) int {
	asJSON := false
	var rest []string
	for _, a := range args {
		if a == "--json" {
			asJSON = true
		} else {
			rest = append(rest, a)
		}
	}
	args = rest
	say := func(j fixReply) {
		b, _ := json.Marshal(j)
		fmt.Fprintln(out, string(b))
	}
	fail := func(name string, err error, code int) int {
		if asJSON {
			say(fixReply{Name: name, Error: err.Error()})
		} else {
			fmt.Fprintln(out, err)
		}
		return code
	}
	if len(args) == 0 {
		fmt.Fprintln(out, fixUse)
		return 2
	}
	m := fix.NewManager(newFixSystem(), dir, fix.Options{Unattended: os.Getenv("NETDOCTOR_UNATTENDED") != ""})
	switch {
	case args[0] == "list":
		if asJSON {
			j := fixReply{OK: true}
			for _, in := range m.List() {
				j.Fixes = append(j.Fixes, fixItem{Name: in.Name, Title: in.Title, Now: in.Now, NeedsAdmin: in.NeedsAdmin, Undoable: in.Undoable})
			}
			say(j)
			return 0
		}
		for _, in := range m.List() {
			admin := ""
			if in.NeedsAdmin {
				admin = " (asks for administrator rights when you say yes)"
			}
			fmt.Fprintf(out, "%s - %s%s\n    now: %s\n", in.Name, in.Title, admin, in.Now)
		}
		return 0
	case args[0] == "enable-here":
		if err := m.EnableHere(); err != nil {
			fmt.Fprintln(out, "Could not switch fixes on:", err)
			return 2
		}
		fmt.Fprintln(out, "Fixes are switched on for this machine only.")
		return 0
	case args[0] == "undo-all":
		// What removal runs first (the Windows uninstaller calls it, ND-316): every fix still
		// applied is put back with undo; 1 and the plain lines, also written to --note, when one is
		// not. --dir names the results folder whatever NETDOCTOR_DIR says; --kept says the results
		// are kept, so the lines do not say they are deleted.
		notesDir, note, kept := dir, "", false
		for i := 1; i < len(args); i++ {
			switch {
			case args[i] == "--kept":
				kept = true
			case (args[i] == "--dir" || args[i] == "--note") && i+1 < len(args) && args[i+1] != "":
				if args[i] == "--dir" {
					notesDir = args[i+1]
				} else {
					note = args[i+1]
				}
				i++
			default:
				fmt.Fprintln(out, fixUse)
				return 2
			}
		}
		if notesDir != dir {
			m = fix.NewManager(newFixSystem(), notesDir, fix.Options{Unattended: os.Getenv("NETDOCTOR_UNATTENDED") != ""})
		}
		// A setting changed by hand since its fix is settled: left as set, its note dropped, said in
		// the lines, never a fault.
		failures, settled := m.UndoAll(nil)
		if len(failures) == 0 && len(settled) == 0 {
			fmt.Fprintln(out, "Every change of the fixes is put back, or there was none.")
			return 0
		}
		lines := install.SettledLines(settled)
		if len(failures) > 0 {
			lines = append(lines, install.NotUndoneLines(failures, notesDir, kept)...)
		}
		text := strings.Join(lines, "\n")
		fmt.Fprintln(out, text)
		if note != "" {
			if err := os.WriteFile(note, []byte(text+"\n"), 0o644); err != nil {
				fmt.Fprintln(out, "Could not write the note:", err)
			}
		}
		if len(failures) == 0 {
			return 0
		}
		return 1
	case args[0] == "undo" && len(args) == 2:
		name := args[1]
		if err := m.Undo(name); err != nil {
			code := 2
			if errors.Is(err, fix.ErrNothingToUndo) {
				code = 0
			}
			return fail(name, err, code)
		}
		if asJSON {
			say(fixReply{OK: true, Name: name, Title: fix.Title(name), NeedsAdmin: m.NeedsAdmin(name), Undone: true})
			return 0
		}
		fmt.Fprintf(out, "Put back: %s. Logged in %s.\n", name, m.Dir())
		return 0
	case args[0] == "apply" && len(args) >= 2:
		name := args[1]
		yes := len(args) == 3 && args[2] == "--yes"
		if len(args) > 3 || (len(args) == 3 && !yes) {
			fmt.Fprintln(out, fixUse)
			return 2
		}
		pl, err := m.Preview(name)
		if err != nil {
			return fail(name, err, 2)
		}
		shown := fixReply{OK: true, Name: name, Title: fix.Title(name), Before: pl.Before, After: pl.After, NeedsAdmin: m.NeedsAdmin(name)}
		if !asJSON {
			fmt.Fprintf(out, "%s\n    now:   %s\n    after: %s\n", name, pl.Before, pl.After)
		}
		if !yes {
			if asJSON {
				say(shown)
			} else {
				fmt.Fprintf(out, "Nothing was changed. To do it, run: fix apply %s --yes\n", name)
			}
			return 0
		}
		if _, err := m.Apply(name, true); err != nil {
			return fail(name, err, 2)
		}
		if asJSON {
			shown.Applied = true
			say(shown)
			return 0
		}
		if !fix.Undoable(name) {
			fmt.Fprintln(out, "Done. This fix has no undo.")
			return 0
		}
		fmt.Fprintf(out, "Done. To put it back, run: fix undo %s\n", name)
		return 0
	}
	fmt.Fprintln(out, fixUse)
	return 2
}
