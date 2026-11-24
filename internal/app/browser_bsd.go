//go:build !windows && !darwin && !linux

package app

import (
	"io"
	"os/exec"
	"runtime"
)

// showNoBrowser on the other Unix systems tries the same boxes as Linux, else prints.
func showNoBrowser(url string, out io.Writer) bool {
	return dialogNoBrowser(runtime.GOOS, exec.LookPath, runDialog, url, out)
}
