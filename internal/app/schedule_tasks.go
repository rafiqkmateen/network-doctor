package app

// The Windows task names, shared by the installer and `schedule`.
const (
	windowsWeeklyTask = "Network Doctor weekly check"
	windowsAlertsTask = "Network Doctor alerts"
)

// windowsTaskSettingsScript is the PowerShell that gives a Windows task the settings schtasks has
// no switch for (ND-289). Both tasks get no run-time limit and run on battery, keeping on when the
// power cord comes out. The weekly task runs as soon as it can after a missed start. The alerts
// task is started again a minute after it fails (up to 3 times), and its log-on start repeats every
// 5 minutes with no end, so a helper that stopped for any reason is started again within 5 minutes;
// a repeat while it runs starts no second copy. It changes the task's own settings and saves them,
// so every other setting stays as it was. The installer runs the same text
// (installer/netdoctor.iss); it holds no double quote or brace, so it goes into the installer
// unchanged. It has no build tag so the test can read it on any computer.
func windowsTaskSettingsScript(task string, alerts bool) string {
	s := "$t = Get-ScheduledTask -TaskName '" + task + "'; " +
		"$t.Settings.ExecutionTimeLimit = 'PT0S'; $t.Settings.DisallowStartIfOnBatteries = $false; " +
		"$t.Settings.StopIfGoingOnBatteries = $false; "
	if alerts {
		s += "$t.Settings.RestartCount = 3; $t.Settings.RestartInterval = 'PT1M'; " +
			"$t.Settings.MultipleInstances = 'IgnoreNew'; $t.Triggers[0].Repetition.Interval = 'PT5M'; "
	} else {
		s += "$t.Settings.StartWhenAvailable = $true; "
	}
	return s + "Set-ScheduledTask -InputObject $t | Out-Null"
}
