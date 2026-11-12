package app

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/store"
	"github.com/rafiqkmateen/network-doctor/internal/web"
)

const help = `Network Doctor %s - looks at your connection and explains what it finds.
It only reads. It never changes a setting, driver, device or program.

Commands:
  check [--json] [--quick] [--plan MBPS]   run a full check now
  weekly [--plan MBPS]                     run one weekly check and record it
  watch [--interval SECONDS]               watch for drops, slow speed, new devices
  serve [--listen ADDR] [--allow CIDRS]    open the local web page (default 127.0.0.1:8765)
  known add MAC [NAME] | known list        mark devices you recognize
  config show | plan N | name TEXT | quiet FROM TO | sinks LIST | alerts-file on|off | alerts-off LIST
  schedule enable | disable | status       turn the weekly check on or off
  version

Exit codes for check and weekly: 0 healthy, 10 this computer, 11 cable or signal,
12 router, 13 provider, 2 bad command.
`

// Run runs one command and returns the exit code.
func Run(ctx context.Context, args []string, src collect.Source, dir string, out io.Writer) int {
	if len(args) == 0 {
		args = []string{"check"}
	}
	cfgPath := filepath.Join(dir, "config.json")
	cfg, err := store.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintln(out, "Could not read the settings file:", err)
		return 2
	}
	known, _ := store.LoadKnown(filepath.Join(dir, "known-devices.txt"))
	cmd, rest := args[0], args[1:]
	fs := flag.NewFlagSet(cmd, flag.ContinueOnError)
	fs.SetOutput(out)
	plan := fs.Float64("plan", cfg.PlanMbps, "subscribed download speed in Mbps")
	asJSON := fs.Bool("json", false, "print JSON")
	quick := fs.Bool("quick", false, "skip the speed test, history and device list")
	interval := fs.Int("interval", 60, "seconds between watch checks")
	listen := fs.String("listen", "127.0.0.1:8765", "address for the web page")
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
		rep := Check(ctx, src, Options{PlanMbps: *plan, Quick: *quick, Known: known})
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
		fmt.Fprintf(out, "Network Doctor page: http://%s/  (Ctrl+C to stop)\n", *listen)
		err := web.Serve(ctx, *listen, al, func() ([]byte, error) {
			return json.Marshal(Check(ctx, src, Options{PlanMbps: *plan, Known: known}))
		})
		if err != nil {
			fmt.Fprintln(out, "Could not serve:", err)
			return 2
		}
		return 0
	case "known":
		return knownCmd(rest, dir, known, out)
	case "config":
		return configCmd(rest, cfgPath, cfg, out)
	case "schedule":
		return scheduleCmd(rest, cfg, out)
	}
	fmt.Fprintf(out, "Unknown command %q.\n\n", cmd)
	fmt.Fprintf(out, help, Version)
	return 2
}

func weekly(ctx context.Context, src collect.Source, dir string, cfg store.Config, plan float64, known map[string]bool, out io.Writer) int {
	rep := Check(ctx, src, Options{PlanMbps: plan, Known: known})
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
	sinks := parseSinks(cfg.Sinks)
	if len(sinks) == 0 {
		sinks = ac.Sinks
	}
	w := NewWatcher(src, core.NewAlerter(ac), Options{PlanMbps: plan, Known: known})
	fmt.Fprintf(out, "Watching every %d seconds. Alerts go to: %v. Ctrl+C to stop.\n", interval, sinks)
	t := time.NewTicker(time.Duration(interval) * time.Second)
	defer t.Stop()
	n := 0
	for {
		for _, a := range w.Tick(ctx, time.Now(), n%30 == 0) {
			for _, p := range Emit(a, sinks, dir, out) {
				fmt.Fprintln(out, "Alert problem:", p)
			}
		}
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
		b, _ := json.MarshalIndent(cfg, "", "  ")
		fmt.Fprintln(out, string(b))
		return 0
	}
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
		cfg.Sinks = strings.Split(args[1], ",")
	case args[0] == "alerts-file" && len(args) == 2 && (args[1] == "on" || args[1] == "off"):
		var keep []string
		for _, k := range cfg.Sinks {
			if strings.TrimSpace(k) != string(core.SinkFile) {
				keep = append(keep, k)
			}
		}
		if args[1] == "on" {
			keep = append(keep, string(core.SinkFile))
		}
		cfg.Sinks = keep
	case args[0] == "alerts-off" && len(args) == 2:
		cfg.AlertsOff = strings.Split(args[1], ",")
	default:
		fmt.Fprintln(out, "Use: config show | plan N | name TEXT | quiet FROM TO | sinks console,file,screen | alerts-file on|off | alerts-off drop,slow,new_device")
		return 2
	}
	if err := store.SaveConfig(path, cfg); err != nil {
		fmt.Fprintln(out, "Could not save:", err)
		return 2
	}
	fmt.Fprintln(out, "Saved.")
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
			if !d.Known {
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
