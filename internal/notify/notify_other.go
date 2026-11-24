//go:build !windows

// Package notify shows a message on the screen. It only displays text.
package notify

import (
	"context"
	"os/exec"
	"runtime"
	"strings"
	"time"
)

// Screen shows a notification: on a Mac through osascript, on Linux through notify-send (ND-287).
// A missing or failing helper gives an error; the caller keeps going (ND-288).
func Screen(title, msg string) error {
	return ScreenWith(runtime.GOOS, exec.LookPath, execRun, title, msg)
}

// execRun runs the helper, giving up after half a minute so a desktop with no notification
// service cannot hold the watcher.
func execRun(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(b)), err
}
