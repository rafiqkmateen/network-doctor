package fix

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
)

// readConditions reads the situation at the press (ND-244), using the check's own read code. The
// adapter and its address are read first. With no usable address nothing can answer, so no ping is
// run and renew-address is decided on the address alone. Otherwise the router and the internet are
// pinged: a ping that ran and ended, with a loss line or with only "Network is unreachable" and an
// exit code, means that side does not answer; a ping that could not start at all (missing, not
// allowed) or was killed by the system means the situation was not read, and an error is returned,
// so reset-adapter and renew-address are refused with the plain note. It only reads, and it has no
// build tag so the tests run it with a simulated computer.
func readConditions(ctx context.Context, src collect.Source) (Conditions, error) {
	ad, err := src.Adapter(ctx)
	if err != nil {
		return Conditions{}, err
	}
	c := Conditions{
		AdapterUp:  ad.Up,
		HasAddress: ad.IP != "" && !strings.HasPrefix(ad.IP, "169.254.") && !ad.NoAddress,
	}
	if !c.AdapterUp || !c.HasAddress {
		return c, nil
	}
	if ad.Gateway != "" {
		answers, err := pingAnswers(ctx, src, ad.Gateway)
		if err != nil {
			return Conditions{}, fmt.Errorf("the router could not be checked: %w", err)
		}
		c.RouterAnswers = answers
	}
	answers, err := pingAnswers(ctx, src, "1.1.1.1")
	if err != nil {
		return Conditions{}, fmt.Errorf("the internet could not be checked: %w", err)
	}
	c.InternetAnswers = answers
	return c, nil
}

// pingAnswers pings once and says whether the host answered. An error means the ping could not run.
func pingAnswers(ctx context.Context, src collect.Source, host string) (bool, error) {
	p, err := src.Ping(ctx, host, 2)
	if err == nil {
		return p.LossPct < 100, nil
	}
	var ran interface{ ExitCode() int }
	if errors.As(err, &ran) && ran.ExitCode() >= 0 {
		return false, nil // it ran and ended: no answer
	}
	return false, err // not started, or killed by the system
}
