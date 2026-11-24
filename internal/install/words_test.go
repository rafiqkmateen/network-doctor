package install

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

var terminalWords = []string{"type", "typing", "terminal", "command", "cron", "sudo", "systemd"}

// banned returns the first terminal word found in text as a whole word, any case; with
// allowType false the word "type" is let through (the icon's page says "Connection type").
func banned(text string, withType bool) string {
	for _, w := range terminalWords {
		if w == "type" && !withType {
			continue
		}
		if regexp.MustCompile(`(?i)\b` + w + `\b`).MatchString(text) {
			return w
		}
	}
	return ""
}

func repoFile(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("..", "..", filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(b), "\r\n", "\n")
}

// everyShownText gathers every text the setup route can show on Mac and Linux.
func everyShownText() []string {
	var all []string
	for _, goos := range []string{"darwin", "linux"} {
		for _, c := range []Choice{{}, {Weekly: true, Alerts: true}, {Weekly: true}, {Alerts: true}} {
			for _, r := range []Result{{}, {OoklaTried: true}, {OoklaTried: true, OoklaAdded: true}} {
				all = append(all, FinishLines(goos, "/p", "/r", c, r)...)
			}
		}
		all = append(all, installTexts(goos)...)
	}
	all = append(all, visibleText(MacOpenPage()), visibleText(string(pageHTML)))
	return all
}

// ND-280: no terminal words anywhere on the setup route, and none but "type" on the icon's page.
func TestNoTerminalWordsOnTheIconRoute(t *testing.T) {
	for _, s := range everyShownText() {
		if w := banned(s, true); w != "" {
			t.Errorf("%q in %q", w, s)
		}
	}
	page := repoFile(t, "internal/web/index.html")
	page = regexp.MustCompile(`(?s)<!--.*?-->`).ReplaceAllString(page, "")
	page = regexp.MustCompile(`(?s)/\*.*?\*/`).ReplaceAllString(page, "")
	page = regexp.MustCompile(`(?m)(^|[\s;{}])//.*$`).ReplaceAllString(page, "$1")
	if w := banned(page, false); w != "" {
		t.Errorf("the icon's page holds %q outside its comments", w)
	}
}

// ND-280: the guide's icon route holds no command and no terminal words, and the Mac and Linux
// install sections exist. A paragraph that starts "For IT" (bold or not) is left out.
func TestGuideIconRouteHasNoTerminalTalk(t *testing.T) {
	guide := repoFile(t, "docs/GUIDE.md")
	sections := map[string]string{}
	var name string
	for _, l := range strings.Split(guide, "\n") {
		if strings.HasPrefix(l, "## ") {
			name = strings.TrimPrefix(l, "## ")
			continue
		}
		sections[name] += l + "\n"
	}
	var missing []string
	for _, s := range []string{"Install it (Mac)", "Install it (Linux)"} {
		if _, ok := sections[s]; !ok {
			missing = append(missing, s)
		}
	}
	start := sections["Start it"]
	icon := ""
	for _, p := range strings.Split(start, "\n\n") {
		if strings.Contains(strings.ToLower(p), "icon") {
			icon += p + "\n\n"
		}
	}
	check := icon + sections["Install it (Mac)"] + "\n\n" + sections["Install it (Linux)"]
	var faults []string
	for _, p := range strings.Split(check, "\n\n") {
		if strings.HasPrefix(strings.TrimLeft(strings.TrimSpace(p), "*"), "For IT") {
			continue
		}
		if regexp.MustCompile("`\\.?/?netdoctor\\b").MatchString(p) {
			faults = append(faults, "a command in the icon route: "+p)
		}
		if w := banned(p, false); w != "" {
			faults = append(faults, w+" in the icon route: "+p)
		}
	}
	if len(missing) > 0 {
		t.Errorf("the guide has no section %q", missing)
	}
	for _, f := range faults {
		t.Error(f)
	}
}

// screens joins the words of each screen a person reads, so each is graded as one passage, the way
// a reading grade is meant to be used; a two-word button alone has no fair grade.
func screens() map[string]string {
	all := map[string]string{}
	for _, goos := range []string{"darwin", "linux"} {
		t := removalPageTexts(goos, Layout{ProgramDir: "/p"})
		var parts []string
		for _, p := range t.Parts {
			parts = append(parts, p.Sentence)
		}
		o := t.Ookla
		all[goos+" welcome"] = strings.Join(append([]string{t.Welcome}, parts...), " ")
		all[goos+" ookla"] = strings.Join([]string{o.Lead, o.Note, o.Offer + ".", o.Agree + ".", o.Self, o.NeedTick}, " ")
		all[goos+" ready"] = strings.Join([]string{t.ReadyDir, t.ReadyParts, t.ReadyNone + ".", t.Working}, " ")
		all[goos+" problems"] = strings.Join([]string{t.Failed, probFolder, probProgram, probIcons, probChoices, probJobs,
			probRemove, probResults, t.Gone, t.ClosePage}, " ")
		u := t.Uninstall
		all[goos+" removal"] = strings.Join([]string{u.Ask, u.Note, u.Working}, " ")
		all[goos+" removed"] = strings.Join(removedLines(nil), " ") + " " + unFailed + " " + unNotThere
		for i, c := range []Choice{{}, {Weekly: true, Alerts: true}} {
			for j, r := range []Result{{}, {OoklaTried: true}, {OoklaTried: true, OoklaAdded: true}} {
				all[goos+" finish "+string(rune('a'+i))+string(rune('a'+j))] = strings.Join(FinishLines(goos, "/p", "/r", c, r), " ")
			}
		}
	}
	all["mac open page"] = strings.Join(paragraphs(visibleText(MacOpenPage())), " ")
	return all
}

// ND-05, ND-314: every screen reads at grade 8 or lower.
func TestInstallTextsReadable(t *testing.T) {
	for name, s := range screens() {
		if g := core.GradeLevel(s); g > 8 {
			t.Errorf("%s: grade %.1f: %q", name, g, s)
		}
	}
}

// ND-314: Mac and Linux show the same words except folder names, menu names and the removal route.
func TestMacAndLinuxWordsMatch(t *testing.T) {
	mac, lin := installTexts("darwin"), installTexts("linux")
	if len(mac) != len(lin) {
		t.Fatalf("%d Mac texts, %d Linux texts", len(mac), len(lin))
	}
	for i := range mac {
		m := strings.ReplaceAll(mac[i], "Applications folder", "applications menu")
		if m != lin[i] {
			t.Errorf("Mac %q and Linux %q differ", mac[i], lin[i])
		}
	}
}

// issStrings joins the quoted pieces of one Pascal string expression from the Windows installer.
func issStrings(expr string) string {
	var b strings.Builder
	for _, m := range regexp.MustCompile(`'((?:[^']|'')*)'`).FindAllStringSubmatch(expr, -1) {
		b.WriteString(strings.ReplaceAll(m[1], "''", "'"))
	}
	return b.String()
}

// ND-273: the parts carry the Windows installer's names and sentences, in its order.
func TestInstallPageHasTheWindowsPartWords(t *testing.T) {
	iss := repoFile(t, "installer/netdoctor.iss")
	var win []string
	for _, m := range regexp.MustCompile(`(?m)^Name: "(\w+)"; Description: "([^"]*)"`).FindAllStringSubmatch(iss, -1) {
		if m[1] == "core" || m[1] == "scheduled" || m[1] == "proactive" {
			win = append(win, m[1]+"|"+m[2])
		}
	}
	tx := pageTexts("linux", Layout{})
	var ours []string
	for _, p := range tx.Parts {
		ours = append(ours, p.ID+"|"+p.Sentence)
	}
	if strings.Join(win, "\n") != strings.Join(ours, "\n") || len(ours) != 3 {
		t.Errorf("parts differ from the Windows installer:\nWindows:\n%s\nours:\n%s", strings.Join(win, "\n"), strings.Join(ours, "\n"))
	}
	if !tx.Parts[0].Fixed || tx.Parts[1].Fixed || tx.Parts[2].Fixed {
		t.Error("only the check on demand is fixed")
	}
	if !strings.HasPrefix(tx.Parts[0].Sentence, "Check on demand (always included)") ||
		!strings.HasPrefix(tx.Parts[1].Sentence, "Weekly check") || !strings.HasPrefix(tx.Parts[2].Sentence, "Alerts") {
		t.Error("part names")
	}
	page := string(pageHTML)
	if !strings.Contains(page, `fixed ? ' checked disabled' : ''`) {
		t.Error("the page does not tick and lock the fixed part, and leave the others clear")
	}
}

// ND-301 words: the Ookla step carries the Windows Ookla page's words, without "command-line".
func TestInstallOoklaWordsMatchWindows(t *testing.T) {
	iss := repoFile(t, "installer/netdoctor.iss")
	pick := func(re string) string {
		m := regexp.MustCompile(re).FindStringSubmatch(iss)
		if m == nil {
			t.Fatalf("not found in the Windows installer: %s", re)
		}
		return issStrings(m[1])
	}
	tx := pageTexts("linux", Layout{}).Ookla
	cases := map[string]string{
		tx.Title:    pick(`CreateCustomPage\(wpSelectComponents, ('[^']*')`),
		tx.Lead:     pick(`CreateCustomPage\(wpSelectComponents, '[^']*', ('(?:[^']|'')*')`),
		tx.Note:     strings.Replace(pick(`(?s)Note.Caption := ('Ookla Speedtest is.*?);`), "free command-line tool", "free tool", 1),
		tx.Offer:    pick(`OoklaOffer.Caption := ('[^;]*);`),
		tx.Agree:    pick(`OoklaAgree.Caption := ('[^;]*);`),
		tx.Eula:     pick(`AddLink\(OoklaPage, ('[^']*'), Y, @OpenEula\)`),
		tx.Terms:    pick(`AddLink\(OoklaPage, ('[^']*'), Y, @OpenTerms\)`),
		tx.NeedTick: pick(`MsgBox\(('To download the tool from Ookla[^;]*), mbInformation`),
		tx.CliPage:  pick(`AddLink\(OoklaPage, ('Open Ookla''s Speedtest CLI page'), Y, @OpenCliPage\)`),
	}
	for ours, win := range cases {
		if ours != win {
			t.Errorf("Ookla words differ:\nours:    %q\nWindows: %q", ours, win)
		}
	}
	if strings.Contains(strings.ToLower(tx.Note), "command-line") {
		t.Error("the tool is called a command-line tool")
	}
	if tx.EulaURL != "https://www.speedtest.net/about/eula" || tx.TermsURL != "https://www.speedtest.net/about/terms" ||
		tx.CliURL != "https://www.speedtest.net/apps/cli" {
		t.Error("license links")
	}
}
