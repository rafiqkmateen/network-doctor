package fix

import (
	"context"
	"errors"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
)

// pingSource is a simulated computer for the reachability read: its adapter, and a ping that answers,
// gets no reply, or cannot run at all.
type pingSource struct {
	collect.Source // only Adapter and Ping are used
	ad             collect.AdapterInfo
	router, net    error // the ping command itself failed
	routerLoss     float64
	netLoss        float64
}

func (s pingSource) Adapter(context.Context) (collect.AdapterInfo, error) { return s.ad, nil }
func (s pingSource) Ping(_ context.Context, host string, _ int) (collect.PingResult, error) {
	if host == s.ad.Gateway {
		return collect.PingResult{LossPct: s.routerLoss}, s.router
	}
	return collect.PingResult{LossPct: s.netLoss}, s.net
}

// ND-244: a clean "no reply" means the router or the internet does not answer; a ping that cannot
// run means the situation was not read, so reset-adapter and renew-address are refused.
func TestReachabilityNeedsAPingThatRan(t *testing.T) {
	up := collect.AdapterInfo{Up: true, IP: "192.168.1.20", Gateway: "192.168.1.1"}
	c, err := readConditions(context.Background(), pingSource{ad: up, routerLoss: 100, netLoss: 100})
	if err != nil || c.RouterAnswers || c.InternetAnswers || !c.AdapterUp {
		t.Errorf("clean no reply: %+v %v", c, err)
	}
	c, err = readConditions(context.Background(), pingSource{ad: up, routerLoss: 0, netLoss: 50})
	if err != nil || !c.RouterAnswers || !c.InternetAnswers {
		t.Errorf("answers: %+v %v", c, err)
	}
	for name, s := range map[string]pingSource{
		"router ping missing":  {ad: up, router: errors.New(`exec: "ping": executable file not found`), netLoss: 100},
		"internet ping denied": {ad: up, routerLoss: 100, net: errors.New("ping: socket: Operation not permitted")},
	} {
		if _, err := readConditions(context.Background(), s); err == nil {
			t.Errorf("%s: a ping that could not run must mean not read", name)
		}
		sys := newLinux().sys()
		sys.Conds = func() (Conditions, error) { return readConditions(context.Background(), s) }
		for _, fixName := range []string{resetAdapter, renewAddress} {
			var re *RefusedError
			if _, err := NewManager(sys, t.TempDir(), Options{}).Apply(fixName, true); !errors.As(err, &re) ||
				re.Reason != "I could not read the connection right now, so nothing was changed. Try again in a moment." {
				t.Errorf("%s %s: must be refused with the plain note: %v", name, fixName, err)
			}
		}
	}
	// No adapter up: nothing to ping, and that is read, not an error.
	if c, err := readConditions(context.Background(), pingSource{ad: collect.AdapterInfo{}}); err != nil || c.AdapterUp {
		t.Errorf("adapter off: %+v %v", c, err)
	}
}

// pingExit is a ping that ran and ended with an exit code, as exec.ExitError does: "Network is
// unreachable" with no loss line, or a code of -1 when the system killed it.
type pingExit int

func (e pingExit) Error() string { return "Network is unreachable" }
func (e pingExit) ExitCode() int { return int(e) }

// ND-244: a ping that ran and ended, even with only "Network is unreachable" and no loss line,
// means "does not answer"; only a ping that could not start, or was killed, means not read. With
// no usable address no ping is run at all, so renew-address is decided on the address alone.
func TestPingThatRanCountsAsNoAnswer(t *testing.T) {
	up := collect.AdapterInfo{Up: true, IP: "192.168.1.20", Gateway: "192.168.1.1"}
	for _, code := range []int{1, 2} {
		c, err := readConditions(context.Background(), pingSource{ad: up, router: pingExit(code), net: pingExit(code)})
		if err != nil || c.RouterAnswers || c.InternetAnswers {
			t.Errorf("exit %d: a ping that ran is a clean no answer: %+v %v", code, c, err)
		}
	}
	if _, err := readConditions(context.Background(), pingSource{ad: up, router: pingExit(-1), netLoss: 100}); err == nil {
		t.Error("a ping the system killed must mean not read")
	}
	if _, err := readConditions(context.Background(), pingSource{ad: up, router: errors.New(`exec: "ping": executable file not found in $PATH`)}); err == nil {
		t.Error("a missing ping must mean not read")
	}
	// reset-adapter on a dead line read through "unreachable" pings is made, not refused.
	sys := newLinux().sys()
	sys.Conds = func() (Conditions, error) {
		return readConditions(context.Background(), pingSource{ad: up, router: pingExit(2), net: pingExit(2)})
	}
	if _, err := NewManager(sys, t.TempDir(), Options{}).Apply(resetAdapter, true); err != nil {
		t.Errorf("reset-adapter on an unreachable line: %v", err)
	}
	// No usable address: renew-address goes ahead even with ping missing, since no ping is run.
	none := collect.AdapterInfo{Up: true, IP: "169.254.3.4"}
	missing := errors.New(`exec: "ping": executable file not found in $PATH`)
	c, err := readConditions(context.Background(), pingSource{ad: none, router: missing, net: missing})
	if err != nil || c.HasAddress || c.RouterAnswers || c.InternetAnswers {
		t.Errorf("no address: %+v %v", c, err)
	}
	p := newLinux()
	p.route = false
	rs := p.sys()
	rs.Conds = func() (Conditions, error) {
		return readConditions(context.Background(), pingSource{ad: none, router: missing, net: missing})
	}
	if _, err := NewManager(rs, t.TempDir(), Options{}).Apply(renewAddress, true); err != nil {
		t.Errorf("renew-address with no address and no ping: %v", err)
	}
}
