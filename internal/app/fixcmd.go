package app

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/rafiqkmateen/network-doctor/internal/fix"
)

// newFixSystem makes the real computer for fixes. Tests swap it for a pretend one.
var newFixSystem = func() fix.System { return fix.Real() }

const fixUse = `Use: fix list | fix apply NAME [--yes] | fix undo NAME | fix enable-here
Without --yes, apply only shows the setting now and what it would become.`

func fixCmd(args []string, dir string, out io.Writer) int {
	if len(args) == 0 {
		fmt.Fprintln(out, fixUse)
		return 2
	}
	m := fix.NewManager(newFixSystem(), dir, fix.Options{Unattended: os.Getenv("NETDOCTOR_UNATTENDED") != ""})
	switch {
	case args[0] == "list":
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
	case args[0] == "undo" && len(args) == 2:
		if err := m.Undo(args[1]); err != nil {
			fmt.Fprintln(out, err)
			if errors.Is(err, fix.ErrNothingToUndo) {
				return 0
			}
			return 2
		}
		fmt.Fprintf(out, "Put back: %s. Logged in %s.\n", args[1], m.Dir())
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
			fmt.Fprintln(out, err)
			return 2
		}
		fmt.Fprintf(out, "%s\n    now:   %s\n    after: %s\n", name, pl.Before, pl.After)
		if !yes {
			fmt.Fprintf(out, "Nothing was changed. To do it, run: fix apply %s --yes\n", name)
			return 0
		}
		if _, err := m.Apply(name, true); err != nil {
			fmt.Fprintln(out, err)
			return 2
		}
		fmt.Fprintf(out, "Done. To put it back, run: fix undo %s\n", name)
		return 0
	}
	fmt.Fprintln(out, fixUse)
	return 2
}
