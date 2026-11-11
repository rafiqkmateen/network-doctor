//go:build windows

package collect

import (
	"os/exec"
	"syscall"
)

func hide(c *exec.Cmd) { c.SysProcAttr = &syscall.SysProcAttr{HideWindow: true} }
