package notify

import (
	"errors"
	"strings"
	"testing"
)

// ND-287: a screen alert on a pretend Mac and a pretend Linux. The title and the message are each
// their own argument and never part of any script text, so a message cannot run as a command.
func TestScreenAlertMacAndLinux(t *testing.T) {
	msg := `A device "x" is on: do shell script "rm -rf ~" \ end run' $(reboot) ` + "`id`"
	for _, c := range []struct{ goos, prog string }{{"darwin", "osascript"}, {"linux", "notify-send"}, {"freebsd", "notify-send"}} {
		var got []string
		look := func(name string) (string, error) { return "/usr/bin/" + name, nil }
		run := func(name string, args ...string) (string, error) {
			got = append([]string{name}, args...)
			return "", nil
		}
		if err := ScreenWith(c.goos, look, run, "Network Doctor", msg); err != nil {
			t.Fatalf("%s: %v", c.goos, err)
		}
		if got[0] != c.prog {
			t.Errorf("%s: ran %q, want %q", c.goos, got[0], c.prog)
		}
		n := len(got)
		if got[n-2] != "Network Doctor" || got[n-1] != msg {
			t.Errorf("%s: title and message must be the last two arguments, whole: %q", c.goos, got)
		}
		for _, a := range got[:n-2] {
			if strings.Contains(a, "rm -rf") || strings.Contains(a, "Network Doctor") {
				t.Errorf("%s: the message or title leaked into the script text: %q", c.goos, a)
			}
		}
		if c.goos == "darwin" && !strings.Contains(strings.Join(got, " "), "item 2 of argv") {
			t.Errorf("the Mac script must read the message from its arguments: %q", got)
		}
		if c.prog == "notify-send" && got[1] != "--" {
			t.Errorf("notify-send must end its options before the title, so a message starting with - is text: %q", got)
		}
	}
	// A message that starts with a dash is still only text.
	var got []string
	run := func(name string, args ...string) (string, error) {
		got = append([]string{name}, args...)
		return "", nil
	}
	look := func(name string) (string, error) { return name, nil }
	_ = ScreenWith("linux", look, run, "Network Doctor", "--help")
	if got[len(got)-1] != "--help" || got[len(got)-3] != "--" {
		t.Errorf("dash message: %q", got)
	}
}

// ND-288: a missing helper and a failing one each give an error and never panic.
func TestScreenAlertWithoutHelper(t *testing.T) {
	missing := func(string) (string, error) { return "", errors.New("not found") }
	ran := false
	run := func(string, ...string) (string, error) { ran = true; return "", nil }
	if err := ScreenWith("linux", missing, run, "Network Doctor", "hi"); !errors.Is(err, ErrNoHelper) || ran {
		t.Errorf("missing helper: err=%v ran=%v", err, ran)
	}
	found := func(n string) (string, error) { return n, nil }
	failing := func(string, ...string) (string, error) { return "no display", errors.New("exit status 1") }
	if err := ScreenWith("darwin", found, failing, "Network Doctor", "hi"); err == nil {
		t.Error("a failing helper must give an error")
	}
}
