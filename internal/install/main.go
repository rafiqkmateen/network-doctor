package install

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"strings"
)

// Main runs the install or uninstall command with its words (ND-271, ND-291, ND-293):
//
//	install                                   the setup page in the browser
//	install --silent [--parts=scheduled,proactive]   no page; never Ookla's tool
//	install --write-setup-files DIR --system mac|linux   the setup download's text files
//	uninstall                                 the removal page in the browser; removes everything
//	uninstall --silent [--keep-results]       no page; results deleted unless kept
//
// --delete-results is still accepted for older scripts and only repeats the default; with
// --keep-results too, the results are kept. --keep-results without --silent is refused.
//
// It returns the exit code. A real install or removal is refused on Windows, which has its own setup file.
func Main(ctx context.Context, cmd string, args []string, e Env, out io.Writer) int {
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(out)
	silent := fs.Bool("silent", false, "no page and no browser")
	var parts, writeDir, system *string
	var keepResults *bool
	if cmd == "install" {
		parts = fs.String("parts", "", "add-ons for a silent install: scheduled, proactive or both")
		writeDir = fs.String("write-setup-files", "", "write the setup download's files into this folder")
		system = fs.String("system", "", "mac or linux, with --write-setup-files")
	} else {
		keepResults = fs.Bool("keep-results", false, "with --silent: keep the saved results and settings")
		fs.Bool("delete-results", false, "accepted for older scripts: the saved results are deleted anyway")
	}
	if fs.Parse(args) != nil || fs.NArg() > 0 {
		return 2
	}
	if cmd == "uninstall" && *keepResults && !*silent {
		fmt.Fprintln(out, outKeepSilent) // never ignored: the page always deletes
		return 2
	}
	if cmd == "install" && *writeDir != "" {
		if err := WriteSetupFiles(*writeDir, *system); err != nil {
			fmt.Fprintln(out, "Could not write the setup files:", err)
			return 2
		}
		fmt.Fprintf(out, "Setup files written. Put the program at %s and the guide at %s.\n",
			SetupProgramPath(*system), SetupGuidePath(*system))
		return 0
	}
	if e.GOOS == "windows" {
		if cmd == "install" {
			fmt.Fprintln(out, outWindowsSetup)
		} else {
			fmt.Fprintln(out, outWindowsGone)
		}
		return 2
	}
	if !*silent {
		mode := ModeInstall
		if cmd == "uninstall" {
			mode = ModeUninstall
		}
		if err := Serve(ctx, e, mode, out); err != nil {
			fmt.Fprintln(out, "Could not open the page:", err)
			return 2
		}
		return 0
	}
	e.Silent = true
	if cmd == "uninstall" {
		rep, err := UninstallReport(e, *keepResults, io.Discard)
		// A setting changed by hand since its fix was left as set: said, never a fault (ND-316).
		if lines := SettledLines(rep.Settled); len(lines) > 0 {
			fmt.Fprintln(out, strings.Join(lines, "\n"))
		}
		if err != nil {
			var nu *NotUndoneError
			if errors.As(err, &nu) {
				// A fix not put back: the lines say which, and that its notes are kept (ND-316).
				fmt.Fprintln(out, strings.Join(NotUndoneLines(nu.Failures, nu.Notes, *keepResults), "\n"))
				if strings.Count(err.Error(), "\n") == 0 {
					return 1
				}
			}
			fmt.Fprintln(out, probRemove)
			return 1
		}
		if *keepResults {
			fmt.Fprintln(out, sprintf(outRemovedKept, e.ResultsDir))
		} else {
			fmt.Fprintln(out, outRemoved)
		}
		return 0
	}
	var c Choice
	for _, p := range strings.Split(*parts, ",") {
		switch p = strings.TrimSpace(p); p {
		case "":
		case "scheduled":
			c.Weekly = true
		case "proactive":
			c.Alerts = true
		default:
			fmt.Fprintf(out, outBadPart+"\n", p)
			return 2
		}
	}
	r := Install(e, c, io.Discard)
	if len(r.Problems) > 0 {
		fmt.Fprintln(out, strings.Join(r.Problems, " "))
		return 1
	}
	fmt.Fprintln(out, outInstalled)
	return 0
}
