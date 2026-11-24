package app

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/store"
	"github.com/rafiqkmateen/network-doctor/internal/web"
)

const help = `Network Doctor %s - looks at your connection and explains what it finds.
Checks only read. The only commands that change a setting are under "fix", and
each one needs your yes to that one fix. It never changes drivers or security.

Commands:
  check [--json] [--quick] [--plan MBPS]   run a full check now
  weekly [--plan MBPS]                     run one weekly check and record it
  watch [--interval SECONDS]               watch for drops, slow speed, new devices
  open [--plan MBPS]                       run a check in a page in your web browser; quits when the page closes
  serve [--listen ADDR] [--allow CIDRS] [--https]   open the local web page (default 127.0.0.1:8765)
  setup                                    answer a few questions to set things up
  share [--light] [--http]                 show how to open the page on your phone (private by default)
  known add MAC [NAME] | known list       mark devices you recognize
  config show | plan N | name TEXT | quiet FROM TO | sinks LIST | alerts-file on|off | alerts-off LIST | known-from PATH|off
  schedule enable | disable | status       turn the weekly check on or off
  fix list | apply NAME [--yes] | undo NAME   small fixes, one yes each, always undoable
  install [--silent] [--parts=scheduled,proactive]   Mac and Linux: set it up for you, with icons
  uninstall [--silent [--keep-results]]              Mac and Linux: take it all away, results too
  version

Exit codes for check and weekly: 0 healthy, 10 this computer, 11 cable or signal,
12 router, 13 provider, 2 bad command.
`

// Run runs one command and returns the exit code. Nobody can type to it.
func Run(ctx context.Context, args []string, src collect.Source, dir string, out io.Writer) int {
	return RunWith(ctx, args, src, dir, Terminal{}, out)
}

// RunWith is Run for a caller that may have a person at a keyboard. Only then,
// with no command and no settings file, does the first-run helper start.
func RunWith(ctx context.Context, args []string, src collect.Source, dir string, term Terminal, out io.Writer) int {
	cfgPath := filepath.Join(dir, "config.json")
	if len(args) == 0 {
		if _, err := os.Stat(cfgPath); err != nil && term.Keyboard && term.In != nil {
			Setup(ctx, src, dir, term.In, out, SetupOptions{})
			fmt.Fprintln(out)
		}
		args = []string{"check"}
	}
	cfg, err := store.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintln(out, "Could not read the settings file:", err)
		return 2
	}
	// One joined list: the tool's own known list plus the list the known_from setting names.
	// A note about an unreadable list is kept for the check's report, so --json stays JSON.
	var knownNote strings.Builder
	known := newKnownList(dir, cfg.KnownFrom, &knownNote).Load()
	cmd, rest := args[0], args[1:]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(out)
	plan := fs.Float64("plan", cfg.PlanMbps, "subscribed download speed in Mbps")
	asJSON := fs.Bool("json", false, "print JSON")
	quick := fs.Bool("quick", false, "skip the speed test, history and device list")
	interval := fs.Int("interval", 60, "seconds between watch checks")
	listen := fs.String("listen", "127.0.0.1:8765", "address for the web page")
	light := fs.Bool("light", false, "draw the phone code for a light screen")
	plainHTTP := fs.Bool("http", false, "share: use a plain http address, not https")
	useHTTPS := fs.Bool("https", false, "serve: open the page over https")
	allow := fs.String("allow", strings.Join(cfg.Allow, ","), "addresses allowed to open the web page")

	switch cmd {
	case "version", "--version", "-v":
		fmt.Fprintln(out, "Network Doctor", Version)
		return 0
	case "help", "--help", "-h":
		fmt.Fprintf(out, help, Version)
		return 0
	case "check":
		if fs.Parse(rest) != nil {
			return 2
		}
		rep := runCheck(ctx, src, dir, Options{PlanMbps: *plan, Quick: *quick, Known: known,
			States: newKnownList(dir, "", nil).States(), NoNewDeviceWarnings: !cfg.NewDeviceWarnings()})
		if n := strings.TrimSpace(knownNote.String()); n != "" {
			rep.Notes = append(rep.Notes, strings.TrimPrefix(n, "Note: "))
		}
		if *asJSON {
			b, _ := json.MarshalIndent(rep, "", "  ")
			fmt.Fprintln(out, string(b))
		} else {
			printReport(out, rep)
		}
		return core.ExitCode(rep.Finding)
	case "weekly":
		if fs.Parse(rest) != nil {
			return 2
		}
		return weekly(ctx, src, dir, cfg, *plan, known, out)
	case "watch":
		if fs.Parse(rest) != nil {
			return 2
		}
		return watch(ctx, src, dir, cfg, *plan, *interval, known, out)
	case "serve":
		if fs.Parse(rest) != nil {
			return 2
		}
		var al []string
		for _, a := range strings.Split(*allow, ",") {
			if a = strings.TrimSpace(a); a != "" {
				al = append(al, a)
			}
		}
		run := pageRun(ctx, src, dir, *plan)
		var err error
		if *useHTTPS {
			ips := []net.IP{net.ParseIP("127.0.0.1"), net.ParseIP("::1")}
			if h, _, e := net.SplitHostPort(*listen); e == nil {
				if ip := net.ParseIP(h); ip != nil && !ip.IsLoopback() && !ip.IsUnspecified() {
					ips = append(ips, ip)
				}
			}
			cert, cerr := web.EnsureCert(dir, ips, time.Now())
			if cerr != nil {
				fmt.Fprintln(out, shareNoCert, cerr)
				return 2
			}
			fmt.Fprintf(out, "Network Doctor page: https://%s/  (Ctrl+C to stop)\n", *listen)
			err = web.ServeTLSSave(ctx, *listen, al, run, nil, cert, pageSaver(dir))
		} else {
			fmt.Fprintf(out, "Network Doctor page: http://%s/  (Ctrl+C to stop)\n", *listen)
			err = web.ServeSave(ctx, *listen, al, run, nil, pageSaver(dir))
		}
		if err != nil {
			fmt.Fprintln(out, "Could not serve:", err)
			return 2
		}
		return 0
	case "open": // a new press retires the older page (one page at a time)
		if fs.Parse(rest) != nil {
			return 2
		}
		run := pageRun(ctx, src, dir, *plan)
		octx, stop := context.WithCancel(ctx)
		defer stop()
		ready := func(url string) {
			if err := openBrowser(url); err != nil {
				go func() { // a message box waits for the person, so it runs beside the server
					if noBrowser(url, out) {
						stop()
					}
				}()
			}
		}
		if err := web.ServeOpen(octx, run, web.OpenOptions{Idle: openIdle, NoPage: openNoPage, Ready: ready, Save: pageSaver(dir),
			StateFile: filepath.Join(dir, openStateName)}); err != nil {
			fmt.Fprintln(out, "Could not open the page:", err)
			return 2
		}
		return 0
	case "setup":
		if term.In == nil {
			fmt.Fprintln(out, setupNoInput)
			return 2
		}
		return Setup(ctx, src, dir, term.In, out, SetupOptions{})
	case "share":
		if fs.Parse(rest) != nil {
			return 2
		}
		return Share(ctx, src, ShareOptions{LightTerm: *light, CertDir: dir, Plain: *plainHTTP}, pageOptions(dir, *plan), out)
	case "fix":
		return fixCmd(rest, dir, out)
	case "known":
		own, _ := store.LoadKnown(filepath.Join(dir, "known-devices.txt")) // only the devices marked here
		return knownCmd(rest, dir, own, out)
	case "config":
		return configCmd(rest, cfgPath, cfg, out)
	case "schedule":
		return scheduleCmd(rest, cfg, out)
	case "install", "uninstall": // Mac and Linux setup; Windows has its own setup file
		return installCmd(ctx, cmd, rest, dir, out)
	}
	fmt.Fprintf(out, "Unknown command %q.\n\n", cmd)
	fmt.Fprintf(out, help, Version)
	return 2
}

// pageRun is one full check for the web page. It reads the settings and both device lists again
// each time, so a choice made on the page counts at the next check.
func pageRun(ctx context.Context, src collect.Source, dir string, plan float64) func() ([]byte, error) {
	return func() ([]byte, error) {
		return json.Marshal(runCheck(ctx, src, dir, pageOptions(dir, plan)))
	}
}

// pageOptions are the options for a page's check: the device lists and the new-device switch are
// read again at every check (see Check), so a choice made on the page counts at the next one.
func pageOptions(dir string, plan float64) Options {
	cfgPath := filepath.Join(dir, "config.json")
	from := func() string { c, _ := store.LoadConfig(cfgPath); return c.KnownFrom }
	return Options{
		PlanMbps:   plan,
		LoadKnown:  func() map[string]bool { return newKnownList(dir, from(), nil).Load() },
		LoadStates: newKnownList(dir, "", nil).States,
		NewDeviceWarningsOff: func() bool {
			c, err := store.LoadConfig(cfgPath)
			return err == nil && !c.NewDeviceWarnings()
		},
	}
}

// pageSaver stores what the page on this computer sends: device choices go to the same known list
// the known command writes, and the switch to the settings file. Nothing else is written, and
// nothing on the computer or the network is changed (ND-264, ND-265).
func pageSaver(dir string) *web.Saver {
	var mu sync.Mutex
	knownPath, cfgPath := filepath.Join(dir, "known-devices.txt"), filepath.Join(dir, "config.json")
	return &web.Saver{
		Devices: func(list []web.Mark) error {
			mu.Lock()
			defer mu.Unlock()
			changes := make([]store.KnownChange, 0, len(list))
			for _, m := range list {
				changes = append(changes, store.KnownChange{MAC: m.MAC, State: store.KnownState(m.State), Name: m.Name})
			}
			return store.SetKnownMany(knownPath, changes) // one write: all of them or none
		},
		Settings: func(s web.Settings) error {
			mu.Lock()
			defer mu.Unlock()
			on := s.WarnNewDevices
			// Changed under the file's lock on the settings as they are then, so nothing is lost.
			return store.UpdateConfig(cfgPath, func(c *store.Config) error { c.WarnNewDevices = &on; return nil })
		},
	}
}

func weekly(ctx context.Context, src collect.Source, dir string, cfg store.Config, plan float64, known map[string]bool, out io.Writer) int {
	rep := runCheck(ctx, src, dir, Options{PlanMbps: plan, Known: known})
	if !rep.Reading.SpeedMeasured {
		// A made-up zero would look like a slow week and could start a false complaint.
		fmt.Fprintln(out, "Weekly check not saved: the speed was not measured, and a zero would look like a slow week. Run it again later.")
		return 0
	}
	csvPath := filepath.Join(dir, "weekly.csv")
	err := store.AppendWeekly(csvPath, store.Row{Time: rep.Time, Down: rep.Reading.DownMbps, Up: rep.Reading.UpMbps, PingMs: rep.Reading.InternetMs, Plan: plan})
	if err != nil {
		fmt.Fprintln(out, "Could not save the weekly record:", err)
		return 2
	}
	rows, _ := store.ReadWeekly(csvPath)
	var weeks []core.Week
	for _, r := range rows {
		weeks = append(weeks, core.Week{Time: r.Time, DownMbps: r.Down, PlanMbps: r.Plan})
	}
	fmt.Fprintf(out, "Weekly check saved to %s: %.0f Mbps down, %.0f up, %.0f ms.\n", csvPath, rep.Reading.DownMbps, rep.Reading.UpMbps, rep.Reading.InternetMs)
	if core.ShouldDraftComplaint(weeks) {
		p := filepath.Join(dir, "complaint-draft.txt")
		if err := os.WriteFile(p, []byte(core.ComplaintDraft(weeks, cfg.Name)), 0o644); err == nil {
			fmt.Fprintf(out, "Three weeks in a row under half your plan. A draft email is saved at %s. It has not been sent.\n", p)
		}
	}
	return core.ExitCode(rep.Finding)
}

func watch(ctx context.Context, src collect.Source, dir string, cfg store.Config, plan float64, interval int, known map[string]bool, out io.Writer) int {
	ac := core.DefaultAlertConfig()
	ac.QuietFromHour, ac.QuietToHour = cfg.QuietFrom, cfg.QuietTo
	for _, k := range cfg.AlertsOff {
		ac.Enabled[core.AlertKind(strings.TrimSpace(k))] = false
	}
	sinks := alertSinks(cfg)
	// The lists are read again at every full check, so a device marked known meanwhile counts.
	kl := newKnownList(dir, cfg.KnownFrom, out)
	cfgPath := filepath.Join(dir, "config.json")
	w := NewWatcher(src, core.NewAlerter(ac), Options{PlanMbps: plan, Known: known, LoadKnown: kl.Load, LoadStates: kl.States,
		// The page's switch is read again at every check, so turning it off counts at once (ND-265).
		NewDeviceWarningsOff: func() bool { c, err := store.LoadConfig(cfgPath); return err == nil && !c.NewDeviceWarnings() }})
	fmt.Fprintf(out, "Watching every %d seconds. Alerts go to: %v. Ctrl+C to stop.\n", interval, sinks)
	t := time.NewTicker(time.Duration(interval) * time.Second)
	defer t.Stop()
	n := 0
	said := map[string]bool{}       // each problem is logged once, not at every alert (ND-288)
	first := testAlerts(time.Now()) // the proof switch: one test alert right after the start, when asked
	for {
		for _, a := range append(first, w.Tick(ctx, time.Now(), n%30 == 0)...) {
			for _, p := range Emit(a, sinks, dir, out) {
				if !said[p] {
					said[p] = true
					fmt.Fprintln(out, "Alert problem:", p)
				}
			}
		}
		first = nil
		n++
		select {
		case <-ctx.Done():
			return 0
		case <-t.C:
		}
	}
}

func knownCmd(args []string, dir string, known map[string]bool, out io.Writer) int {
	p := filepath.Join(dir, "known-devices.txt")
	switch {
	case len(args) >= 2 && args[0] == "add":
		if err := store.AddKnown(p, args[1], strings.Join(args[2:], " ")); err != nil {
			fmt.Fprintln(out, "Could not save:", err)
			return 2
		}
		fmt.Fprintln(out, "Marked as known:", core.NormalizeMAC(args[1]))
		return 0
	case len(args) == 1 && args[0] == "list":
		for m := range known {
			fmt.Fprintln(out, m)
		}
		return 0
	}
	fmt.Fprintln(out, "Use: known add MAC [NAME]   or   known list")
	return 2
}

func configCmd(args []string, path string, cfg store.Config, out io.Writer) int {
	if len(args) == 0 || args[0] == "show" {
		if cfg.WarnNewDevices == nil { // show the switch even before it was ever changed: on
			on := true
			cfg.WarnNewDevices = &on
		}
		b, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Fprintln(out, string(b))
		return 0
	}
	// The change is made to the settings as they are under the file's lock, so a change saved
	// meanwhile (the page's switch, say) is not lost (ND-265).
	code := 0
	err := store.UpdateConfig(path, func(c *store.Config) error {
		if code = applyConfig(args, c, out); code != 0 {
			return errNotSaved
		}
		return nil
	})
	if code != 0 {
		return code
	}
	if err != nil {
		fmt.Fprintln(out, "Could not save:", err)
		return 2
	}
	fmt.Fprintln(out, "Saved.")
	return 0
}

// errNotSaved stops a settings change that was refused, so nothing is written.
var errNotSaved = errors.New("not saved")

// applyConfig makes one `config` change to cfg; it returns 2 when the change is refused.
func applyConfig(args []string, cfg *store.Config, out io.Writer) int {
	switch {
	case args[0] == "plan" && len(args) == 2:
		v, err := strconv.ParseFloat(args[1], 64)
		if err != nil || v <= 0 {
			fmt.Fprintln(out, "The plan speed must be a number above zero.")
			return 2
		}
		cfg.PlanMbps = v
	case args[0] == "name" && len(args) >= 2:
		cfg.Name = strings.Join(args[1:], " ")
	case args[0] == "quiet" && len(args) == 3:
		a, e1 := strconv.Atoi(args[1])
		b, e2 := strconv.Atoi(args[2])
		if e1 != nil || e2 != nil || a < 0 || a > 23 || b < 0 || b > 23 {
			fmt.Fprintln(out, "Quiet hours are two hours from 0 to 23, such as: quiet 22 6")
			return 2
		}
		cfg.QuietFrom, cfg.QuietTo = a, b
	case args[0] == "sinks" && len(args) == 2:
		cfg.Sinks, cfg.SinksSaved = strings.Split(args[1], ","), true
	case args[0] == "alerts-file" && len(args) == 2 && (args[1] == "on" || args[1] == "off"):
		// Start from where alerts really go: with the alerts part installed and nothing saved, that is
		// the console, the file and the screen, not the stored default.
		base, on := cfg.Sinks, args[1] == "on"
		if !cfg.SinksSaved {
			base = nil
			for _, s := range alertSinks(*cfg) {
				base = append(base, string(s))
			}
		}
		var keep []string
		hasFile := false
		for _, k := range base {
			if strings.TrimSpace(k) == string(core.SinkFile) {
				hasFile = true
				if !on {
					continue
				}
			}
			keep = append(keep, k)
		}
		if on && !hasFile {
			keep = append(keep, string(core.SinkFile))
		}
		cfg.Sinks, cfg.SinksSaved = keep, true
	case args[0] == "alerts-off" && len(args) == 2:
		cfg.AlertsOff = strings.Split(args[1], ",")
	case args[0] == "known-from" && len(args) == 2:
		if args[1] == "off" {
			cfg.KnownFrom = ""
			break
		}
		p, err := filepath.Abs(args[1]) // the watcher may start in another folder
		if err != nil {
			p = args[1]
		}
		cfg.KnownFrom = p
		if _, err := store.LoadKnownFrom(p); err != nil {
			fmt.Fprintln(out, "Note: that file cannot be read as a list of devices right now. It is saved anyway and read again each time.")
		} else if _, err := os.Stat(p); err != nil {
			fmt.Fprintln(out, "Note: there is no file there yet. Only your own known list counts until there is.")
		}
	default:
		fmt.Fprintln(out, "Use: config show | plan N | name TEXT | quiet FROM TO | sinks console,file,screen | alerts-file on|off | alerts-off drop,slow,new_device | known-from PATH|off")
		return 2
	}
	return 0
}

func printReport(w io.Writer, r Report) {
	fmt.Fprintf(w, "\nNetwork Doctor %s   %s\n", Version, r.Time.Format("2006-01-02 1504"))
	fmt.Fprintln(w, strings.Repeat("-", 60))
	fmt.Fprintf(w, "ANSWER: %s\n", r.Finding.Summary)
	if r.Finding.Cause != core.CauseNone {
		fmt.Fprintf(w, "Check first: %s\n", r.Finding.First)
	}
	rd := r.Reading
	fmt.Fprintf(w, "\nConnection: %s", rd.Kind)
	if rd.LinkMbps > 0 {
		fmt.Fprintf(w, " at %d Mbps", rd.LinkMbps)
	}
	fmt.Fprintf(w, "\nTo the router:   %.0f ms, %.0f%% lost\n", rd.RouterMs, rd.RouterLoss)
	fmt.Fprintf(w, "To the internet: %.0f ms, %.0f%% lost\n", rd.InternetMs, rd.InternetLoss)
	if rd.DownMbps > 0 {
		fmt.Fprintf(w, "Speed: %.0f Mbps down, %.0f up", rd.DownMbps, rd.UpMbps)
		if rd.PlanMbps > 0 {
			fmt.Fprintf(w, " (plan %.0f)", rd.PlanMbps)
		}
		fmt.Fprintln(w)
	}
	for _, a := range r.LinkAdvice {
		fmt.Fprintln(w, " -", a)
	}
	if r.WiFiHidden != nil {
		fmt.Fprintln(w, r.WiFiHidden.Headline)
		fmt.Fprintln(w, r.WiFiHidden.Why)
	}
	if r.ExitPlace != "" {
		fmt.Fprintf(w, "Your internet leaves from %s.\n", r.ExitPlace)
	}
	if r.VPNHint != nil {
		fmt.Fprintln(w, r.VPNHint.Headline)
		fmt.Fprintln(w, r.VPNHint.Why)
	}
	if !r.Quick {
		fmt.Fprintf(w, "\nDrops in the last 7 days: %d (plus %d blips under %s)\n", r.Drops.Count, r.Drops.BriefBlips, core.MinDrop)
		for _, d := range r.Drops.Drops {
			fmt.Fprintf(w, " - %s for %s\n", d.Start.Format("2006-01-02 1504"), d.Length.Round(time.Second))
		}
		if r.Drops.Pattern != "" {
			fmt.Fprintln(w, r.Drops.Pattern)
		}
	}
	fmt.Fprintln(w, "\nLine use right now:")
	for _, a := range r.BandwidthAdvice {
		fmt.Fprintln(w, " -", a)
	}
	if len(r.Devices) > 0 {
		fmt.Fprintf(w, "\nDevices on the network (%d, %d not marked known):\n", len(r.Devices), len(r.Unknown))
		for _, d := range r.Devices {
			mark := ""
			switch {
			case d.State == string(store.Ignored):
				mark = "  (ignored)"
			case d.State == string(store.NotMine):
				mark = "  <- marked not mine"
			case !d.Known:
				mark = "  <- not marked known"
			}
			name := d.Name
			if name == "" {
				name = "(no name)"
			}
			fmt.Fprintf(w, " - %-20s %-15s %-17s %s%s\n", name, d.IP, d.MAC, d.Maker, mark)
		}
	}
	fmt.Fprintln(w, "\nSettings review:")
	for _, a := range r.Settings {
		fmt.Fprintf(w, " - %s: %s %s\n", a.Area, a.Finding, a.Effect)
	}
	for _, n := range r.Notes {
		fmt.Fprintln(w, "Note:", n)
	}
}

// openStateName is the note in the user's data folder naming the running open page, so a new press
// can retire it: one page at a time.
const openStateName = "open-page.json"
