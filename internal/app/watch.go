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
	if down, _, err := w.src.Speed(ctx); err == nil && w.opt.PlanMbps > 0 {
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
	if ad.IP != "" {
		if devs, err := w.src.Devices(ctx, ad.IP, ad.PrefixLength); err == nil {
			for _, d := range core.FlagUnknown(devs, w.opt.Known) {
				if d.IP == ad.IP {
					continue // this computer
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
			if err := notify.Screen("Network Doctor", a.Message); err != nil {
				problems = append(problems, "screen: "+err.Error())
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

func parseSinks(list []string) []core.Sink {
	var out []core.Sink
	for _, s := range list {
		if s = strings.TrimSpace(s); s != "" {
			out = append(out, core.Sink(s))
		}
	}
	return out
}
