package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os/exec"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

var (
	lossRe = regexp.MustCompile(`\((\d+)%\s*loss\)`)
	avgRe  = regexp.MustCompile(`Average\s*=\s*(\d+)\s*ms`)
	unixRe = regexp.MustCompile(`(\d+(?:\.\d+)?)% packet loss|min/avg/max[^=]*=\s*[\d.]+/([\d.]+)/`)
)

// ParsePing reads the text a ping command prints.
func ParsePing(out string) PingResult {
	var r PingResult
	if m := lossRe.FindStringSubmatch(out); m != nil {
		r.LossPct, _ = strconv.ParseFloat(m[1], 64)
	}
	if m := avgRe.FindStringSubmatch(out); m != nil {
		r.AvgMs, _ = strconv.ParseFloat(m[1], 64)
	}
	for _, m := range unixRe.FindAllStringSubmatch(out, -1) {
		if m[1] != "" {
			r.LossPct, _ = strconv.ParseFloat(m[1], 64)
		}
		if m[2] != "" {
			r.AvgMs, _ = strconv.ParseFloat(m[2], 64)
		}
	}
	return r
}

func pingCommand(ctx context.Context, host string, count int) *exec.Cmd {
	n := strconv.Itoa(count)
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "ping", "-n", n, "-w", "1000", host)
	}
	if runtime.GOOS == "darwin" {
		return exec.CommandContext(ctx, "ping", "-c", n, "-W", "1000", host) // Mac counts milliseconds
	}
	return exec.CommandContext(ctx, "ping", "-c", n, "-W", "1", host)
}

func doPing(ctx context.Context, host string, count int) (PingResult, error) {
	if net.ParseIP(host) == nil {
		return PingResult{}, fmt.Errorf("not an address: %q", host)
	}
	cmd := pingCommand(ctx, host, count)
	hide(cmd)
	out, err := cmd.CombinedOutput()
	res := ParsePing(string(out))
	if err != nil && res.AvgMs == 0 && res.LossPct == 0 {
		return res, err
	}
	return res, nil
}

func doDNSWorks(ctx context.Context) bool {
	r := net.Resolver{}
	c, cancel := context.WithTimeout(ctx, 4*time.Second)
	defer cancel()
	_, err := r.LookupHost(c, "example.com")
	return err == nil
}

var kvRe = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z ]*?)\s*:\s*(.+?)\s*$`)

// Shapes of the values in `netsh wlan show interfaces`, used when the labels are not in English
// (ND-242). Each must match the whole value, so a network name holding "5 GHz" or "%" is not taken.
var (
	wlanPctRe     = regexp.MustCompile(`^(?:(\d{1,3})\s*%|%\s*(\d{1,3}))$`)
	wlanBandRe    = regexp.MustCompile(`^(\d+(?:[.,]\d+)?)\s*GHz$`)
	wlanChannelRe = regexp.MustCompile(`^\d{1,3}$`)
	wlanRateKeyRe = regexp.MustCompile(`(?i)mbps|bit/s|mb/s`)
)

// wlanPct reads "82%" or "%82" (as some languages write it).
func wlanPct(v string) (int, bool) {
	m := wlanPctRe.FindStringSubmatch(v)
	if m == nil {
		return 0, false
	}
	n, err := strconv.Atoi(m[1] + m[2])
	return n, err == nil && n <= 100
}

// wlanBand reads "5 GHz" or "2,4 GHz" and writes it the English way.
func wlanBand(v string) (string, bool) {
	m := wlanBandRe.FindStringSubmatch(v)
	if m == nil {
		return "", false
	}
	return strings.ReplaceAll(m[1], ",", ".") + " GHz", true
}

// wlanChannel reads a plain channel number.
func wlanChannel(v string) (int, bool) {
	if !wlanChannelRe.MatchString(v) {
		return 0, false
	}
	n, _ := strconv.Atoi(v)
	return n, n >= 1 && n <= 233
}

// ParseWlanInterfaces reads `netsh wlan show interfaces`. The English labels come first; when
// they are missing (Windows in another language) the values are found by their units and shape:
// a percent for the signal, GHz for the band and a plain number for the channel. The network's
// own name (SSID, and the profile that repeats it) and the rate lines are never used.
func ParseWlanInterfaces(out string) WiFiInfo {
	var info WiFiInfo
	type kv struct{ key, val string }
	var rows []kv
	ssid := ""
	for _, l := range lines(out) {
		i := strings.Index(l, ":")
		if i <= 0 {
			continue
		}
		k, v := strings.TrimSpace(l[:i]), strings.TrimSpace(l[i+1:])
		if k == "" || v == "" {
			continue
		}
		if k == "SSID" && ssid == "" {
			ssid = v
		}
		rows = append(rows, kv{k, v})
	}
	sigSet, chanSet := false, false
	for _, r := range rows {
		switch r.key {
		case "Signal":
			if n, ok := wlanPct(r.val); ok {
				info.SignalPct, sigSet = n, true
			}
		case "Channel":
			if n, ok := wlanChannel(r.val); ok {
				info.Channel, chanSet = n, true
			}
		case "Band":
			if b, ok := wlanBand(r.val); ok {
				info.Band = b
			}
		}
	}
	for _, r := range rows {
		if r.key == "SSID" || r.key == "BSSID" || (ssid != "" && r.val == ssid) || wlanRateKeyRe.MatchString(r.key) {
			continue
		}
		if !sigSet {
			if n, ok := wlanPct(r.val); ok {
				info.SignalPct, sigSet = n, true
				continue
			}
		}
		if info.Band == "" {
			if b, ok := wlanBand(r.val); ok {
				info.Band = b
				continue
			}
		}
		if !chanSet {
			if n, ok := wlanChannel(r.val); ok {
				info.Channel, chanSet = n, true
			}
		}
	}
	if info.Band == "" && info.Channel > 0 {
		info.Band = "2.4 GHz"
		if info.Channel > 14 {
			info.Band = "5 GHz"
		}
	}
	return info
}

// wlanEnglishRefusal says the output names location permission, as the English refusal does.
func wlanEnglishRefusal(out string) bool {
	return strings.Contains(strings.ToLower(out), "location permission")
}

// WlanHidden says whether `netsh wlan` refused the Wi-Fi details because
// Windows Location services is off. The refusal both fails (exit code 1) and
// names location permission; a report that was read fine never counts, even
// when a network's name holds those words. On a Windows in another language the
// words differ, so a failed call counts only when the Location consent value,
// read from the registry, says denied (ND-242). Any other failure is handled as before.
func WlanHidden(out string, err error) bool {
	if err == nil || strings.TrimSpace(out) == "" {
		// A refusal always prints something; a silent failure is some other trouble.
		return false
	}
	if wlanEnglishRefusal(out) {
		return true
	}
	denied, known := LocationConsentDenied()
	return known && denied
}

// ParseWlanChannels reads `netsh wlan show networks mode=bssid` and returns the
// channels of the other networks. One entry on the user's own channel is the
// user's own network and is left out. It works on a Windows set to another language
// (ND-246): a block starts at a BSSID line, and its channel is the first whole number
// after the radio type line, the one whose value starts with 802.11, so no label
// has to be in English. The English word Channel still counts as before.
func ParseWlanChannels(out string, own int) []int {
	var chans []int
	removedSelf := false
	take := func(c int) {
		if c == own && !removedSelf {
			removedSelf = true
			return
		}
		chans = append(chans, c)
	}
	inBlock, radioSeen, got := false, false, false
	for _, l := range lines(out) {
		label, value, ok := splitFirstColon(l)
		if !ok {
			continue
		}
		switch {
		case bssidLabelRe.MatchString(label):
			inBlock, radioSeen, got = true, false, false
		case !inBlock || got:
		case strings.HasPrefix(value, "802.11"):
			radioSeen = true
		case radioSeen && wholeNumberRe.MatchString(value):
			c, _ := strconv.Atoi(value)
			take(c)
			got = true
		case label == "Channel" && wholeNumberRe.MatchString(value):
			c, _ := strconv.Atoi(value)
			take(c)
			got = true
		}
	}
	return chans
}

var (
	bssidLabelRe  = regexp.MustCompile(`^BSSID\s*\d*$`)
	wholeNumberRe = regexp.MustCompile(`^\d{1,3}$`)
)

// splitFirstColon splits a report line at its first colon into a trimmed label and value.
func splitFirstColon(l string) (label, value string, ok bool) {
	i := strings.Index(l, ":")
	if i < 0 {
		return "", "", false
	}
	return strings.TrimSpace(l[:i]), strings.TrimSpace(l[i+1:]), true
}

// PowerState combines the two ways Windows reports adapter power saving.
// The first is the power-management setting; the second is the adapter's own
// energy-saving options. If neither can be read the answer is "unknown".
func PowerState(pm, hint string) (known, saving bool) {
	switch {
	case strings.EqualFold(pm, "Enabled") || strings.EqualFold(hint, "Enabled"):
		return true, true
	case strings.EqualFold(pm, "Disabled"):
		return true, false
	case strings.EqualFold(hint, "Disabled"):
		return true, false
	}
	return false, false
}

// ParseThroughput reads the sample the throughput script prints.
func ParseThroughput(b []byte, linkMbps int) (Throughput, error) {
	var raw struct {
		Rx, Tx float64
		Procs  json.RawMessage
	}
	var t Throughput
	if err := json.Unmarshal(b, &raw); err != nil {
		return t, err
	}
	var procs []struct {
		Name  string
		Conns int
	}
	if err := looseList(raw.Procs, &procs); err != nil {
		return t, err
	}
	t.RxMBps, t.TxMBps = raw.Rx/1e6, raw.Tx/1e6
	if linkMbps > 0 {
		t.BusyPct = int((raw.Rx + raw.Tx) * 8 / (float64(linkMbps) * 1e6) * 100)
	}
	agg := map[string]int{}
	for _, p := range procs {
		agg[p.Name] += p.Conns
	}
	for n, c := range agg {
		t.Procs = append(t.Procs, core.Proc{Name: n, Conns: c})
	}
	return t, nil
}

// stringList reads a JSON value that PowerShell wrote as a string or a list.
type stringList []string

func (s *stringList) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '[' {
		var l []string
		if err := json.Unmarshal(b, &l); err != nil {
			return err
		}
		*s = l
		return nil
	}
	var one string
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*s = []string{one}
	return nil
}

// looseList decodes either one object or a list of objects.
func looseList(b []byte, out any) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 {
		return nil
	}
	if b[0] != '[' {
		b = append(append([]byte{'['}, b...), ']')
	}
	return json.Unmarshal(b, out)
}

func trimOut(b []byte) []byte { return bytes.TrimSpace(bytes.TrimPrefix(b, []byte("\xef\xbb\xbf"))) }

func lines(s string) []string { return strings.Split(strings.ReplaceAll(s, "\r", ""), "\n") }
