package notify

import (
	"errors"
	"fmt"
	"strings"
)

// Runner runs one program with its arguments. Tests pass a pretend one.
type Runner func(name string, args ...string) (string, error)

// ErrNoHelper means the system has no program to show a screen alert with.
var ErrNoHelper = errors.New("no program to show alerts on the screen was found")

// macScript reads the title and the message from its own arguments, so neither is ever part of
// the script text (ND-287).
var macScript = []string{
	"-e", "on run argv",
	"-e", "display notification (item 2 of argv) with title (item 1 of argv)",
	"-e", "end run",
}

// Command is the program and arguments that show one alert on a Mac (osascript) or Linux and the
// other Unix systems (notify-send). The title and the message are the last two arguments, whole.
func Command(goos, title, msg string) (string, []string) {
	if goos == "darwin" {
		return "osascript", append(append([]string{}, macScript...), title, msg)
	}
	// "--" ends the options, so a message that starts with a dash is still only text.
	return "notify-send", []string{"--", title, msg}
}

// ScreenWith shows one alert on goos, finding the program with look and running it with run. A
// missing program gives ErrNoHelper; a program that fails gives its error. It has no build tag,
// so the tests run it as a pretend Mac and a pretend Linux on any computer.
func ScreenWith(goos string, look func(string) (string, error), run Runner, title, msg string) error {
	name, args := Command(goos, title, msg)
	if _, err := look(name); err != nil {
		return fmt.Errorf("%w (%s)", ErrNoHelper, name)
	}
	if out, err := run(name, args...); err != nil {
		return fmt.Errorf("%s did not show the alert: %v %s", name, err, strings.TrimSpace(out))
	}
	return nil
}
