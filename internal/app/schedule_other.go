//go:build !windows

package app

import (
	"fmt"
	"io"

	"github.com/rafiqkmateen/network-doctor/internal/store"
)

func scheduleCmd(args []string, cfg store.Config, out io.Writer) int {
	fmt.Fprintln(out, "Scheduling is not built for this system yet. Use cron or a systemd timer to run: netdoctor weekly")
	return 2
}
