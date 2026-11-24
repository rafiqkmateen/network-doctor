//go:build linux

package app

import (
	"io"
	"os/exec"
)

// showNoBrowser prints the address and shows it in a zenity box, else a kdialog box. Pressing OK
// ends the program, as the Windows message box does; with neither, the address is only printed
// (ND-300).
func showNoBrowser(url string, out io.Writer) bool {
	return dialogNoBrowser("linux", exec.LookPath, runDialog, url, out)
}
