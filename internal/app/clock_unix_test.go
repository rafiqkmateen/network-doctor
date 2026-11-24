package app

import (
	"errors"
	"strings"
	"testing"
)

// ND-298: the Mac reader's answers 1, 0, an error, an empty answer and junk.
func TestParseMacClock(t *testing.T) {
	for _, c := range []struct {
		out  string
		err  error
		want string
	}{
		{"1\n", nil, "24-hour"},
		{"0", nil, "12-hour"},
		{"The domain/default pair of (kCFPreferencesAnyApplication, AppleICUForce24HourTime) does not exist", errors.New("exit status 1"), "unknown"},
		{"", nil, "unknown"},
		{"yes please", nil, "unknown"},
		{"1", errors.New("exit status 1"), "unknown"},
	} {
		if got := show(parseMacClock(c.out, c.err)); got != c.want {
			t.Errorf("%q %v: got %s want %s", c.out, c.err, got, c.want)
		}
	}
}

// ND-299: the Linux reader's answers '24h', '12h', a missing tool, an error and junk.
func TestParseLinuxClock(t *testing.T) {
	for _, c := range []struct {
		out  string
		err  error
		want string
	}{
		{"'24h'\n", nil, "24-hour"},
		{"'12h'", nil, "12-hour"},
		{"", errors.New(`exec: "gsettings": executable file not found in $PATH`), "unknown"},
		{"No such schema “org.gnome.desktop.interface”", errors.New("exit status 1"), "unknown"},
		{"'auto'", nil, "unknown"},
		{"'24h'", errors.New("exit status 1"), "unknown"},
	} {
		if got := show(parseLinuxClock(c.out, c.err)); got != c.want {
			t.Errorf("%q %v: got %s want %s", c.out, c.err, got, c.want)
		}
	}
}

// ND-163, ND-298, ND-299: the two clock reads only read.
func TestClockCommandsOnlyRead(t *testing.T) {
	for _, c := range [][]string{macClockCommand, linuxClockCommand} {
		switch c[0] {
		case "defaults", "gsettings":
		default:
			t.Errorf("%q is not an approved reader", c[0])
		}
		if c[1] != "read" && c[1] != "get" {
			t.Errorf("%v must only read or get", c)
		}
		for _, a := range c[1:] {
			low := strings.ToLower(a)
			for _, bad := range []string{"write", "set", "delete", "reset", "sudo"} {
				if strings.HasPrefix(low, bad) {
					t.Errorf("%v uses change word %q", c, a)
				}
			}
		}
	}
}
