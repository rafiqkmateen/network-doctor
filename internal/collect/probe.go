package collect

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

// probeTarget is the address a system itself uses to test for a sign-in page, and how its
// expected answer looks (ND-231). No other address is ever asked.
type probeTarget struct {
	URL      string
	Expected func(status int, body string) bool
}

// The three system test addresses.
var (
	probeWindows = probeTarget{"http://www.msftconnecttest.com/connecttest.txt", func(st int, body string) bool {
		return st == http.StatusOK && strings.TrimSpace(body) == "Microsoft Connect Test"
	}}
	probeMac = probeTarget{"http://captive.apple.com/hotspot-detect.html", func(st int, body string) bool {
		return st == http.StatusOK && strings.Contains(body, "Success")
	}}
	probeLinux = probeTarget{"http://connectivity-check.ubuntu.com/", func(st int, body string) bool {
		return st == http.StatusNoContent || (st == http.StatusOK && strings.Contains(body, "NetworkManager is online"))
	}}
)

const (
	probeTimeout  = 4 * time.Second
	probeBodyMax  = 4 << 10
	probeAgent    = "Network Doctor"
	errNoProbeMsg = "this system has no sign-in test address"
)

// doProbe sends the one plain request: no cookies, no redirects followed, a short wait, and
// nothing about the person. The answer's status, Location, the start of the body and the Date
// header are kept.
func doProbe(ctx context.Context, t probeTarget) (ProbeResult, error) {
	if t.URL == "" {
		return ProbeResult{}, errors.New(errNoProbeMsg)
	}
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, t.URL, nil)
	if err != nil {
		return ProbeResult{}, err
	}
	req.Header.Set("User-Agent", probeAgent)
	req.Header.Set("Cache-Control", "no-cache")
	client := &http.Client{
		Timeout:       probeTimeout,
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
		Transport:     &http.Transport{Proxy: http.ProxyFromEnvironment, DisableKeepAlives: true},
	}
	resp, err := client.Do(req)
	if err != nil {
		return ProbeResult{}, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, probeBodyMax))
	r := ProbeResult{Status: resp.StatusCode, Location: resp.Header.Get("Location"), Body: string(b)}
	if d := resp.Header.Get("Date"); d != "" {
		if tm, e := http.ParseTime(d); e == nil {
			r.Date = tm
		}
	}
	r.Expected = t.Expected(r.Status, r.Body)
	return r, nil
}
