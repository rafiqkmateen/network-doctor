package app

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/store"
)

// Terminal says where a person can type. A zero Terminal means nobody can.
type Terminal struct {
	In       io.Reader
	Keyboard bool // a person is at a keyboard, so the first-run helper may start by itself
}

const (
	setupWelcome   = "Welcome to Network Doctor. I will ask a few short questions. Press Enter to skip any one of them. I only change my own settings."
	setupAskPlan   = "What download speed do you pay for, in Mbps? Press Enter to skip."
	setupBadNumber = "That was not a number above zero, so I skipped it."
	setupAskName   = "What name should go on a letter to your internet company? Press Enter to skip."
	setupDevices   = "I found these devices on your network:"
	setupAskDev    = "Type the numbers of the devices that are yours, like 1 3 4, or type all. Press Enter to skip."
	setupNoDevices = "I could not list the devices on your network."
	setupNotInList = "Skipped %q. It is not on the list."
	setupAskWeekly = "Turn on the weekly check? It tests your speed once a week and saves the result in a file. Type y for yes. Press Enter for no."
	setupWeeklyOn  = "The weekly check is already on."
	setupAskFile   = "Save alerts in a file? You get one line each time the connection drops or a new device shows up. Type y for yes. Press Enter for no."
	setupAskPhone  = "Show this page on your phone now? I will print a code you can scan. Type y for yes. Press Enter for no."
	setupDone      = "All set. Type netdoctor check any time to look at your connection."
	setupNoInput   = "I need a window where you can type. Open a terminal and run: netdoctor setup"
	setupCantSave  = "I could not save your answers."
)

// setupTexts lists every sentence the helper can print, for the reading test.
func setupTexts() []string {
	return []string{setupWelcome, setupAskPlan, setupBadNumber, setupAskName, setupDevices, setupAskDev, setupNoDevices,
		setupNotInList, setupAskWeekly, setupWeeklyOn, setupAskFile, setupAskPhone, setupDone, setupNoInput, setupCantSave}
}

// SetupOptions are the three things the helper can start. Zero values are the real ones.
type SetupOptions struct {
	Status func() bool         // is the weekly check already on
	Enable func(io.Writer) int // turn the weekly check on
	Share  func() int          // show the page for phones
}

// Setup is the first-run helper. It asks, explains, and changes nothing until
// the person says yes. It saves only the tool's own settings and lists.
func Setup(ctx context.Context, src collect.Source, dir string, in io.Reader, out io.Writer, opt SetupOptions) int {
	cfgPath := filepath.Join(dir, "config.json")
	cfg, err := store.LoadConfig(cfgPath)
	if err != nil {
		fmt.Fprintln(out, "Could not read the settings file:", err)
		return 2
	}
	rd := bufio.NewReader(in)
	ask := func(q string) string {
		fmt.Fprintln(out)
		fmt.Fprintln(out, q)
		fmt.Fprint(out, "> ")
		line, _ := rd.ReadString('\n') // an end of input reads as Enter
		return strings.TrimSpace(line)
	}
	yes := func(q string) bool {
		a := ask(q)
		return a != "" && (a[0] == 'y' || a[0] == 'Y')
	}
	if opt.Status == nil {
		opt.Status = func() bool {
			var b bytes.Buffer
			scheduleCmd([]string{"status"}, cfg, &b)
			return strings.Contains(b.String(), "is on")
		}
	}
	if opt.Enable == nil {
		opt.Enable = func(w io.Writer) int { return scheduleCmd([]string{"enable"}, cfg, w) }
	}

	fmt.Fprintln(out, setupWelcome)

	if a := ask(setupAskPlan); a != "" {
		if v, err := strconv.ParseFloat(a, 64); err == nil && v > 0 {
			cfg.PlanMbps = v
		} else {
			fmt.Fprintln(out, setupBadNumber)
		}
	}
	if a := ask(setupAskName); a != "" {
		cfg.Name = a
	}

	knownPath := filepath.Join(dir, "known-devices.txt")
	known, _ := store.LoadKnown(knownPath)
	ad, _ := src.Adapter(ctx)
	var list []core.Device
	if devs, err := src.Devices(ctx, ad.IP, ad.PrefixLength); err == nil {
		for _, d := range core.FlagUnknown(devs, known) {
			if d.IP != ad.IP && !d.Known {
				list = append(list, d)
			}
		}
	}
	if len(list) == 0 {
		fmt.Fprintln(out)
		fmt.Fprintln(out, setupNoDevices)
	} else {
		fmt.Fprintln(out)
		fmt.Fprintln(out, setupDevices)
		for i, d := range list {
			name := d.Name
			if name == "" {
				name = "(no name)"
			}
			fmt.Fprintf(out, " %d. %-20s %-15s %s\n", i+1, name, d.IP, d.Maker)
		}
		for _, tok := range strings.FieldsFunc(ask(setupAskDev), func(r rune) bool { return r == ' ' || r == ',' }) {
			if strings.EqualFold(tok, "all") {
				for _, d := range list {
					store.AddKnown(knownPath, d.MAC, d.Name)
				}
				break
			}
			n, err := strconv.Atoi(tok)
			if err != nil || n < 1 || n > len(list) {
				fmt.Fprintf(out, setupNotInList+"\n", tok)
				continue
			}
			store.AddKnown(knownPath, list[n-1].MAC, list[n-1].Name)
		}
	}

	if opt.Status() {
		fmt.Fprintln(out)
		fmt.Fprintln(out, setupWeeklyOn)
	} else if yes(setupAskWeekly) {
		opt.Enable(out)
	}
	if yes(setupAskFile) {
		var keep []string
		for _, k := range cfg.Sinks {
			if strings.TrimSpace(k) != string(core.SinkFile) {
				keep = append(keep, k)
			}
		}
		cfg.Sinks = append(keep, string(core.SinkFile))
	}

	if err := store.SaveConfig(cfgPath, cfg); err != nil {
		fmt.Fprintln(out, setupCantSave, err)
		return 2
	}
	if opt.Share == nil {
		kn, _ := store.LoadKnown(knownPath)
		opt.Share = func() int { return Share(ctx, src, ShareOptions{}, Options{PlanMbps: cfg.PlanMbps, Known: kn}, out) }
	}
	if yes(setupAskPhone) {
		return opt.Share()
	}
	fmt.Fprintln(out)
	fmt.Fprintln(out, setupDone)
	return 0
}
