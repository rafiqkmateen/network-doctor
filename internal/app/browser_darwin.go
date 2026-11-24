//go:build darwin

package app

import (
	"io"
	"os/exec"
)

// showNoBrowser prints the address and shows it in a Mac dialog made with osascript. Pressing OK
// ends the program, as the Windows message box does (ND-300).
func showNoBrowser(url string, out io.Writer) bool {
	return dialogNoBrowser("darwin", exec.LookPath, runDialog, url, out)
}
