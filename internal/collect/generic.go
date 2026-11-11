package collect

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
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

// doSpeed measures download and upload against a public test server.
// This is one of the two things that go online; the other is the internet ping.
func doSpeed(ctx context.Context) (down, up float64, err error) {
	client := &http.Client{Timeout: 25 * time.Second}
	start := time.Now()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, "https://speed.cloudflare.com/__down?bytes=25000000", nil)
	resp, err := client.Do(req)
	if err != nil {
		return 0, 0, err
	}
	n, _ := io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if secs := time.Since(start).Seconds(); secs > 0 {
		down = float64(n) * 8 / secs / 1e6
	}
	body := bytes.NewReader(make([]byte, 8_000_000))
	start = time.Now()
	req, _ = http.NewRequestWithContext(ctx, http.MethodPost, "https://speed.cloudflare.com/__up", body)
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err = client.Do(req)
	if err != nil {
		return down, 0, err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	if secs := time.Since(start).Seconds(); secs > 0 {
		up = 8_000_000 * 8 / secs / 1e6
	}
	return down, up, nil
}

var kvRe = regexp.MustCompile(`^\s*([A-Za-z][A-Za-z ]*?)\s*:\s*(.+?)\s*$`)

// ParseWlanInterfaces reads `netsh wlan show interfaces`.
func ParseWlanInterfaces(out string) WiFiInfo {
	var info WiFiInfo
	for _, l := range lines(out) {
		m := kvRe.FindStringSubmatch(l)
		if m == nil {
			continue
		}
		switch m[1] {
		case "Signal":
			info.SignalPct, _ = strconv.Atoi(strings.TrimSuffix(m[2], "%"))
		case "Channel":
			info.Channel, _ = strconv.Atoi(m[2])
		case "Band":
			info.Band = m[2]
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

// ParseWlanChannels reads `netsh wlan show networks mode=bssid` and returns the
// channels of the other networks. One entry on the user's own channel is the
// user's own network and is left out.
func ParseWlanChannels(out string, own int) []int {
	var chans []int
	removedSelf := false
	for _, l := range lines(out) {
		m := kvRe.FindStringSubmatch(l)
		if m == nil || m[1] != "Channel" {
			continue
		}
		c, _ := strconv.Atoi(m[2])
		if c == own && !removedSelf {
			removedSelf = true
			continue
		}
		chans = append(chans, c)
	}
	return chans
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
