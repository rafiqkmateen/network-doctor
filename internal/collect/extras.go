package collect

import (
	"context"
	"time"
)

// ProbeResult is the answer to the one plain web request the sign-in page check makes (ND-231).
// Date is the Date header of the answer, used for the clock check (ND-232); zero when absent.
type ProbeResult struct {
	Status   int
	Location string
	Body     string
	Date     time.Time
	// Expected is true when the answer was the one the system's own test address gives when
	// nothing stands in the way. The source decides, per system.
	Expected bool
}

// SpeedPair is the second speed test (ND-234) and the country the speed server reported (ND-241).
type SpeedPair struct {
	Down, Up float64
	Country  string
}

// Extras are the newer reads. A source that does not have them is simply not asked, so older
// simulated sources keep working.
type Extras interface {
	// Probe sends the system's own sign-in test request.
	Probe(ctx context.Context) (ProbeResult, error)
	// SpeedCloudflare runs one Cloudflare speed test, never Ookla's.
	SpeedCloudflare(ctx context.Context) (SpeedPair, error)
	// Region gives the computer's region as a two-letter country code, or "" when unknown.
	Region(ctx context.Context) string
	// SpeedCountry asks the speed host (a tiny request, not a speed test) which country the
	// connection appears to come from, or "" when it does not say (ND-241).
	SpeedCountry(ctx context.Context) string
}
