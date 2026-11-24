package fix

import (
	"strings"
	"testing"
)

// ND-245: the reset-adapter preview must not say the line is already down while the line works. It
// says instead that the fix only runs when the line is down, which is true whatever the line does now.
func TestND245ResetPreviewIsTrueOnAWorkingLine(t *testing.T) {
	for name, c := range map[string]Conditions{
		"working": {AdapterUp: true, HasAddress: true, RouterAnswers: true, InternetAnswers: true},
		"dead":    lineDead,
	} {
		m := NewManager(newNDCond(c), t.TempDir(), Options{})
		pl, err := m.Preview(ndResetAdapter)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		after := strings.ToLower(pl.After)
		if strings.Contains(after, "already down") {
			t.Errorf("%s: the preview claims the line is already down: %q", name, pl.After)
		}
		if !strings.Contains(after, "only runs when the line is down") {
			t.Errorf("%s: the preview must say the fix only runs when the line is down: %q", name, pl.After)
		}
		if !strings.Contains(after, "no undo") {
			t.Errorf("%s: the preview must still say there is no undo: %q", name, pl.After)
		}
	}
}
