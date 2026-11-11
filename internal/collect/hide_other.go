//go:build !windows

package collect

import "os/exec"

func hide(c *exec.Cmd) {}
