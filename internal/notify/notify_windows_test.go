//go:build windows

package notify

import (
	"strings"
	"testing"
)

// A device name can hold any quote sign. None may end the script's single-quoted string, which in
// PowerShell the straight quote and the four single curly quotes all do.
func TestWindowsAlertTextCannotLeaveItsQuotes(t *testing.T) {
	got := esc("a'b‘c’d‚e‛f\"g<h>&")
	for _, q := range []string{"‘", "’", "‚", "‛", "\"", "<", ">"} {
		if strings.Contains(got, q) {
			t.Errorf("%q is still in %q", q, got)
		}
	}
	if strings.Count(got, "'") != 2 || !strings.Contains(got, "a''b") {
		t.Errorf("a straight quote must be doubled: %q", got)
	}
}
