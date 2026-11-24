package app

import (
	"errors"
	"io"
	"os/exec"
)

// The words of the no-browser box on Mac and Linux, the same as the Windows message box (ND-300).
const (
	noBrowserLead = "Network Doctor could not open your web browser."
	noBrowserOpen = "Open this address in your web browser:"
	noBrowserDone = "Press OK when you are done. Network Doctor then stops."
)

// noBrowserText is the whole box text with the address in it, for zenity and kdialog, which take
// the text as one argument and run no script.
func noBrowserText(url string) string {
	return noBrowserLead + "\n\n" + noBrowserOpen + "\n" + url + "\n\n" + noBrowserDone
}

// macDialogScript shows the box on a Mac. The address is read from the script's own argument, so
// it is never part of the script text.
var macDialogScript = []string{
	"-e", "on run argv",
	"-e", `display dialog ("` + noBrowserLead + `" & return & return & "` + noBrowserOpen +
		`" & return & (item 1 of argv) & return & return & "` + noBrowserDone +
		`") buttons {"OK"} default button "OK" with title "Network Doctor"`,
	"-e", "end run",
}

// noBrowserDialogs lists the ways to show the box on goos, in the order they are tried: osascript
// on a Mac; zenity, then kdialog, elsewhere.
func noBrowserDialogs(goos, url string) [][]string {
	if goos == "darwin" {
		return [][]string{append(append([]string{"osascript"}, macDialogScript...), url)}
	}
	text := noBrowserText(url)
	return [][]string{
		{"zenity", "--info", "--no-markup", "--title=Network Doctor", "--text=" + text},
		{"kdialog", "--title", "Network Doctor", "--msgbox", text},
	}
}

// dialogNoBrowser prints the address, then shows it in the first box program that is there and
// works. It returns true once the person has pressed OK, so the program ends (as on Windows), and
// false when the address was only printed. It has no build tag, so the tests run it as a simulated
// Mac and a simulated Linux on any computer.
func dialogNoBrowser(goos string, look func(string) (string, error), run func(string, ...string) error, url string, out io.Writer) bool {
	printNoBrowser(url, out)
	for _, c := range noBrowserDialogs(goos, url) {
		if _, err := look(c[0]); err != nil {
			continue
		}
		err := run(c[0], c[1:]...)
		if err == nil || (c[0] == "zenity" && closedByPerson(err)) {
			return true // OK pressed, or the zenity box closed with its X: no second box
		}
	}
	return false
}

// closedByPerson is true when a box program ended with exit code 1, which zenity gives when the
// person closes the box; a box that could not start at all has no exit code.
func closedByPerson(err error) bool {
	var ec interface{ ExitCode() int }
	return errors.As(err, &ec) && ec.ExitCode() == 1
}

// runDialog runs a box program and waits for the person to press OK.
func runDialog(name string, args ...string) error { return exec.Command(name, args...).Run() }
