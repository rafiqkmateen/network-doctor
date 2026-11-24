package fix

import (
	"os/user"
	"strings"
)

// accountSuffix marks the note beside a fix's own note that names the account that applied it
// (ND-316): on Windows the user name and its SID, on a Mac and Linux the user name and its number.
const accountSuffix = "@account"

// CurrentAccount names the account this program runs as, the same way a note records it, or ""
// when it cannot be read.
func CurrentAccount() string {
	u, err := user.Current()
	if err != nil || u.Uid == "" {
		return ""
	}
	return u.Username + " (" + u.Uid + ")"
}

// isAccountNote says whether a key of the undo notes is an account note, not a fix.
func isAccountNote(key string) bool { return strings.HasSuffix(key, accountSuffix) }

// parseRegValue reads one value from the answer of `reg query KEY /v NAME`. A value that is not
// set is no error (found is false); any other failure, a refused read or a reg that did not run,
// is an error, so a read that failed is never taken for "not set" (ND-316).
func parseRegValue(out string, runErr error, name string) (value string, found bool, err error) {
	if runErr != nil {
		if strings.Contains(out, "unable to find the specified registry key or value") {
			return "", false, nil
		}
		msg := strings.TrimSpace(out)
		if msg == "" {
			msg = runErr.Error()
		}
		return "", false, &readError{what: name, msg: msg}
	}
	for _, l := range strings.Split(out, "\n") {
		f := strings.Fields(l)
		if len(f) >= 3 && f[0] == name {
			return strings.Join(f[2:], " "), true, nil
		}
	}
	return "", false, nil
}

// readError is a setting that could not be read.
type readError struct{ what, msg string }

func (e *readError) Error() string { return "could not read " + e.what + ": " + e.msg }
