package install

import (
	"strings"

	"github.com/rafiqkmateen/network-doctor/internal/fix"
)

// Every word the setup and removal pages show lives here, in Go, so one test can read them all
// (ND-05, ND-280, ND-314). The page itself holds no words of its own.

// The parts, word for word as the Windows installer's parts list (ND-273).
const (
	partCore = "Check on demand (always included). Run a full check any time you want one. Your data never leaves " +
		"your computer. The speed test and a few small requests do go out: see What goes over the internet in the guide."
	partWeekly = "Weekly check. Once a week, Monday at 0900, saves your speed to a file. Your data never leaves your " +
		"computer. The speed test and a few small requests do go out: see What goes over the internet in the guide."
	partAlerts = "Alerts. A small helper starts when you log on and runs all the time. It alerts you when your " +
		"connection drops, stays slow (if you set your plan speed), or a new device appears. Alerts are kept in a file " +
		"and shown on the screen, and setup can turn off the pop-ups and the file. Your data never leaves your computer. " +
		"The speed test and a few small requests do go out: see What goes over the internet in the guide."
)

// The Ookla step, word for word as the Windows Ookla page, except that it is not called a
// "command-line tool" and the file is named for Mac and Linux (ND-301).
const (
	ooklaTitle = "Speed tool (optional)"
	ooklaLead  = "Network Doctor can measure your speed with Ookla Speedtest. This is your choice."
	ooklaNote  = "Ookla Speedtest is a free tool from Ookla, the company behind speedtest.net. " +
		"Ookla allows it for your own personal, non-commercial use on a computer you own. It is not part of Network Doctor, " +
		"Network Doctor never passes it to anyone, and without it Network Doctor measures speed with its own, lighter test. " +
		"Each Ookla test uses about two and a half gigabytes of data."
	ooklaOffer    = "Download Ookla Speedtest now, straight from Ookla"
	ooklaAgree    = "I have read the Ookla license and privacy notice and accept them for my own personal use"
	ooklaEula     = "Read the Ookla license (EULA)"
	ooklaTerms    = "Read the Ookla terms of use and privacy policy"
	ooklaNeedTick = "To download the tool from Ookla you need to tick that you have read and accept its license. " +
		"Or clear the download box and install it yourself later."
	ooklaSelf    = "Not sure yet? Leave both boxes empty. You can run this setup again later and tick both boxes to add the tool."
	ooklaCliPage = "Open Ookla's Speedtest CLI page"

	ooklaEulaURL  = "https://www.speedtest.net/about/eula"
	ooklaTermsURL = "https://www.speedtest.net/about/terms"
	ooklaCliURL   = "https://www.speedtest.net/apps/cli"
)

// The install page's other words.
const (
	txTitle       = "Network Doctor Setup"
	txWelcomeHead = "Set up Network Doctor"
	txWelcome     = "Network Doctor checks your internet connection and tells you in plain words what it finds. " +
		"Pick the parts you want. You can run this setup again later to change them."
	txPartsHead  = "Parts to install"
	txNext       = "Next"
	txBack       = "Back"
	txInstall    = "Install"
	txReadyHead  = "Ready to install"
	txReadyDir   = "Network Doctor goes in this folder:"
	txReadyParts = "Parts to install:"
	txReadySpeed = "Speed tool:"
	txReadyOokla = "Download Ookla Speedtest from Ookla"
	txReadyNone  = "Not added (Network Doctor uses its own speed test)"
	txShortCore  = "Check on demand"
	txShortWeek  = "Weekly check"
	txShortAlert = "Alerts"
	txWorking    = "Installing Network Doctor. This takes a moment."
	txFailed     = "Network Doctor could not finish the install. Close this page and open the setup file again to try once more."
	txClosePage  = "You can close this browser tab now."
	txGone       = "This setup page has closed. Open the setup file again to start over."
)

// The finish screen (ND-279), in the Windows finish screen's words with the system's own names.
const (
	finDone      = "Network Doctor is installed."
	finRunMac    = "To run a check, press the Network Doctor icon on your Desktop or in your Applications folder."
	finRunLinux  = "To run a check, press the Network Doctor icon on your Desktop or in your applications menu."
	finRunRest   = "A page opens in your web browser, runs the check and shows the answer. Close the browser tab when you are done."
	finWeeklyOn  = "Weekly check: on. It saves your speed to a file every Monday at 0900."
	finWeeklyOff = "Weekly check: not chosen."
	finAlertsOn  = "Alerts: on. They start the next time you log in."
	finAlertsOff = "Alerts: not chosen."
	finProgram   = "The program is in %s."
	finResults   = "Your results are saved in %s."
	finRemoveMac = "To remove it, open Uninstall Network Doctor in your Applications folder. " + finRemoveAll
	finRemoveLin = "To remove it, open Uninstall Network Doctor in your applications menu. " + finRemoveAll
	finRemoveAll = "It takes away everything, your saved results and settings too, so a new install starts fresh. " +
		"It first tries to put back any setting a fix changed."
	// On Mac and Linux the guide is shown by the page itself, from the setup page's guide route: on a
	// Mac the program folder is inside the app, which Finder will not open (ND-279, amended).
	finGuideLine  = "The full guide explains every part."
	finGuideOpen  = "Open the guide"
	finAllowLinux = "The first time you open the Desktop icon, some desktops ask once: right-click it and choose \"Allow Launching\". " +
		"The menu launcher needs nothing."
	finOoklaOn  = "Speed tool: Ookla Speedtest is added."
	finOoklaOff = "The speed tool was not added. Network Doctor will use its own speed test. " +
		"You can add the tool yourself later from Ookla's page."
)

// The removal page (ND-291): no keep-or-delete question; it says everything goes, and one button.
const (
	unTitle = "Remove Network Doctor"
	unAsk   = "Remove Network Doctor and everything it saved?"
	unNote  = "This takes away the program, its icons, the weekly check and the alerts. " +
		"It also deletes everything it saved: your results, the alert log, your list of devices and your settings. " +
		"It first tries to put back any setting a fix changed. If you install it again, it starts fresh."
	unButton   = "Remove Network Doctor"
	unWorking  = "Removing Network Doctor. This takes a moment."
	unDone     = "Network Doctor is removed."
	unDeleted  = "Everything it saved is deleted too, so a new install starts fresh."
	unFailed   = "Some parts of Network Doctor could not be removed. Open Uninstall Network Doctor again to try once more."
	unNotThere = "Network Doctor is not installed for you, so there is nothing to remove."
)

// A change a fix made that removal could not put back (ND-316). The same words go to the removal
// page, the silent removal and the Windows uninstaller's note.
const (
	nuHead   = "Network Doctor could not put back every change its fixes made."
	nuItem   = "Not put back: %s."
	nuNotes  = "Not put back: the changes in its notes, because the notes could not be read."
	nuAdmin  = "This needs administrator rights, and removal never asks for them."
	nuFailed = "The computer did not make the change, or the setting could not be read."
	// The note was made by another user account, or before the account was kept, so removal
	// cannot tell whether the setting was changed by hand.
	nuAccount = "Another user account set it, or the notes do not say who did, so removal left it alone."
	// A setting the person changed by hand since the fix is settled, not a fault (ND-316).
	nuSettled     = "%s: left as you set it, because you changed it yourself after the fix."
	nuSettledName = "That setting"
	nuKept        = "The notes for it are kept in %s, so it can still be put back."
	nuNext        = "To do that, install Network Doctor again and use its undo, as the guide shows under Fix a problem. Or change the setting back by hand."
	nuRestGone    = "Everything else it saved is deleted."
	nuRestKept    = "Your saved results are kept too."
)

// Lines for the silent install and removal, and for Windows.
const (
	outInstalled    = "Network Doctor is installed."
	outRemoved      = "Network Doctor is removed."
	outRemovedKept  = "Network Doctor is removed. Your saved results are kept in %s."
	outKeepSilent   = "--keep-results works only with --silent: uninstall --silent --keep-results."
	outWindowsSetup = "On Windows, install Network Doctor with its setup file, NetworkDoctor-Setup."
	outWindowsGone  = "On Windows, remove Network Doctor in Windows Settings, Apps, Installed apps."
	outBadPart      = "Unknown part %q. The parts are scheduled and proactive, such as --parts=scheduled,proactive."
	outNoBrowser    = "Could not open your web browser. Open this address in it: %s"
)

// Problems: one plain sentence each, never a system error (those go to the log only).
const (
	probFolder      = "Could not make the Network Doctor folder."
	probProgram     = "Could not put the program in place."
	probIcons       = "Could not make the Network Doctor icons."
	probChoices     = "Could not save your choices."
	probJobs        = "Could not set up the weekly check or the alerts."
	probRemove      = "Could not remove every part of Network Doctor."
	probResults     = "Could not delete your saved results."
	probResultsName = "Your saved results were not deleted, because their folder is not named NetworkDoctor."
)

// Part is one part on the first step.
type Part struct {
	ID       string `json:"id"`
	Short    string `json:"short"`
	Sentence string `json:"sentence"`
	Fixed    bool   `json:"fixed"`
}

// OoklaTexts are the words of the Ookla step.
type OoklaTexts struct {
	Title    string `json:"title"`
	Lead     string `json:"lead"`
	Note     string `json:"note"`
	Offer    string `json:"offer"`
	Agree    string `json:"agree"`
	Eula     string `json:"eula"`
	EulaURL  string `json:"eula_url"`
	Terms    string `json:"terms"`
	TermsURL string `json:"terms_url"`
	NeedTick string `json:"need_tick"`
	Self     string `json:"self"`
	CliPage  string `json:"cli_page"`
	CliURL   string `json:"cli_url"`
}

// UninstallTexts are the words of the removal page.
type UninstallTexts struct {
	Ask     string `json:"ask"`
	Note    string `json:"note"`
	Button  string `json:"button"`
	Working string `json:"working"`
}

// PageTexts is everything the page shows, sent to it as JSON.
type PageTexts struct {
	Mode        string         `json:"mode"`
	Title       string         `json:"title"`
	WelcomeHead string         `json:"welcome_head"`
	Welcome     string         `json:"welcome"`
	PartsHead   string         `json:"parts_head"`
	Parts       []Part         `json:"parts"`
	Ookla       OoklaTexts     `json:"ookla"`
	Next        string         `json:"next"`
	Back        string         `json:"back"`
	Install     string         `json:"install"`
	ReadyHead   string         `json:"ready_head"`
	ReadyDir    string         `json:"ready_dir"`
	Folder      string         `json:"folder"`
	ReadyParts  string         `json:"ready_parts"`
	ReadySpeed  string         `json:"ready_speed"`
	ReadyOokla  string         `json:"ready_ookla"`
	ReadyNone   string         `json:"ready_none"`
	Working     string         `json:"working"`
	Failed      string         `json:"failed"`
	ClosePage   string         `json:"close_page"`
	Gone        string         `json:"gone"`
	GuideLine   string         `json:"guide_line"`
	GuideOpen   string         `json:"guide_open"`
	Uninstall   UninstallTexts `json:"uninstall"`
	Installed   bool           `json:"installed"`
	NotThere    string         `json:"not_there"`
}

// pageTexts builds the words for one system; l gives the folder shown on the ready step.
func pageTexts(goos string, l Layout) PageTexts {
	return PageTexts{
		Mode: "install", Title: txTitle, WelcomeHead: txWelcomeHead, Welcome: txWelcome, PartsHead: txPartsHead,
		Parts: []Part{
			{ID: "core", Short: txShortCore, Sentence: partCore, Fixed: true},
			{ID: "scheduled", Short: txShortWeek, Sentence: partWeekly},
			{ID: "proactive", Short: txShortAlert, Sentence: partAlerts},
		},
		Ookla: OoklaTexts{Title: ooklaTitle, Lead: ooklaLead, Note: ooklaNote, Offer: ooklaOffer, Agree: ooklaAgree,
			Eula: ooklaEula, EulaURL: ooklaEulaURL, Terms: ooklaTerms, TermsURL: ooklaTermsURL, NeedTick: ooklaNeedTick,
			Self: ooklaSelf, CliPage: ooklaCliPage, CliURL: ooklaCliURL},
		Next: txNext, Back: txBack, Install: txInstall, ReadyHead: txReadyHead, ReadyDir: txReadyDir, Folder: l.ProgramDir,
		ReadyParts: txReadyParts, ReadySpeed: txReadySpeed, ReadyOokla: txReadyOokla, ReadyNone: txReadyNone,
		Working: txWorking, Failed: txFailed, ClosePage: txClosePage, Gone: txGone,
		GuideLine: finGuideLine, GuideOpen: finGuideOpen,
	}
}

// uninstallTexts builds the words of the removal page.
func uninstallTexts(goos, results string) UninstallTexts {
	return UninstallTexts{Ask: unAsk, Note: unNote, Button: unButton, Working: unWorking}
}

// removalPageTexts is the removal page's whole set of words.
func removalPageTexts(goos string, l Layout) PageTexts {
	t := pageTexts(goos, l)
	t.Mode, t.Title, t.Uninstall = "uninstall", unTitle, uninstallTexts(goos, l.Results)
	t.Installed, t.NotThere = installed(l), unNotThere
	return t
}

// FinishLines builds the finish screen for the parts chosen in this run (ND-279), with the
// folders the install really used. r says how Ookla's tool went.
func FinishLines(goos, programDir, resultsDir string, c Choice, r Result) []string {
	run, remove := finRunLinux, finRemoveLin
	if goos == "darwin" {
		run, remove = finRunMac, finRemoveMac
	}
	lines := []string{finDone, run + " " + finRunRest}
	if c.Weekly {
		lines = append(lines, finWeeklyOn)
	} else {
		lines = append(lines, finWeeklyOff)
	}
	if c.Alerts {
		lines = append(lines, finAlertsOn)
	} else {
		lines = append(lines, finAlertsOff)
	}
	switch {
	case r.OoklaAdded:
		lines = append(lines, finOoklaOn)
	case r.OoklaTried:
		lines = append(lines, finOoklaOff)
	}
	lines = append(lines, sprintf(finProgram, programDir), sprintf(finResults, resultsDir), remove)
	if goos != "darwin" {
		lines = append(lines, finAllowLinux)
	}
	return lines
}

// removedLines is the last screen of the removal page, which always deletes the results.
func removedLines(problems []string) []string {
	if len(problems) > 0 {
		return append([]string{unFailed}, problems...)
	}
	return []string{unDone, unDeleted, txClosePage}
}

// settledNames name, in plain words, the setting each fix changed.
var settledNames = map[string]string{"proxy": "The web proxy", "power-saving": "Power saving on the network adapter",
	"public-dns": "The name servers"}

// SettledLines say, one line each, which settings removal left as the person set them, because they
// changed them by hand after the fix (ND-316). It is not a fault.
func SettledLines(settled []string) []string {
	var lines []string
	for _, n := range settled {
		name, ok := settledNames[n]
		if !ok {
			name = nuSettledName
		}
		lines = append(lines, sprintf(nuSettled, name))
	}
	return lines
}

// NotUndoneLines says, in plain words, which changes of the fixes removal could not put back, why,
// where their notes are kept and what the person can do next (ND-316). kept says whether the saved
// results were kept too. The system's own errors go to the log only, never here.
func NotUndoneLines(failures []fix.UndoFailure, notesDir string, kept bool) []string {
	lines := []string{nuHead}
	for _, f := range failures {
		if f.Reason == fix.ReasonNotes {
			lines = append(lines, nuNotes)
			continue
		}
		why := nuFailed
		switch f.Reason {
		case fix.ReasonAdmin:
			why = nuAdmin
		case fix.ReasonAccount:
			why = nuAccount
		}
		lines = append(lines, sprintf(nuItem, fix.UndoTitle(f.Name))+" "+why)
	}
	rest := nuRestGone
	if kept {
		rest = nuRestKept
	}
	return append(lines, sprintf(nuKept, notesDir), nuNext, rest)
}

func sprintf(format, s string) string { return strings.Replace(format, "%s", s, 1) }

// installTexts lists every fixed sentence the pages can show on one system, for the reading test
// and the terminal-word test, the way setupTexts lists the helper's (ND-05, ND-280, ND-314).
func installTexts(goos string) []string {
	t := removalPageTexts(goos, Layout{})
	all := []string{t.Title, t.WelcomeHead, t.Welcome, t.PartsHead, t.Next, t.Back, t.Install, t.ReadyHead,
		t.ReadyDir, t.ReadyParts, t.ReadySpeed, t.ReadyOokla, t.ReadyNone, t.Working, t.Failed, t.ClosePage, t.Gone,
		txTitle, unTitle, unAsk, unNote, unButton, unWorking, unDone, unDeleted, unFailed, unNotThere,
		nuHead, nuItem, nuNotes, nuAdmin, nuFailed, nuKept, nuNext, nuRestGone, nuRestKept, nuSettled, nuAccount,
		probFolder, probProgram, probIcons, probChoices, probJobs, probRemove, probResults, probResultsName}
	for _, p := range t.Parts {
		all = append(all, p.Short, p.Sentence)
	}
	o := t.Ookla
	all = append(all, o.Title, o.Lead, o.Note, o.Offer, o.Agree, o.Eula, o.Terms, o.NeedTick, o.Self, o.CliPage)
	run, remove := finRunLinux, finRemoveLin
	if goos == "darwin" {
		run, remove = finRunMac, finRemoveMac
	}
	return append(all, finDone, run, finRunRest, finWeeklyOn, finWeeklyOff, finAlertsOn, finAlertsOff, finProgram,
		finResults, remove, finGuideLine, finGuideOpen, finOoklaOn, finOoklaOff)
}
