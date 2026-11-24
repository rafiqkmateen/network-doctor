package collect

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"strings"
	"time"
)

// speedTrace is the speed host's small plain-text answer about the connection; its "loc=" line
// names the country. It is the same host the speed test already uses; no other address is contacted.
const speedTrace = speedBase + "/cdn-cgi/trace"

// speedCloudflare is the second speed test (ND-234): always the Cloudflare test, never Ookla's
// tool, and it does not change which tool the first test reports. It makes no country request:
// the VPN guess that used it was removed, so Country stays "".
func speedCloudflare(ctx context.Context) (SpeedPair, error) {
	down, up, err := runSpeed(ctx, speedBase, defaultSpeedPlan)
	return SpeedPair{Down: down, Up: up}, err
}

// speedCountry makes one tiny request to the speed host and gives the two-letter country it
// names, or "" when it names none or cannot be reached.
func speedCountry(ctx context.Context) string {
	return fetchCountry(ctx, &http.Client{Timeout: 5 * time.Second}, speedTrace)
}

func fetchCountry(ctx context.Context, client *http.Client, url string) string {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return ""
	}
	resp, err := client.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return ""
	}
	sc := bufio.NewScanner(io.LimitReader(resp.Body, 64*1024))
	for sc.Scan() {
		if v, ok := strings.CutPrefix(strings.TrimSpace(sc.Text()), "loc="); ok {
			return countryCode(v)
		}
	}
	return ""
}

// countryCode keeps a two-letter code in capitals and drops anything else.
func countryCode(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) != 2 || s[0] < 'A' || s[0] > 'Z' || s[1] < 'A' || s[1] > 'Z' {
		return ""
	}
	return s
}
