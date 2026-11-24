// Package docs holds the tests that read the guide (docs/GUIDE.md) and the front page
// (README.md) as plain text. They check that each topic is named by its key words, that both
// example pictures are linked, captioned and exist, and that no banned word is used (ND-250 to
// ND-255). The private words are kept out of this file.
package docs

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/guardwords"
)

// root finds the folder that holds go.mod, walking up from where the test runs.
func root(t *testing.T) string {
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

// doc is one document: its path from the root and its text with line endings made plain.
type doc struct {
	rel  string
	text string
}

func load(t *testing.T, rel string) doc {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(root(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return doc{rel: rel, text: strings.ReplaceAll(string(b), "\r\n", "\n")}
}

func guide(t *testing.T) doc  { return load(t, "docs/GUIDE.md") }
func readme(t *testing.T) doc { return load(t, "README.md") }

// mustHave reports each key word that the document does not hold, ignoring case.
func mustHave(t *testing.T, d doc, topic string, words ...string) {
	t.Helper()
	low := strings.ToLower(d.text)
	for _, w := range words {
		if !strings.Contains(low, strings.ToLower(w)) {
			t.Errorf("%s: %s: missing %q", d.rel, topic, w)
		}
	}
}

// mustNotHave reports each retired sentence that the document still holds, ignoring case.
func mustNotHave(t *testing.T, d doc, topic string, words ...string) {
	t.Helper()
	low := strings.ToLower(d.text)
	for _, w := range words {
		if strings.Contains(low, strings.ToLower(w)) {
			t.Errorf("%s: %s: still says %q", d.rel, topic, w)
		}
	}
}

// mustMatch reports each pattern that the document does not match, with case kept.
func mustMatch(t *testing.T, d doc, topic string, patterns ...string) {
	t.Helper()
	for _, p := range patterns {
		if !regexp.MustCompile(p).MatchString(d.text) {
			t.Errorf("%s: %s: nothing matches %s", d.rel, topic, p)
		}
	}
}

// TestPlatformsAndAddOns: ND-250.
func TestPlatformsAndAddOns(t *testing.T) {
	for _, d := range []doc{guide(t), readme(t)} {
		mustMatch(t, d, "systems", `\bWindows\b`, `\bMac\b`, `\bLinux\b`)
		mustHave(t, d, "default and add-ons", "check on demand", "weekly check", "alerts", "add-on")
		mustHave(t, d, "page only looks", "The web page only looks", "fixes are run by command")
		mustHave(t, d, "power saving refused", "could drop the connection you are using")
		mustNotHave(t, d, "retired sentences", "typed in the terminal", "work only on Windows", "only on Windows")
	}
	// The guide holds the whole list. The front page holds the short set and points to the guide.
	g := guide(t)
	mustHave(t, g, "what changes settings", "apart from its own settings and the parts setup installs and you ask for")
	mustHave(t, g, "setup files", "Windows, Mac and Linux each have a setup", "## Install it (Windows)",
		"## Install it (Mac)", "## Install it (Linux)")
	mustNotHave(t, g, "retired setup sentence", "There is no setup file for Mac or Linux",
		"apart from its own settings and the weekly task you ask for")
	mustHave(t, g, "fixes everywhere", "`proxy`, `public-dns`, `renew-address` and `reset-adapter` work on Windows, Mac and Linux",
		"a fix whose system tool is missing is refused")
	mustHave(t, g, "no fix buttons", "no fix buttons on any system")
	mustHave(t, g, "no download offered", "No ready-made download is offered", "build it yourself")
	mustNotHave(t, g, "retired release sentence", "carries everything")
	r := readme(t)
	mustHave(t, r, "what changes settings", "apart from its own settings and the weekly task you ask for")
	mustHave(t, r, "fixes", "six fixes", "five of them", "work on Windows, Mac and Linux", "no fix buttons")
	mustHave(t, r, "no download offered", "No ready-made download is offered", "build it yourself")
	mustHave(t, r, "link to the guide", "](docs/GUIDE.md)")
}

// TestCarriedFeatures: ND-251.
func TestCarriedFeatures(t *testing.T) {
	d := guide(t)
	mustHave(t, d, "sign-in page", "sign-in page")
	mustHave(t, d, "clock", "clock")
	mustHave(t, d, "country", "leaves from")
	mustHave(t, d, "second test", "second speed test", "upload is at least 20 megabits")
	mustHave(t, d, "weak signal", "-75 dBm", "20 megabits")
	mustHave(t, d, "hidden Wi-Fi", "Wi-Fi details")
	mustHave(t, d, "fix offers and undo", "fix_offers", "undos", "it refuses and changes nothing")
	mustHave(t, d, "new fixes", "public-dns", "renew-address", "reset-adapter")
	mustHave(t, d, "name-cache tools", "resolvectl flush-caches", "systemd-resolved", "dscacheutil")
	mustHave(t, d, "Mac and Linux fix tools", "networksetup -getwebproxy", "-setwebproxystate", "gsettings",
		"org.gnome.system.proxy", "networksetup -setdnsservers", "\"Empty\" means automatic", "nmcli",
		"ipconfig set", "nmcli device reapply", "networksetup -setnetworkserviceenabled", "nmcli device disconnect")
	mustHave(t, d, "unattended", "NETDOCTOR_UNATTENDED", "fix enable-here")
	mustHave(t, d, "alerts by default", "If you picked the Alerts add-on", "to the screen")
	mustHave(t, d, "trusted devices", "config known-from")
	mustHave(t, d, "other languages", "set to another language")
	mustHave(t, d, "no address read", "joined but got no address from the router", "on Windows, on a Mac and on Linux")
	mustHave(t, d, "where results show", "print below the numbers in the terminal", "show only in `netdoctor check --json`")
	mustHave(t, d, "data folders", "Library/Application Support/NetworkDoctor", ".config/NetworkDoctor")
	mustHave(t, d, "build version", "A build from source says 0.4.0")
	mustHave(t, d, "speed waits", "`watch` ignores both waits")
	mustHave(t, d, "slow alert", "three light tests in a row", "about an hour after the first slow test")
	mustHave(t, d, "environment variables", "environment variable", "setx", "export")
	mustHave(t, d, "line use", "Line use right now")
	mustHave(t, d, "exit codes", "for `check`", "settings file could not be read")
	mustHave(t, d, "trusted file", "a JSON object whose keys are")
}

// TestOutsideRequestsNamed: ND-252. Each request is named with what it sends or keeps.
func TestOutsideRequestsNamed(t *testing.T) {
	for _, d := range []doc{guide(t), readme(t)} {
		mustHave(t, d, "what servers see", "your public internet address", "your name, your devices")
	}
	// The guide names every request in full. The front page names the main ones and points to the guide.
	g := guide(t)
	inOneBullet(t, g, "speed test", "speed.cloudflare.com", "test data")
	inOneBullet(t, g, "country lookup", "cdn-cgi/trace", "keeps only the two-letter country code")
	inOneBullet(t, g, "sign-in test", "www.msftconnecttest.com", "captive.apple.com", "connectivity-check.ubuntu.com",
		"keeps no cookies", "follows no redirects")
	inOneBullet(t, g, "device names", "name lookup for each device", "own name server")
	inOneBullet(t, g, "alerts helper", "about every 30 minutes", "every 30th round", "first round")
	inOneBullet(t, g, "Ookla", "Ookla's servers", "Ookla's own privacy terms", "reports its test to Ookla",
		"install.speedtest.net", "NETDOCTOR_OOKLA", "PATH", "Windows, Mac or Linux")
	inOneBullet(t, g, "reachability", "1.1.1.1", "example.com")
	r := readme(t)
	inOneBullet(t, r, "speed test", "speed.cloudflare.com", "test data")
	inOneBullet(t, r, "country lookup", "cdn-cgi/trace", "two-letter country code")
	inOneBullet(t, r, "sign-in test", "sign-in test", "no cookies")
	inOneBullet(t, r, "device names", "name lookup for each device", "own name server")
	inOneBullet(t, r, "Ookla", "Ookla", "only if you add it yourself")
	inOneBullet(t, r, "reachability", "1.1.1.1", "example.com")
	mustHave(t, r, "link to the guide", "](docs/GUIDE.md)")
}

// inOneBullet reports a topic when no single bullet line ("- ...") holds all its words, so a
// host and the words for what it sends or keeps must sit together.
func inOneBullet(t *testing.T, d doc, topic string, words ...string) {
	t.Helper()
	for _, line := range strings.Split(d.text, "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "- ") {
			continue
		}
		low, all := strings.ToLower(line), true
		for _, w := range words {
			if !strings.Contains(low, strings.ToLower(w)) {
				all = false
				break
			}
		}
		if all {
			return
		}
	}
	t.Errorf("%s: %s: no one bullet holds all of %q", d.rel, topic, words)
}

// TestInstallLinesHonest: ND-252. The setup lines for the two add-ons must not say nothing
// leaves the computer, since both run the speed test; they point to the list of requests.
func TestInstallLinesHonest(t *testing.T) {
	for _, d := range []doc{guide(t), readme(t)} {
		if strings.Contains(strings.ToLower(d.text), "nothing leaves your computer") {
			t.Errorf("%s: says \"Nothing leaves your computer\"", d.rel)
		}
	}
	g := guide(t)
	seen := 0
	for _, line := range strings.Split(g.text, "\n") {
		if strings.HasPrefix(line, "- **Weekly check.** Once a week") || strings.HasPrefix(line, "- **Weekly check.** Saves your speed") ||
			strings.HasPrefix(line, "- **Alerts.** A small helper") {
			seen++
			if !strings.Contains(line, "Your data never leaves your computer") || !strings.Contains(line, "\"What goes over the internet\"") {
				t.Errorf("%s: setup line must say your data never leaves and point to \"What goes over the internet\": %q", g.rel, line)
			}
		}
	}
	if seen != 6 { // Windows, Mac and Linux
		t.Errorf("%s: found %d of the 6 setup lines for the weekly check and alerts", g.rel, seen)
	}
}

// TestExactFacts: ND-253.
func TestExactFacts(t *testing.T) {
	d := guide(t)
	mustHave(t, d, "clock source", "only from the answer to the sign-in test")
	mustHave(t, d, "Linux address", "Linux always uses Ubuntu's sign-in test address")
	mustHave(t, d, "second test timing", "after 2 minutes", "10-minute")
}

var (
	picture = regexp.MustCompile(`!\[([^\]]*)\]\(([^)\s]+)\)`)
	italic  = regexp.MustCompile(`^(\*[^*].*[^*]\*|_[^_].*[^_]_)$`)
)

// TestPictures: ND-254.
func TestPictures(t *testing.T) {
	// Both documents show the answer page; the guide also shows the devices list and the
	// notice an older page shows when a newer one retires it.
	both := []string{"docs/images/web-check-good.png", "docs/images/web-check-problem.png"}
	wants := map[string][]string{
		"docs/GUIDE.md": append(append([]string{}, both...),
			"docs/images/web-devices.png", "docs/images/web-newer-page.png"),
	}
	top := root(t)
	for _, d := range []doc{guide(t), readme(t)} {
		want, ok := wants[d.rel]
		if !ok {
			want = both
		}
		mustHave(t, d, "Mac and Linux look", "looks the same on Mac and Linux")
		lines := strings.Split(d.text, "\n")
		found := map[string]bool{}
		for i, line := range lines {
			for _, m := range picture.FindAllStringSubmatch(line, -1) {
				alt, link := strings.TrimSpace(m[1]), m[2]
				rel := filepath.ToSlash(filepath.Join(filepath.Dir(filepath.FromSlash(d.rel)), filepath.FromSlash(link)))
				found[rel] = true
				if !strings.HasPrefix(alt, "Example data") || len(alt) < len("Example data")+10 {
					t.Errorf("%s:%d: alt text %q must start with \"Example data\" and say what it shows", d.rel, i+1, alt)
				}
				if _, err := os.Stat(filepath.Join(top, filepath.FromSlash(rel))); err != nil {
					t.Errorf("%s:%d: picture %s is not in the repository", d.rel, i+1, rel)
				}
				// The caption is the next line that is not blank: an italic line, not another picture.
				next := ""
				for j := i + 1; j < len(lines); j++ {
					if s := strings.TrimSpace(lines[j]); s != "" {
						next = s
						break
					}
				}
				if picture.MatchString(next) || !italic.MatchString(next) || !strings.Contains(strings.ToLower(next), "example") {
					t.Errorf("%s:%d: the line after %s must be an italic caption saying the numbers are an example, got %q", d.rel, i+1, link, next)
				}
			}
		}
		for _, w := range want {
			if !found[w] {
				t.Errorf("%s: no picture link to %s", d.rel, w)
			}
		}
	}
}

// TestDemoVideo: ND-317. The front page links the short demo video by a relative text link (not a
// picture, so the example-picture rule of ND-254 does not apply), says it is silent and a real
// check, and the file is in the repository, is an MP4 and stays small.
func TestDemoVideo(t *testing.T) {
	const rel = "docs/images/network-doctor-demo.mp4"
	mustHave(t, readme(t), "demo video", "]("+rel+")", "no sound", "real check", "windows")
	b, err := os.ReadFile(filepath.Join(root(t), filepath.FromSlash(rel)))
	if err != nil {
		t.Fatalf("the demo video is not in the repository: %v", err)
	}
	if len(b) < 12 || string(b[4:8]) != "ftyp" {
		t.Errorf("%s is not an MP4 file", rel)
	}
	if len(b) > 5<<20 {
		t.Errorf("%s is %d bytes; the limit is 5 MB", rel, len(b))
	}
}

// banned are style words the two documents must not use: a particular computer and the word
// for a tool that stands alone. The author's private words are not here; they are read from
// conf/guard_words.txt when that file exists (see package guardwords).
var banned = []string{
	"this " + "laptop", // joined so the code check for that phrase does not flag this list
	"my laptop",
	"the laptop",
	"standalone",
	"stand-alone",
}

// TestNoBannedWords: ND-255.
func TestNoBannedWords(t *testing.T) {
	private, err := guardwords.Load(root(t))
	if err != nil {
		t.Fatal(err)
	}
	if !private.Found {
		t.Log("conf/guard_words.txt is not here, so only the generic words were checked")
	}
	for _, d := range []doc{guide(t), readme(t)} {
		low := strings.ToLower(d.text)
		for _, w := range banned {
			if regexp.MustCompile(`` + regexp.QuoteMeta(w) + ``).MatchString(low) {
				t.Errorf("%s: holds a banned word (%d letters, starting %q)", d.rel, len(w), w[:3])
			}
		}
		for _, re := range private.Doc {
			if re.MatchString(d.text) {
				t.Errorf("%s: holds a word from the private list", d.rel)
			}
		}
	}
}
