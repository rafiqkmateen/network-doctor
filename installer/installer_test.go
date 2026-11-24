package installer

import (
	"os"
	"regexp"
	"strings"
	"testing"
)

func script(t *testing.T) string {
	t.Helper()
	b, err := os.ReadFile("netdoctor.iss")
	if err != nil {
		t.Fatalf("the installer script is missing: %v", err)
	}
	return string(b)
}

// section returns the lines of one [Section] of the script.
func section(t *testing.T, s, name string) []string {
	t.Helper()
	var out []string
	in := false
	for _, l := range strings.Split(s, "\n") {
		l = strings.TrimSpace(l)
		if strings.HasPrefix(l, "[") && strings.HasSuffix(l, "]") {
			in = strings.EqualFold(l, "["+name+"]")
			continue
		}
		if in && l != "" && !strings.HasPrefix(l, ";") {
			out = append(out, l)
		}
	}
	return out
}

// ND-111: the on-demand check is a component the person cannot clear.
func TestCoreComponentIsFixed(t *testing.T) {
	for _, l := range section(t, script(t), "Components") {
		if strings.Contains(l, `Name: "core"`) {
			if !strings.Contains(l, "fixed") {
				t.Fatalf("core must carry the fixed flag: %s", l)
			}
			return
		}
	}
	t.Fatal(`no component named "core"`)
}

// ND-112: each optional part has its own off-by-default component with a plain sentence.
func TestOptionalPartsHaveTheirOwnSentence(t *testing.T) {
	s := script(t)
	for _, name := range []string{"scheduled", "proactive"} {
		found := false
		for _, l := range section(t, s, "Components") {
			if strings.Contains(l, `Name: "`+name+`"`) {
				found = true
				if strings.Contains(l, "fixed") {
					t.Errorf("%s must not be fixed", name)
				}
			}
		}
		if !found {
			t.Errorf("no component named %q", name)
		}
	}
	for _, phrase := range []string{"Once a week", "Your data never leaves your computer", "runs all the time"} {
		if !strings.Contains(s, phrase) {
			t.Errorf("the plain sentences should say %q", phrase)
		}
	}
	if !strings.Contains(s, "Types:") && !strings.Contains(s, "[Types]") {
		t.Error("the type list must let the person pick parts by hand (custom)")
	}
}

// ND-113 and ND-117: tasks exist only with their component.
func TestTasksBelongToTheirComponents(t *testing.T) {
	var weekly, logon bool
	for _, l := range section(t, script(t), "Run") {
		if strings.Contains(l, "schtasks") && strings.Contains(l, "/SC WEEKLY") {
			weekly = true
			if !strings.Contains(l, `Components: scheduled`) {
				t.Errorf("the weekly task must need the scheduled component: %s", l)
			}
		}
		if strings.Contains(l, "schtasks") && strings.Contains(l, "/SC ONLOGON") {
			logon = true
			if !strings.Contains(l, `Components: proactive`) {
				t.Errorf("the log-on task must need the proactive component: %s", l)
			}
		}
	}
	if !weekly || !logon {
		t.Fatalf("both tasks must be created by the script (weekly=%v logon=%v)", weekly, logon)
	}
}

// ND-115: Program Files, administrator rights, an entry in Apps and features.
func TestInstallsInProgramFilesAsAdministrator(t *testing.T) {
	s := script(t)
	if !strings.Contains(s, "{autopf}") && !strings.Contains(s, "{pf}") {
		t.Error("the program folder must be under Program Files")
	}
	if !regexp.MustCompile(`(?m)^PrivilegesRequired\s*=\s*admin`).MatchString(s) {
		t.Error("PrivilegesRequired must be admin")
	}
	if !regexp.MustCompile(`(?m)^AppId\s*=`).MatchString(s) || !strings.Contains(s, "UninstallDisplayName") {
		t.Error("an AppId and an uninstall display name give the Apps and features entry")
	}
}

// ND-116 and ND-118: the marker that tells the program it is installed, and ProgramData handling.
func TestWritesTheInstalledMarkerAndOwnsProgramData(t *testing.T) {
	s := script(t)
	if !strings.Contains(s, "installed.marker") {
		t.Error("the installer must write installed.marker beside the program")
	}
	if !strings.Contains(s, `{commonappdata}\NetworkDoctor`) {
		t.Error("the shared folder must be commonappdata plus NetworkDoctor")
	}
	if !strings.Contains(s, "users-modify") {
		t.Error("ordinary users must be able to write to the shared folder")
	}
}

// codeBody returns the body of the [Code] routine whose header starts with head, or "" if none.
func codeBody(s, head string) string {
	start := strings.Index(s, head)
	if start < 0 {
		return ""
	}
	end := strings.Index(s[start:], "\nend;")
	if end < 0 {
		return s[start:]
	}
	return s[start : start+end]
}

// ND-118: the uninstaller removes both tasks and, with no question, the whole results folder;
// only /KEEPRESULTS=1 keeps it, and the old /DELETERESULTS=1 changes nothing.
func TestUninstallUndoesTheInstallAndDeletesResults(t *testing.T) {
	s := script(t)
	run := strings.Join(section(t, s, "UninstallRun"), "\n")
	if !strings.Contains(run, "/Delete") || strings.Count(run, "schtasks") < 2 {
		t.Error("both tasks must be deleted on uninstall")
	}
	if strings.Contains(strings.ToLower(s), "keep your saved results") {
		t.Error("the uninstaller must no longer ask: Keep your saved results")
	}
	for _, head := range []string{"function InitializeUninstall(", "procedure CurUninstallStepChanged(",
		"procedure InitializeUninstallProgressForm(", "procedure DeinitializeUninstall("} {
		if b := codeBody(s, head); strings.Contains(b, "MsgBox") {
			t.Errorf("%s shows a message box; removal must ask nothing of its own", head)
		}
	}
	// Only the note about a change not put back may show, once, with only OK, never in a silent removal.
	if strings.Contains(s, "MB_YESNO") {
		t.Error("no yes-or-no question may be left in the script")
	}
	step := codeBody(s, "procedure CurUninstallStepChanged(")
	if !strings.Contains(step, "usPostUninstall") ||
		!strings.Contains(step, `DelTree(ExpandConstant('{commonappdata}\NetworkDoctor'), True, True, True)`) {
		t.Fatal("after removal the uninstaller must delete the whole results folder")
	}
	if !regexp.MustCompile(`(?s)\(not KeepResults\).*DelTree`).MatchString(step) {
		t.Error("the results folder must be deleted unless KeepResults is set")
	}
	if !strings.Contains(codeBody(s, "function InitializeUninstall("), "KeepResults := KeepResultsAsked();") {
		t.Error("/KEEPRESULTS must be read to keep the results")
	}
	// The old switch is still accepted (Inno passes on any /NAME=value) and decides nothing: it is
	// named only in comments, so a script that gives it gets the default, which deletes.
	for i, l := range strings.Split(s, "\n") {
		trim := strings.TrimSpace(l)
		if strings.Contains(l, "DELETERESULTS") && !strings.HasPrefix(trim, ";") && !strings.HasPrefix(trim, "//") {
			t.Errorf("line %d: the old /DELETERESULTS switch must decide nothing: %s", i+1, trim)
		}
	}
	if !strings.Contains(s, "/DELETERESULTS=1") {
		t.Error("a comment must say the old /DELETERESULTS=1 is still accepted")
	}
}

// ND-118: /KEEPRESULTS keeps the results given bare or with any value but empty or 0, as /NAME or
// -NAME, in any case; {param:} matches only /NAME=value, so the script reads the switches itself.
func TestKeepResultsSwitchValues(t *testing.T) {
	s := script(t)
	b := codeBody(s, "function KeepResultsAsked(")
	if b == "" {
		t.Fatal("no KeepResultsAsked function")
	}
	for _, want := range []string{
		"for I := 1 to ParamCount do",
		"P := Uppercase(Trim(ParamStr(I)));",
		"if Copy(P, 1, 1) = '-' then P := '/' + Copy(P, 2, Length(P));",
		"if P = '/KEEPRESULTS' then",
		"else if Copy(P, 1, 13) = '/KEEPRESULTS=' then",
		"V := Trim(Copy(P, 14, Length(P)));",
		"Result := (V <> '') and (V <> '0');",
	} {
		if !strings.Contains(b, want) {
			t.Errorf("KeepResultsAsked must contain %q", want)
		}
	}
	if len("/KEEPRESULTS=") != 13 {
		t.Fatal("the value starts after 13 letters")
	}
	if strings.Contains(s, "{param:KEEPRESULTS") {
		t.Error("{param:KEEPRESULTS} misses a bare /KEEPRESULTS, so it must not decide")
	}
}

// ND-316: on Windows the uninstaller runs the installed program's own undo of every fix still
// applied once, after the person confirms and before the tasks, program or results go; on a fault,
// or a program that will not start while notes exist, it keeps only the undo notes and says so,
// once, with only OK, never in a silent removal.
func TestUninstallPutsBackFixesFirstOnWindows(t *testing.T) {
	s := script(t)
	step := codeBody(s, "procedure CurUninstallStepChanged(")
	if !regexp.MustCompile(`(?s)CurUninstallStep = usUninstall\) then\s+PutBackFixes\(\);`).MatchString(step) {
		t.Error("the fixes must be put back at usUninstall, after the confirmation and before [UninstallRun]")
	}
	if strings.Contains(codeBody(s, "function InitializeUninstall("), "PutBackFixes") {
		t.Error("InitializeUninstall runs before the person confirms; the undo must wait")
	}
	put := codeBody(s, "procedure PutBackFixes(")
	for _, want := range []string{
		`if not FileExists(ExpandConstant('{commonappdata}\NetworkDoctor\fix-saved.json')) then`,
		`Dir := ExpandConstant('{commonappdata}\NetworkDoctor');`,
		`NotePath := Dir + '\fix-not-undone.txt';`,
		// The results folder is named, so a NETDOCTOR_DIR set for the whole computer cannot point
		// the undo at other notes while the real ones in ProgramData are deleted.
		`Args := 'fix undo-all --dir "' + Dir + '" --note "' + NotePath + '"';`,
		// With /KEEPRESULTS the note must not say the results are deleted.
		`if KeepResults then Args := Args + ' --kept';`,
		`Exec(ExpandConstant('{app}\netdoctor.exe'), Args, '',`,
		"SW_HIDE, ewWaitUntilTerminated, Code)",
		"FixesLeft := True;",
		"Log(Note);",
		"if not UninstallSilent then",
		"SuppressibleMsgBox(Note, mbInformation, MB_OK, IDOK);",
	} {
		if !strings.Contains(put, want) {
			t.Errorf("PutBackFixes must contain %q", want)
		}
	}
	// Notes with nothing recorded are no notes: even if the program cannot start, nobody is told a
	// change was not put back.
	if !regexp.MustCompile(`(?s)LoadStringFromFile\(Dir \+ '\\fix-saved.json', SavedA\) and \(Trim\(String\(SavedA\)\) = '\{\}'\) then\s+Exit;.*Exec\(`).MatchString(put) {
		t.Error("notes holding only {} must end the step before undo-all runs")
	}
	// A change left as the person set it is told too, while the removal goes on as normal.
	if !regexp.MustCompile(`(?s)end\s+else if LoadStringFromFile\(NotePath, NoteA\) and \(Trim\(String\(NoteA\)\) <> ''\) then\s+begin\s+Note := String\(NoteA\);\s+Log\(Note\);\s+if not UninstallSilent then\s+SuppressibleMsgBox\(Note, mbInformation, MB_OK, IDOK\);`).MatchString(put) {
		t.Error("after a good undo-all its note (a change left as set) must be logged and, unless silent, shown with OK")
	}
	// An old note from an earlier removal is deleted before the undo runs, so it is never shown again.
	if !regexp.MustCompile(`(?s)DeleteFile\(NotePath\);.*Exec\(`).MatchString(put) {
		t.Error("an old fix-not-undone.txt must be deleted before undo-all runs")
	}
	if regexp.MustCompile(`(?s)Exec\(.*\(Code <> 0\)`).FindString(put) == "" {
		t.Error("a program that will not start, or a fault it reports, must both count as not put back")
	}
	if !regexp.MustCompile(`(?s)if FixesLeft then\s+KeepOnlyUndoNotes\(ExpandConstant\('\{commonappdata\}\\NetworkDoctor'\)\)\s+else\s+DelTree`).MatchString(step) {
		t.Error("on a fault only the undo notes may stay; otherwise the whole folder goes")
	}
	keep := codeBody(s, "procedure KeepOnlyUndoNotes(")
	for _, name := range []string{"'fix-saved.json'", "'fix-log.txt'", "'fix-enabled-here.txt'", "'fix-not-undone.txt'"} {
		if !strings.Contains(keep, name) {
			t.Errorf("KeepOnlyUndoNotes must keep %s", name)
		}
	}
}

// ND-119 and ND-210: nothing goes off the computer, except Ookla's own tool when the person asks for it.
func TestNothingLeavesTheComputerExceptOoklaOnRequest(t *testing.T) {
	s := script(t)
	low := strings.ToLower(s)
	for _, bad := range []string{"idp", "smtp", "curl", "invoke-webrequest", "downloadtemporaryfile"} {
		if strings.Contains(low, bad) {
			t.Errorf("the installer script must not contain %q", bad)
		}
	}
	for _, m := range regexp.MustCompile(`(?i)https?://[^/\s"']+`).FindAllString(s, -1) {
		host := strings.ToLower(strings.SplitN(strings.SplitN(m, "://", 2)[1], "/", 2)[0])
		if m[:5] != "https" || (host != "www.speedtest.net" && host != "install.speedtest.net") {
			t.Errorf("the only addresses allowed are Ookla's own, over https: %s", m)
		}
	}
}

// ND-209: the installer writes the choices file from the parts selected, and the uninstaller removes it.
func TestWritesTheChoicesFileAndRemovesItOnUninstall(t *testing.T) {
	s := script(t)
	if !strings.Contains(s, "choices.json") {
		t.Fatal("the installer must write choices.json beside the program")
	}
	for _, key := range []string{`"core"`, `"scheduled"`, `"proactive"`, `"ookla"`} {
		if !strings.Contains(s, key) {
			t.Errorf("choices.json must carry the key %s", key)
		}
	}
	for _, comp := range []string{"scheduled", "proactive"} {
		if !strings.Contains(s, "WizardIsComponentSelected('"+comp+"')") {
			t.Errorf("the file must follow what the person selected for %s", comp)
		}
	}
	del := strings.Join(section(t, s, "UninstallDelete"), " ")
	if !strings.Contains(del, "choices.json") || !strings.Contains(del, `{app}\ookla`) {
		t.Errorf("the uninstaller must remove choices.json and the ookla folder: %s", del)
	}
}

// ND-209: a part that was not chosen has its task removed, so running the installer again repairs the machine to match the choices.
func TestUnchosenPartsHaveTheirTasksRemoved(t *testing.T) {
	var weekly, logon bool
	for _, l := range section(t, script(t), "Run") {
		if strings.Contains(l, "/Delete") && strings.Contains(l, "Network Doctor weekly check") {
			weekly = strings.Contains(l, "Components: not scheduled")
		}
		if strings.Contains(l, "/Delete") && strings.Contains(l, "Network Doctor alerts") {
			logon = strings.Contains(l, "Components: not proactive")
		}
	}
	if !weekly || !logon {
		t.Errorf("each task must be deleted when its part is not chosen (weekly=%v alerts=%v)", weekly, logon)
	}
}

// ND-210: Ookla's tool is offered, never forced: unchecked by default, never in a silent install, the person's own agreement is a
// separate tick, and the file comes only from Ookla, checked against a fixed hash.
func TestOoklaOfferNeedsTheirOwnYesAndNeverRunsSilently(t *testing.T) {
	s := script(t)
	for _, want := range []string{
		"WizardSilent",
		"I have read the Ookla license",
		"https://www.speedtest.net/about/eula",
		"https://install.speedtest.net/app/cli/ookla-speedtest-1.2.0-win64.zip",
		"Checked := False",
	} {
		if !strings.Contains(s, want) {
			t.Errorf("the Ookla offer must contain %q", want)
		}
	}
	if !regexp.MustCompile(`(?i)[0-9a-f]{64}`).MatchString(s) {
		t.Error("the download must be checked against a fixed SHA-256")
	}
	// The license switches may appear only in the step that runs after the person's tick.
	if strings.Count(s, "--accept-license") != 1 {
		t.Errorf("--accept-license must appear exactly once, in the step after the person's tick, found %d", strings.Count(s, "--accept-license"))
	}
	if !strings.Contains(s, "OoklaAgreed") {
		t.Error("the step that accepts the license must depend on the person's tick (OoklaAgreed)")
	}
}

// ND-218: the alerts line says alerts are kept in a file and shown on the screen, and that setup can turn them off.
func TestAlertsLineSaysWhatTheyDo(t *testing.T) {
	for _, l := range section(t, script(t), "Components") {
		if strings.Contains(l, `Name: "proactive"`) {
			for _, phrase := range []string{"kept in a file", "on the screen", "setup can turn"} {
				if !strings.Contains(l, phrase) {
					t.Errorf("the alerts line should say %q:\n%s", phrase, l)
				}
			}
			return
		}
	}
	t.Error("no alerts component")
}

// ND-256: the setup lines must not claim nothing leaves the computer, since the checks run the speed
// test and a few small requests. Each part's line says the person's data stays, says requests go out,
// and points to the guide's list.
func TestSetupLinesAreHonest(t *testing.T) {
	s := script(t)
	if strings.Contains(strings.ToLower(s), "nothing leaves your computer") {
		t.Error(`the script says "Nothing leaves your computer"`)
	}
	seen := 0
	for _, l := range section(t, s, "Components") {
		if !strings.Contains(l, `Name: "`) {
			continue
		}
		seen++
		for _, phrase := range []string{"Your data never leaves your computer", "do go out", "What goes over the internet"} {
			if !strings.Contains(l, phrase) {
				t.Errorf("setup line must say %q: %s", phrase, l)
			}
		}
	}
	if seen != 3 {
		t.Errorf("found %d of the 3 setup lines", seen)
	}
}

// ND-257: the Ready page must not print the long part descriptions (they run past the page), and
// the Ookla tick boxes must be tall enough for their text at any screen scaling.
func TestReadyPageAndTickBoxesFit(t *testing.T) {
	s := script(t)
	if !strings.Contains(s, "function UpdateReadyMemo(") {
		t.Fatal("the script has no UpdateReadyMemo, so the Ready page prints the long part descriptions")
	}
	if !strings.Contains(s, "AlwaysShowDirOnReadyPage=yes") {
		t.Error("the folder page is off, so the Ready page shows the folder only with AlwaysShowDirOnReadyPage=yes")
	}
	start := strings.Index(s, "function UpdateReadyMemo(")
	end := strings.Index(s[start:], "\nend;")
	body := s[start : start+end]
	if strings.Contains(body, "MemoComponentsInfo +") || strings.Contains(body, "+ MemoComponentsInfo") {
		t.Error("UpdateReadyMemo must not reuse MemoComponentsInfo")
	}
	for _, name := range []string{"OoklaOffer", "OoklaAgree"} {
		if !strings.Contains(s, name+".Height := ScaleY(") {
			t.Errorf("%s needs a scaled height", name)
		}
	}
}

// finishBody returns the body of the procedure that writes the finish page text, or "" if none.
func finishBody(s string) string {
	start := strings.Index(s, "procedure CurPageChanged(")
	if start < 0 {
		return ""
	}
	end := strings.Index(s[start:], "\nend;")
	if end < 0 {
		return s[start:]
	}
	return s[start : start+end]
}

// ND-260: the last screen tells the person what to do next: that it is installed, how to run a check,
// where the program and the results live, how to remove it and where the guide is. It is built from
// the parts chosen, so it never claims the weekly check or alerts when they were not picked.
func TestFinishPageSaysWhatToDoNext(t *testing.T) {
	s := script(t)
	body := finishBody(s)
	if body == "" {
		t.Fatal("the script has no CurPageChanged, so the finish page keeps Inno Setup's own words")
	}
	for _, want := range []string{
		"wpFinished",
		"WizardForm.FinishedLabel.Caption",
		"WizardForm.AdjustLabelHeight(WizardForm.FinishedLabel)",
		"is installed",
		"Network Doctor icon on your desktop or in the Start menu",
		"A page opens in your web browser, runs the check and shows the answer.",
		"Close the browser tab when you are done.",
		"ExpandConstant('{app}')",
		`ExpandConstant('{commonappdata}') + '\NetworkDoctor'`,
		"Windows Settings, Apps, Installed apps, Network Doctor, Uninstall",
		"It takes away everything, your saved results and settings too, so a new install starts fresh. It first tries to put back any setting a fix changed.",
		"GUIDE.md",
	} {
		if !strings.Contains(body, want) {
			t.Errorf("the finish text must contain %q", want)
		}
	}
	// The guide shipped beside the program is the one the finish text points to.
	if !strings.Contains(strings.Join(section(t, s, "Files"), "\n"), `GUIDE.md"; DestDir: "{app}"`) {
		t.Error("the finish text points to GUIDE.md in the program folder, so setup must put it there")
	}
	// The text points to the icon, never to a terminal or to typing, and no longer to a console window.
	low := strings.ToLower(body)
	if strings.Contains(low, "keep your saved results") {
		t.Error("the finish text still says removal asks whether to keep the results (ND-260, amended)")
	}
	if strings.Contains(low, "terminal") || strings.Contains(low, "typ") || strings.Contains(body, `.\netdoctor`) {
		t.Error("the finish text sends the person to a terminal or to typing; it must point to the icon (ND-261)")
	}
	if strings.Contains(low, "a window opens") || strings.Contains(low, "close the window") {
		t.Error("the finish text still describes the old console window; the icon now opens a page (ND-261)")
	}
	// ND-261: each icon runs the no-console build with the open command, never a console window.
	iconLines := section(t, s, "Icons")
	n := 0
	for _, l := range iconLines {
		if !strings.HasPrefix(l, "Name:") {
			continue
		}
		n++
		for _, want := range []string{`Filename: "{app}\netdoctorw.exe"`, `Parameters: "open"`, "Components: core"} {
			if !strings.Contains(l, want) {
				t.Errorf("icon line must contain %q: %s", want, l)
			}
		}
		for _, bad := range []string{"{cmd}", "/k ", "cmd.exe", " check"} {
			if strings.Contains(l, bad) {
				t.Errorf("icon line must not open a console (%q): %s", bad, l)
			}
		}
	}
	if n != 2 {
		t.Errorf("want exactly two icons (Start menu and desktop), found %d", n)
	}
	icons := strings.Join(iconLines, "\n")
	for _, want := range []string{`{autoprograms}\Network Doctor`, `{autodesktop}\Network Doctor`} {
		if !strings.Contains(icons, want) {
			t.Errorf("the [Icons] section must contain %q", want)
		}
	}
	// Each add-on claim sits on a line that only runs when that part was chosen.
	claims := map[string]string{"Monday at 0900": "scheduled", "log on": "proactive"}
	for phrase, comp := range claims {
		found := false
		for _, l := range strings.Split(body, "\n") {
			if !strings.Contains(l, phrase) {
				continue
			}
			found = true
			if !strings.Contains(l, "if WizardIsComponentSelected('"+comp+"') then") {
				t.Errorf("%q is claimed without checking that %s was chosen: %s", phrase, comp, strings.TrimSpace(l))
			}
		}
		if !found {
			t.Errorf("the finish text must say %q when %s was chosen", phrase, comp)
		}
	}
	if strings.Count(body, "else") < 2 {
		t.Error("the finish text must say something true for each add-on that was not chosen")
	}
	// Setting a caption is harmless in a silent install; a message box or a program run is not.
	for _, bad := range []string{"MsgBox", "Exec(", "ShellExec"} {
		if strings.Contains(body, bad) {
			t.Errorf("the finish text step must not use %s (a silent install must show nothing)", bad)
		}
	}
}

// ND-260: the finish screen leaves silent installs and the repair run as they were, and any text
// file the setup shows is in the project, so the build can pack it.
func TestFinishPageKeepsSilentInstallsAndShipsItsText(t *testing.T) {
	s := script(t)
	setup := section(t, s, "Setup")
	for _, l := range setup {
		low := strings.ToLower(l)
		if strings.HasPrefix(low, "disablefinishedpage") && strings.Contains(low, "yes") {
			t.Errorf("the finish page is turned off: %s", l)
		}
		for _, key := range []string{"infoafterfile", "infobeforefile", "licensefile"} {
			if strings.HasPrefix(low, key) {
				name := strings.TrimSpace(l[strings.Index(l, "=")+1:])
				if _, err := os.Stat(name); err != nil {
					t.Errorf("%s names a file that is not in the installer folder: %v", l, err)
				}
			}
		}
	}
	for _, guard := range []string{
		"if PageID = OoklaPage.ID then Result := WizardSilent;",
		"OoklaAgreed and (not WizardSilent)",
		"Components: not scheduled",
		"Components: not proactive",
	} {
		if !strings.Contains(s, guard) {
			t.Errorf("the silent-install or repair line %q is gone", guard)
		}
	}
}
