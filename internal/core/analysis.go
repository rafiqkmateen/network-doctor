package core

import (
	"fmt"
	"sort"
	"strings"
	"time"
)

// Event is one connection change taken from the system log.
type Event struct {
	Time time.Time `json:"time"`
	Up   bool      `json:"up"`
}

// Drop is one outage.
type Drop struct {
	Start  time.Time     `json:"start"`
	Length time.Duration `json:"length"`
}

// MinDrop is the shortest outage that counts as a drop. Shorter ones are
// counted as brief blips.
const MinDrop = 5 * time.Second

// DropReport summarizes the last seven days.
type DropReport struct {
	Count      int    `json:"count"`
	BriefBlips int    `json:"brief_blips"`
	Drops      []Drop `json:"drops"`
	Pattern    string `json:"pattern"`
}

// AnalyzeDrops reads seven days of events ending at now.
func AnalyzeDrops(events []Event, now time.Time) DropReport {
	cutoff := now.AddDate(0, 0, -7)
	ev := append([]Event(nil), events...)
	sort.Slice(ev, func(i, j int) bool { return ev[i].Time.Before(ev[j].Time) })
	var rep DropReport
	var downAt time.Time
	down := false
	for _, e := range ev {
		if e.Time.Before(cutoff) || e.Time.After(now) {
			continue
		}
		switch {
		case !e.Up && !down:
			down, downAt = true, e.Time
		case e.Up && down:
			rep.Drops = append(rep.Drops, Drop{downAt, e.Time.Sub(downAt)})
			down = false
		}
	}
	if down {
		rep.Drops = append(rep.Drops, Drop{downAt, now.Sub(downAt)})
	}
	real := rep.Drops[:0:0]
	for _, d := range rep.Drops {
		if d.Length < MinDrop {
			rep.BriefBlips++
			continue
		}
		real = append(real, d)
	}
	rep.Drops = real
	rep.Count = len(rep.Drops)
	hours := map[int]int{}
	for _, d := range rep.Drops {
		hours[d.Start.Hour()]++
	}
	best, bestN := 0, 0
	for h, n := range hours {
		if n > bestN || (n == bestN && h < best) {
			best, bestN = h, n
		}
	}
	if bestN >= 3 {
		rep.Pattern = fmt.Sprintf("Drops often start around %02d00. Check what runs at that time, such as updates or power saving.", best)
	}
	return rep
}

// Week is one weekly speed record.
type Week struct {
	Time     time.Time `json:"time"`
	DownMbps float64   `json:"down_mbps"`
	PlanMbps float64   `json:"plan_mbps"`
}

// WeeklyDue says whether the weekly check should run now.
func WeeklyDue(now, last time.Time, wd time.Weekday, hour int) bool {
	slot := time.Date(now.Year(), now.Month(), now.Day(), hour, 0, 0, 0, now.Location())
	for slot.Weekday() != wd {
		slot = slot.AddDate(0, 0, -1)
	}
	if slot.After(now) {
		slot = slot.AddDate(0, 0, -7)
	}
	return last.Before(slot)
}

// ShouldDraftComplaint is true when the last three weeks all ran under half the plan.
func ShouldDraftComplaint(weeks []Week) bool {
	if len(weeks) < 3 {
		return false
	}
	for _, w := range weeks[len(weeks)-3:] {
		if w.PlanMbps <= 0 || w.DownMbps >= 0.5*w.PlanMbps {
			return false
		}
	}
	return true
}

// ComplaintDraft builds an email for the user to review. It is never sent.
func ComplaintDraft(weeks []Week, name string) string {
	var b strings.Builder
	b.WriteString("DRAFT - not sent. Read it, change it and send it yourself.\n\n")
	b.WriteString("Subject: Internet speed far below my plan\n\n")
	b.WriteString("To whom it may concern,\n\n")
	if len(weeks) > 0 {
		b.WriteString(fmt.Sprintf("I pay for %.0f Mbps. For three weeks in a row my download speed stayed below half of that:\n\n", weeks[len(weeks)-1].PlanMbps))
	}
	start := 0
	if len(weeks) > 3 {
		start = len(weeks) - 3
	}
	for _, w := range weeks[start:] {
		b.WriteString(fmt.Sprintf("- %s: %.0f Mbps down\n", w.Time.Format("2006-01-02 1504"), w.DownMbps))
	}
	b.WriteString("\nThe tests ran from one computer on a schedule. Please look into the line and tell me what you find.\n\n")
	b.WriteString("Thank you,\n" + name + "\n")
	return b.String()
}

// AlertKind names a type of alert.
type AlertKind string

// Alert kinds.
const (
	AlertDrop      AlertKind = "drop"
	AlertSlow      AlertKind = "slow"
	AlertNewDevice AlertKind = "new_device"
)

// Sink is a place an alert may go.
type Sink string

// Alert sinks. Only the console is on by default.
const (
	SinkConsole  Sink = "console"
	SinkFile     Sink = "file"
	SinkScreen   Sink = "screen"
	SinkKhadijah Sink = "khadijah"
)

// AlertConfig says which alerts are on, when to stay quiet and where they go.
type AlertConfig struct {
	Enabled       map[AlertKind]bool
	QuietFromHour int
	QuietToHour   int
	Sinks         []Sink
}

// DefaultAlertConfig has every alert on, no quiet hours and the console only.
func DefaultAlertConfig() AlertConfig {
	return AlertConfig{
		Enabled: map[AlertKind]bool{AlertDrop: true, AlertSlow: true, AlertNewDevice: true},
		Sinks:   []Sink{SinkConsole},
	}
}

// Alerter decides whether an alert may be shown.
type Alerter struct {
	cfg  AlertConfig
	sent map[string]bool
}

// NewAlerter makes an Alerter.
func NewAlerter(cfg AlertConfig) *Alerter { return &Alerter{cfg: cfg, sent: map[string]bool{}} }

func (a *Alerter) quiet(now time.Time) bool {
	from, to, h := a.cfg.QuietFromHour, a.cfg.QuietToHour, now.Hour()
	if from == to {
		return false
	}
	if from < to {
		return h >= from && h < to
	}
	return h >= from || h < to
}

// Offer returns true when the alert should be shown. One alert per outage.
func (a *Alerter) Offer(kind AlertKind, key string, now time.Time) bool {
	if !a.cfg.Enabled[kind] || a.quiet(now) {
		return false
	}
	id := string(kind) + "|" + key
	if a.sent[id] {
		return false
	}
	a.sent[id] = true
	return true
}

// Clear ends an outage so the next one can alert again.
func (a *Alerter) Clear(kind AlertKind, key string) { delete(a.sent, string(kind)+"|"+key) }

// Device is one machine seen on the network.
type Device struct {
	Name  string `json:"name"`
	Maker string `json:"maker"`
	IP    string `json:"ip"`
	MAC   string `json:"mac"`
	Known bool   `json:"known"`
}

// NormalizeMAC puts a hardware address in one form.
func NormalizeMAC(s string) string {
	return strings.ToUpper(strings.ReplaceAll(strings.TrimSpace(s), "-", ":"))
}

// FlagUnknown returns the devices whose address is not on the known list.
func FlagUnknown(devs []Device, known map[string]bool) []Device {
	var out []Device
	for _, d := range devs {
		if !known[NormalizeMAC(d.MAC)] {
			out = append(out, d)
		}
	}
	return out
}

// Proc is one program and how many connections it has open.
// Windows gives byte counts per program only to administrators, so the
// tool ranks by open connections and reports the line total separately.
type Proc struct {
	Name  string `json:"name"`
	Conns int    `json:"connections"`
}

// RankProcs sorts programs by open connections, highest first.
func RankProcs(p []Proc) []Proc {
	out := append([]Proc(nil), p...)
	sort.SliceStable(out, func(i, j int) bool { return out[i].Conns > out[j].Conns })
	return out
}

// BandwidthAdvice gives the line total and, when the line is busy, the
// programs to look at first. It never stops anything.
func BandwidthAdvice(rxMBps, txMBps float64, ranked []Proc) []string {
	if rxMBps+txMBps < 0.5 {
		return []string{"No program is using much of the line."}
	}
	out := []string{fmt.Sprintf("The line is busy: about %.1f MB a second coming in and %.1f going out.", rxMBps, txMBps)}
	var names []string
	for i, p := range ranked {
		if i == 3 {
			break
		}
		names = append(names, fmt.Sprintf("%s (%d)", p.Name, p.Conns))
	}
	if len(names) > 0 {
		out = append(out, "Programs with the most open connections: "+strings.Join(names, ", ")+". Look at these first.")
		out = append(out, "You could pause the ones you do not need for now.")
	}
	return out
}

// Settings is what the settings review reads from a machine.
type Settings struct {
	DNS           []string `json:"dns"`
	PowerKnown    bool     `json:"power_known"`
	PowerSaving   bool     `json:"power_saving"`
	VPNActive     bool     `json:"vpn_active"`
	ProxyOn       bool     `json:"proxy_on"`
	DriverVersion string   `json:"driver_version"`
	DriverAgeDays int      `json:"driver_age_days"`
	ConfigErrors  []string `json:"config_errors"`
}

// Advice is one finding from the settings review.
type Advice struct {
	Area    string `json:"area"`
	Finding string `json:"finding"`
	Effect  string `json:"effect"`
}

// ReviewSettings always returns six areas, in a fixed order.
func ReviewSettings(s Settings) []Advice {
	a := make([]Advice, 0, 6)
	if len(s.DNS) == 0 {
		a = append(a, Advice{"DNS", "No DNS server was found.", "Web names will not load until one is set."})
	} else {
		a = append(a, Advice{"DNS", "DNS servers in use: " + strings.Join(s.DNS, ", ") + ".", "Slow name lookups can make pages feel slow on a fast line."})
	}
	if !s.PowerKnown {
		a = append(a, Advice{"Power saving", "The tool could not read the adapter power-saving setting.", "Look for it in Device Manager, on the power tab of the adapter."})
	} else if s.PowerSaving {
		a = append(a, Advice{"Power saving", "The adapter may turn itself off to save power.", "Turning this off often stops random drops. It uses a little more power."})
	} else {
		a = append(a, Advice{"Power saving", "Adapter power saving is off.", "Nothing to change."})
	}
	if s.VPNActive {
		a = append(a, Advice{"VPN", "A VPN is active.", "A VPN can lower speed and add delay. Turn it off to compare."})
	} else {
		a = append(a, Advice{"VPN", "No VPN is active.", "Nothing to change."})
	}
	if s.ProxyOn {
		a = append(a, Advice{"Proxy", "A proxy is turned on.", "Traffic goes through another server first. That can slow pages down."})
	} else {
		a = append(a, Advice{"Proxy", "No proxy is turned on.", "Nothing to change."})
	}
	if s.DriverAgeDays > 1095 {
		a = append(a, Advice{"Driver", "The adapter driver is over three years old. Version " + s.DriverVersion + ".", "A newer driver can fix drops. Check the maker's site first."})
	} else {
		a = append(a, Advice{"Driver", "The adapter driver looks recent. Version " + s.DriverVersion + ".", "Nothing to change."})
	}
	if len(s.ConfigErrors) > 0 {
		a = append(a, Advice{"Errors", "Windows reports a problem with the adapter: " + strings.Join(s.ConfigErrors, ", ") + ".", "A problem here can cause drops. Fixing it may need a driver or a restart."})
	} else {
		a = append(a, Advice{"Errors", "No adapter errors were found.", "Nothing to change."})
	}
	return a
}
