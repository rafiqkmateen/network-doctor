//go:build !windows && !darwin && !linux

package app

import (
	"fmt"
	"io"

	"github.com/rafiqkmateen/network-doctor/internal/store"
)

// scheduleCmd on a system with no weekly job built (not Windows, Mac or Linux).
func scheduleCmd(args []string, cfg store.Config, out io.Writer) int {
	fmt.Fprintln(out, "The weekly check is not built for this system.")
	return 2
}
