package app

import (
	"context"
	"os/exec"
	"strings"
	"time"
)

// The clock reads on Mac and Linux. Each only reads a setting (ND-163).
var (
	macClockCommand   = []string{"defaults", "read", "-g", "AppleICUForce24HourTime"}
	linuxClockCommand = []string{"gsettings", "get", "org.gnome.desktop.interface", "clock-format"}
)

// parseMacClock reads the Mac's global AppleICUForce24HourTime: 1 is a 24-hour clock, 0 a
// 12-hour one; an error, nothing, or anything else is unknown (ND-298).
func parseMacClock(out string, err error) *bool {
	if err != nil {
		return nil
	}
	yes, no := true, false
	switch strings.TrimSpace(out) {
	case "1":
		return &yes
	case "0":
		return &no
	}
	return nil
}

// parseLinuxClock reads the desktop's clock-format: '24h' or '12h'; an error or anything else is
// unknown (ND-299).
func parseLinuxClock(out string, err error) *bool {
	if err != nil {
		return nil
	}
	yes, no := true, false
	switch strings.Trim(strings.TrimSpace(out), `'"`) {
	case "24h":
		return &yes
	case "12h":
		return &no
	}
	return nil
}

// readSetting runs one read command for at most a few seconds.
func readSetting(c []string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	b, err := exec.CommandContext(ctx, c[0], c[1:]...).Output()
	return string(b), err
}
