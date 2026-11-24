package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ND-289: the weekly task runs as soon as it can after a missed start, and the alerts task is
// started again if it stops with an error, every minute up to 3 times; both from the installer
// and (weekly) from schedule enable. The settings are read, never applied, here.
func TestWindowsTaskSettings(t *testing.T) {
	weekly := windowsTaskSettingsScript(windowsWeeklyTask, false)
	alerts := windowsTaskSettingsScript(windowsAlertsTask, true)
	// Both: no run-time limit, and they run on battery and keep running when the power cord comes out.
	both := []string{"Set-ScheduledTask -InputObject $t", "$t.Settings.ExecutionTimeLimit = 'PT0S'",
		"$t.Settings.DisallowStartIfOnBatteries = $false", "$t.Settings.StopIfGoingOnBatteries = $false"}
	for _, want := range append([]string{"Get-ScheduledTask -TaskName 'Network Doctor weekly check'", "$t.Settings.StartWhenAvailable = $true"}, both...) {
		if !strings.Contains(weekly, want) {
			t.Errorf("weekly settings lack %q: %s", want, weekly)
		}
	}
	if strings.Contains(weekly, "Restart") || strings.Contains(weekly, "Repetition") {
		t.Errorf("the weekly task does not restart: %s", weekly)
	}
	// Alerts: started again a minute after a failure (3 times), and, whatever stopped it, started again
	// within 5 minutes by a repeat of the log-on start; never a second copy at once.
	for _, want := range append([]string{"Get-ScheduledTask -TaskName 'Network Doctor alerts'", "$t.Settings.RestartCount = 3",
		"$t.Settings.RestartInterval = 'PT1M'", "$t.Settings.MultipleInstances = 'IgnoreNew'",
		"$t.Triggers[0].Repetition.Interval = 'PT5M'"}, both...) {
		if !strings.Contains(alerts, want) {
			t.Errorf("alerts settings lack %q: %s", want, alerts)
		}
	}
	// Changing the task's own settings keeps every other setting; a fresh settings set would reset them.
	if strings.Contains(weekly+alerts, "New-ScheduledTaskSettingsSet") || strings.ContainsAny(weekly+alerts, `"{}`) {
		t.Errorf("the scripts must change the task's own settings and hold no double quote or brace (Inno): %s %s", weekly, alerts)
	}

	b, err := os.ReadFile(filepath.Join("..", "..", "installer", "netdoctor.iss"))
	if err != nil {
		t.Fatal(err)
	}
	var run []string
	in := false
	for _, l := range strings.Split(string(b), "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") {
			in = l == "[Run]"
			continue
		}
		if in && l != "" && !strings.HasPrefix(l, ";") {
			run = append(run, l)
		}
	}
	find := func(sub string) int {
		for i, l := range run {
			if strings.Contains(l, sub) {
				return i
			}
		}
		return -1
	}
	for _, c := range []struct{ create, script, comp string }{
		{"/SC WEEKLY", weekly, "Components: scheduled;"},
		{"/SC ONLOGON", alerts, "Components: proactive;"},
	} {
		ci, si := find(c.create), find(c.script)
		if ci < 0 || si < 0 {
			t.Errorf("the installer must create the task and then set it: create=%d settings=%d (%s)", ci, si, c.script)
			continue
		}
		if si <= ci {
			t.Errorf("the settings line must come after the task is made")
		}
		if !strings.Contains(run[si], c.comp) || !strings.Contains(run[si], "Flags: runhidden") || !strings.Contains(run[si], `Filename: "powershell.exe"`) {
			t.Errorf("settings line: %s", run[si])
		}
	}

	src, err := os.ReadFile("schedule_windows.go")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(src), "windowsTaskSettingsScript(windowsWeeklyTask, false)") {
		t.Error("schedule enable must give the weekly task the same settings")
	}
}
