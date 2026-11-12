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
	for _, phrase := range []string{"Once a week", "Nothing leaves your computer", "runs all the time"} {
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

// ND-118: the uninstaller removes both tasks and asks about saved results, keeping them by default.
func TestUninstallUndoesTheInstallAndKeepsResultsByDefault(t *testing.T) {
	s := script(t)
	run := strings.Join(section(t, s, "UninstallRun"), "\n")
	if !strings.Contains(run, "/Delete") || strings.Count(run, "schtasks") < 2 {
		t.Error("both tasks must be deleted on uninstall")
	}
	if !strings.Contains(s, "Keep your saved results") {
		t.Error("the uninstaller must ask: Keep your saved results")
	}
	if !regexp.MustCompile(`(?s)UNINSTALL_DELETE_RESULTS|DeleteResults`).MatchString(s) {
		t.Error("deleting saved results must be a named, opt-in step")
	}
}

// ND-119: nothing goes off the computer.
func TestNothingLeavesTheComputer(t *testing.T) {
	low := strings.ToLower(script(t))
	for _, bad := range []string{"http://", "https://", "downloadtemporaryfile", "idp", "smtp", "curl", "invoke-webrequest"} {
		if strings.Contains(low, bad) {
			t.Errorf("the installer script must not contain %q", bad)
		}
	}
}
