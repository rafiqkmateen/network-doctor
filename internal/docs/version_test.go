package docs

import (
	"regexp"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/app"
)

// lastRelease is the newest version that was ever published as a release. No download is offered any
// more, so it only keeps the history in CHANGELOG.md whole (ND-211).
const lastRelease = "0.3.0"

// ND-211: the program's version (app.Version) is X.Y.Z, the guide and the front page say that a build
// from source is the only way to get it (no download is offered), CHANGELOG.md has a section for the
// program's version and for the last release, and the installer builds its file name from AppVersion.
// The installer's version itself is passed in when it is built, so this test shows the file name
// follows AppVersion, not which version a given build was handed.
func TestVersionMatchesDocsChangelogAndInstaller(t *testing.T) {
	v := app.Version
	if !regexp.MustCompile(`^\d+\.\d+\.\d+$`).MatchString(v) {
		t.Fatalf("app.Version %q is not X.Y.Z", v)
	}
	for _, d := range []doc{guide(t), readme(t)} {
		if !strings.Contains(d.text, "A build from source says "+v) {
			t.Errorf("%s: does not say that a build from source says %s", d.rel, v)
		}
	}
	changelog := load(t, "CHANGELOG.md")
	for _, ver := range []string{v, lastRelease} {
		if !strings.Contains(changelog.text, "\n## ["+ver+"]") {
			t.Errorf("CHANGELOG.md has no \"## [%s]\" section", ver)
		}
	}
	iss := load(t, "installer/netdoctor.iss")
	for _, line := range []string{"AppVersion={#AppVersion}", "OutputBaseFilename=NetworkDoctor-Setup-{#AppVersion}"} {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(line) + `\s*$`).MatchString(iss.text) {
			t.Errorf("installer/netdoctor.iss: missing the line %q", line)
		}
	}
	// The requirement itself names the version, so a version that is consistent everywhere but wrong
	// (not the one ND-211 asks for) still fails.
	req := load(t, "docs/requirements.md")
	m := regexp.MustCompile(`- ND-211 The program version shall be (\d+\.\d+\.\d+)`).FindStringSubmatch(req.text)
	if m == nil {
		t.Fatal("docs/requirements.md: no ND-211 line naming the version")
	}
	if m[1] != v {
		t.Errorf("ND-211 asks for version %s, the program says %s", m[1], v)
	}
	// The built file name follows the program: the build script reads the version from app.go, and does
	// not default to a number of its own.
	build := load(t, "scripts/build-installer.ps1")
	if regexp.MustCompile(`\$Version\s*=\s*'\d+\.\d+\.\d+'`).MatchString(build.text) {
		t.Error("scripts/build-installer.ps1 defaults to a fixed version instead of the program's")
	}
	if !strings.Contains(build.text, "internal/app/app.go") && !strings.Contains(build.text, `internal\app\app.go`) {
		t.Error("scripts/build-installer.ps1 does not read the version from internal/app/app.go")
	}
}
