package app

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// probeSource is the part of the newer reads this file uses: the sign-in request and the region.
// Only these are asked for, so a source needs nothing more for the sign-in and clock notes.
type probeSource interface {
	Probe(ctx context.Context) (collect.ProbeResult, error)
	Region(ctx context.Context) string
}

// clockLimit is how far the computer's clock may be from the web server's before it is named (ND-232).
const clockLimit = 5 * time.Minute

// The words for the two notes live here, in one place.
var (
	signInNote = Note{
		Headline: "This Wi-Fi needs you to sign in through a web browser.",
		Why:      "A test page asked for by this computer came back as a different page. Hotels, cafes and airports do this until you sign in or agree to their terms.",
	}
	clockWhy = "Websites and sign-ins check the time. When the clock is wrong, they can fail even though the network is fine. Network Doctor does not change the clock."
)

// fillReadingExtras sets the sign-in page and clock notes (ND-231, ND-232) and anything else
// the readings add before the diagnosis.
func fillReadingExtras(ctx context.Context, rep *Report, rd *core.Reading, src collect.Source, opt Options) {
	ps, ok := src.(probeSource)
	if !ok {
		return
	}
	res, err := ps.Probe(ctx)
	if err != nil || res.Status <= 0 {
		// No answer: nothing to say about a sign-in page or the clock.
		return
	}
	if !res.Expected {
		n := signInNote
		rep.SignIn = &n
		rd.SignIn = true
	}
	if !res.Date.IsZero() {
		if words, off := clockWords(time.Now(), res.Date); off {
			rep.Clock = &Note{Headline: words, Why: clockWhy}
		}
	}
}

// clockWords compares this computer's time with the server's Date header. It says nothing
// (false) when they are 5 minutes apart or less.
func clockWords(now, server time.Time) (string, bool) {
	d := now.Sub(server)
	if d < 0 {
		d = -d
	}
	if d <= clockLimit {
		return "", false
	}
	min := int(math.Round(d.Minutes()))
	way := "fast"
	if now.Before(server) {
		way = "slow"
	}
	if min >= 120 {
		return fmt.Sprintf("This computer's clock is wrong. It is about %d hours %s, or %d minutes.", int(math.Round(d.Hours())), way, min), true
	}
	return fmt.Sprintf("This computer's clock is wrong by about %d minutes. It is %s.", min, way), true
}
