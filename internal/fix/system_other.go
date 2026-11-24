//go:build !windows

package fix

import (
	"context"
	"net"
	"os/exec"
	"runtime"
	"strings"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
)

// Real is the computer this program runs on: a Mac or Linux computer gets the fixes through its
// own tools (ND-303 to ND-307); any other system refuses every fix.
func Real() System {
	return UnixSystem{GOOS: runtime.GOOS, Run: execRun, Look: exec.LookPath, Index: ifIndex, Conds: conditionsNow}
}

// execRun runs one program, giving up after two minutes.
func execRun(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	b, err := exec.CommandContext(ctx, name, args...).CombinedOutput()
	return strings.TrimSpace(string(b)), err
}

// ifIndex is the system's own number for an adapter, read without running anything.
func ifIndex(dev string) int {
	if i, err := net.InterfaceByName(dev); err == nil {
		return i.Index
	}
	return 0
}

// conditionsNow reads the situation at the press (ND-244) with the check's own read code.
func conditionsNow() (Conditions, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	return readConditions(ctx, collect.NewSource())
}
