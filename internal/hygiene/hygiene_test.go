// Package hygiene holds the test that keeps private words, local user paths, personal email
// addresses, working notes and backup files out of the public tree. It reads every file Git
// tracks, so a note kept on the computer and ignored by Git never counts.
//
// The checks that need no secret run everywhere. The author's private word list is not in the
// tree: it sits in conf/guard_words.txt, which is not tracked (see package guardwords), and a
// checkout without that file runs only the generic checks.
package hygiene

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/guardwords"
)

// phrasing bans: working-note wording that does not belong in the program or its tests.
var phrasing = []*regexp.Regexp{
	regexp.MustCompile(`(?i)this laptop`),
	regexp.MustCompile(`(?i)\bhim\b`),
	regexp.MustCompile(`(?i)change list [0-9]`),
	regexp.MustCompile(`(?i)builders? (do not|[0-9]|own)`),
	regexp.MustCompile(`(?i)independent auditor`),
	regexp.MustCompile(`(?i)found on the live`),
}

var codeRoots = []string{"cmd/", "internal/"}

// userPath finds a path into somebody's home folder on Windows or Mac; the name after it must
// be one of the made-up names below.
var userPath = regexp.MustCompile("(?i)(?:\\b[a-z]:[\\\\/]+|[\\s\"'`(=:]/)users[\\\\/]+([^\\\\/\\s\"'`<>)]+)")

var madeUpUser = regexp.MustCompile(`(?i)^(pat|tester|someone|alice|sam|all|public|default|%username%|\$env:username|\{.*\}|\*)$`)

// anyEmail finds an email address; only the listed ones may appear.
var anyEmail = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9-]+(?:\.[A-Za-z0-9-]+)+`)

var allowedEmail = map[string]bool{
	"rkmaerial@gmail.com": true, // the contact address in README.md
	"user@uid.service":    true, // the shape of a systemd user unit name, in a Linux script comment
}

// oddFile matches a tracked path that is a backup or a working note.
var oddFile = regexp.MustCompile(`(?i)(\.bak-|(^|/)handover\.md$|(^|/)claude\.md$)`)

func projectRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("go.mod not found")
		}
		dir = parent
	}
}

func tracked(t *testing.T) (string, []string) {
	t.Helper()
	dir := projectRoot(t)
	cmd := exec.Command("git", "ls-files", "-z")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		t.Skip("git is not available here, so the tracked files cannot be listed")
	}
	var files []string
	for _, f := range strings.Split(string(out), "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return dir, files
}

func isCode(rel string) bool {
	for _, r := range codeRoots {
		if strings.HasPrefix(rel, r) {
			return true
		}
	}
	return false
}

// selfFile holds the patterns and the sample lines the checks are tried on, so it is not checked.
const selfFile = "internal/hygiene/hygiene_test.go"

// checkText returns what is wrong with one tracked text file, one message per finding.
// private may be empty; then only the generic checks run. A path the private list skips, such
// as a file that is not part of the public copy, gets no check at all.
func checkText(rel string, text []byte, private guardwords.List) []string {
	var bad []string
	if rel == selfFile || private.Skipped(rel) {
		return bad
	}
	for i, line := range strings.Split(string(text), "\n") {
		trim := strings.TrimSpace(line)
		for _, m := range userPath.FindAllStringSubmatch(line, -1) {
			if !madeUpUser.MatchString(m[1]) {
				bad = append(bad, rel+":"+strconv.Itoa(i+1)+" holds a local user path: "+trim)
			}
		}
		for _, e := range anyEmail.FindAllString(line, -1) {
			if !allowedEmail[strings.ToLower(e)] {
				bad = append(bad, rel+":"+strconv.Itoa(i+1)+" holds an email address that is not on the list: "+e)
			}
		}
		for _, re := range private.Hard {
			if re.MatchString(line) {
				bad = append(bad, rel+":"+strconv.Itoa(i+1)+" holds a private word or path: "+trim)
			}
		}
		if isCode(rel) {
			for _, re := range phrasing {
				if re.MatchString(line) {
					bad = append(bad, rel+":"+strconv.Itoa(i+1)+" holds working-note wording: "+trim)
				}
			}
			for _, re := range private.Phrase {
				if re.MatchString(line) {
					bad = append(bad, rel+":"+strconv.Itoa(i+1)+" holds working-note wording: "+trim)
				}
			}
		}
	}
	return bad
}

// TestNoPrivateWordsInTrackedFiles: ND-249.
func TestNoPrivateWordsInTrackedFiles(t *testing.T) {
	dir, files := tracked(t)
	private, err := guardwords.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	if !private.Found {
		t.Log("conf/guard_words.txt is not here, so only the generic checks ran")
	}
	for _, rel := range files {
		if oddFile.MatchString(rel) {
			t.Errorf("%s is a backup or a working note and must not be tracked", rel)
			continue
		}
		for _, re := range private.Hard {
			if re.MatchString(rel) && !private.Skipped(rel) {
				t.Errorf("%s: the path holds a private word", rel)
			}
		}
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil || bytes.IndexByte(b, 0) >= 0 {
			continue // missing or a binary file
		}
		for _, msg := range checkText(rel, b, private) {
			t.Error(msg)
		}
	}
}

// The generic checks must catch what they are for, and let the made-up names through.
func TestGenericChecksCatchWhatTheyAreFor(t *testing.T) {
	none := guardwords.List{}
	// Written in two halves so the copy tool's own path scan does not read these samples as real paths.
	win := "C:" + `\Users\`
	mac := "/Use" + "rs/"
	cases := []struct {
		name, text string
		want       int
	}{
		{"real-looking Windows user path", `p := "` + win + `jsmith\x"`, 1},
		{"real-looking Mac user path", `const p = "` + mac + `jsmith/x"`, 1},
		{"made-up Windows user", `p := "` + win + `pat\AppData"`, 0},
		{"made-up Mac user", `const exe = "` + mac + `tester/Applications/x"`, 0},
		{"unlisted email", `write to jsmith@example.org`, 1},
		{"listed email", `write to rkmaerial@gmail.com`, 0},
		{"plain text", `nothing to see here`, 0},
	}
	for _, c := range cases {
		if got := len(checkText("docs/x.md", []byte(c.text), none)); got != c.want {
			t.Errorf("%s: %d findings, want %d", c.name, got, c.want)
		}
	}
	if got := len(checkText("internal/a/a.go", []byte("// ask him later"), none)); got != 1 {
		t.Errorf("working-note wording in code: %d findings, want 1", got)
	}
	if got := len(checkText("docs/x.md", []byte("// ask him later"), none)); got != 0 {
		t.Errorf("working-note wording outside code: %d findings, want 0", got)
	}
}

// A private list, when present, is applied to text and to paths, and its skip entries are honoured.
func TestPrivateListIsApplied(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "conf"), 0o755); err != nil {
		t.Fatal(err)
	}
	body := "hard (?i)zebra\nphrase (?i)giraffe\nskip docs/archive/\n"
	if err := os.WriteFile(filepath.Join(root, "conf", guardwords.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	private, err := guardwords.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if got := len(checkText("docs/x.md", []byte("a Zebra"), private)); got != 1 {
		t.Errorf("hard word: %d findings, want 1", got)
	}
	if got := len(checkText("docs/archive/x.md", []byte("a Zebra"), private)); got != 0 {
		t.Errorf("skipped path: %d findings, want 0", got)
	}
	if got := len(checkText("internal/a/a.go", []byte("a giraffe"), private)); got != 1 {
		t.Errorf("phrase in code: %d findings, want 1", got)
	}
	if got := len(checkText("docs/x.md", []byte("a giraffe"), private)); got != 0 {
		t.Errorf("phrase outside code: %d findings, want 0", got)
	}
}
