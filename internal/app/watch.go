package app

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/notify"
	"github.com/rafiqkmateen/network-doctor/internal/store"
)

// Alert is one message for the user.
type Alert struct {
	Kind    core.AlertKind `json:"kind"`
	Message string         `json:"message"`
	Time    time.Time      `json:"time"`
}

// Watcher watches for drops, slow stretches and new devices.
type Watcher struct {
	src     collect.Source
	alerter *core.Alerter
	opt     Options
	slowRun int
}

// NewWatcher makes a Watcher.
func NewWatcher(src collect.Source, a *core.Alerter, opt Options) *Watcher {
	return &Watcher{src: src, alerter: a, opt: opt}
}

// Tick checks the link, and on a full tick also the speed and the devices.
func (w *Watcher) Tick(ctx context.Context, now time.Time, full bool) []Alert {
	var out []Alert
	ad, _ := w.src.Adapter(ctx)
	linkDown := !ad.Up
	if !linkDown && ad.Gateway != "" {
		if p, err := w.src.Ping(ctx, ad.Gateway, 2); err == nil && p.LossPct >= 100 {
			linkDown = true
		}
	}
	if linkDown {
		if w.alerter.Offer(core.AlertDrop, "link", now) {
			out = append(out, Alert{core.AlertDrop, "The connection dropped.", now})
		}
		return out
	}
	w.alerter.Clear(core.AlertDrop, "link")
	if !full {
		return out
	}
	if down, _, err := w.src.Speed(collect.LightSpeed(ctx)); err == nil && w.opt.PlanMbps > 0 {
		if down < 0.5*w.opt.PlanMbps {
			w.slowRun++
		} else {
			w.slowRun = 0
			w.alerter.Clear(core.AlertSlow, "speed")
		}
		if w.slowRun >= 3 && w.alerter.Offer(core.AlertSlow, "speed", now) {
			out = append(out, Alert{core.AlertSlow, fmt.Sprintf("Download speed has stayed under half of your %.0f Mbps plan for three checks in a row.", w.opt.PlanMbps), now})
		}
	}
	// With the page's "Warn me about new devices" switch off, no new-device alert is raised (ND-265).
	warnOff := w.opt.NoNewDeviceWarnings
	if w.opt.NewDeviceWarningsOff != nil {
		warnOff = w.opt.NewDeviceWarningsOff() // asked again, so the switch counts at once
	}
	if ad.IP != "" && !warnOff {
		known := w.opt.Known
		if w.opt.LoadKnown != nil {
			known = w.opt.LoadKnown() // read again, so a device marked known since the start counts
		}
		states := w.opt.States
		if w.opt.LoadStates != nil {
			states = w.opt.LoadStates()
		}
		if devs, err := w.src.Devices(ctx, ad.IP, ad.PrefixLength); err == nil {
			for _, d := range core.FlagUnknown(devs, known) {
				if d.IP == ad.IP || (ad.Gateway != "" && d.IP == ad.Gateway) {
					continue // this computer, or the router
				}
				if states[core.NormalizeMAC(d.MAC)].State == store.Ignored {
					continue // the person chose to ignore it (ND-264)
				}
				if w.alerter.Offer(core.AlertNewDevice, d.MAC, now) {
					name := d.Name
					if name == "" {
						name = "no name"
					}
					out = append(out, Alert{core.AlertNewDevice, fmt.Sprintf("A device you have not marked as known is on the network: %s, %s, %s.", name, d.IP, d.MAC), now})
				}
			}
		}
	}
	return out
}

// The proof switch: with NETDOCTOR_TEST_ALERT set to 1, watch raises this one alert right after it
// starts, through the person's chosen places, so a test can prove the whole alert path.
const (
	testAlertSwitch  = "NETDOCTOR_TEST_ALERT"
	testAlertKind    = core.AlertKind("test")
	testAlertMessage = "This is a test alert from Network Doctor."
)

// testAlerts gives the one test alert when the switch is 1, and nothing otherwise.
func testAlerts(now time.Time) []Alert {
	if os.Getenv(testAlertSwitch) != "1" {
		return nil
	}
	return []Alert{{Kind: testAlertKind, Message: testAlertMessage, Time: now}}
}

// screenAlert shows an alert on the screen. Tests replace it so no pop-up appears.
var screenAlert = notify.Screen

// What the watcher's log says when an alert could not be shown on the screen (ND-288).
const (
	screenFailSaved  = "Alerts could not be shown on the screen. They are still saved in the alerts file."
	screenFailNoFile = "Alerts could not be shown on the screen."
)

// Emit sends an alert to the sinks the user chose. Only the console is on by default.
func Emit(a Alert, sinks []core.Sink, dir string, console io.Writer) []string {
	var problems []string
	for _, s := range sinks {
		switch s {
		case core.SinkConsole:
			fmt.Fprintf(console, "%s  ALERT  %s\n", a.Time.Format("2006-01-02 1504"), a.Message)
		case core.SinkFile:
			if err := os.MkdirAll(dir, 0o755); err == nil {
				if fh, err := os.OpenFile(filepath.Join(dir, "alerts.log"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644); err == nil {
					fmt.Fprintf(fh, "%s\t%s\t%s\n", a.Time.Format(time.RFC3339), a.Kind, oneLine(a.Message))
					fh.Close()
				}
			}
		case core.SinkScreen:
			if err := screenAlert("Network Doctor", a.Message); err != nil {
				// One fixed plain sentence, so the watcher can say it once (ND-288).
				p := screenFailNoFile
				for _, f := range sinks {
					if f == core.SinkFile {
						p = screenFailSaved
					}
				}
				problems = append(problems, p)
			}
		default:
			problems = append(problems, string(s)+": this place for alerts is not built yet")
		}
	}
	return problems
}

// oneLine keeps a message on one line with no tabs, so each alert is one line in the file.
func oneLine(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// alertSinks says where alerts go. A choice the person saved always wins. With none saved, the
// alerts part of the installer, when it was chosen, sends them to the console, the file and
// the screen; otherwise they go to the console.
func alertSinks(cfg store.Config) []core.Sink {
	if !cfg.SinksSaved {
		if c, ok := LoadChoices(programDir()); ok && c.Proactive {
			return []core.Sink{core.SinkConsole, core.SinkFile, core.SinkScreen}
		}
	}
	sinks := parseSinks(cfg.Sinks)
	if len(sinks) == 0 {
		sinks = core.DefaultAlertConfig().Sinks
	}
	return sinks
}

func parseSinks(list []string) []core.Sink {
	var out []core.Sink
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, core.Sink(s))
		}
	}
	return out
}
