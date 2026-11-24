package app

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// hiddenWiFi is a pretend machine where Windows hides the Wi-Fi details.
type hiddenWiFi struct{ *fake }

func (hiddenWiFi) WiFi(context.Context) (collect.WiFiInfo, error) {
	return collect.WiFiInfo{Hidden: true}, nil
}

func hiddenOn(kind core.Kind) hiddenWiFi {
	f := healthy()
	f.adapter.Kind = kind
	return hiddenWiFi{f}
}

const (
	wantHeadline = "Windows is hiding your Wi-Fi signal, band and channel."
	wantWhy      = "Windows only shares them with programs while Location is on. Network Doctor does not use your location and never changes that setting. Speed, drops and devices are not affected."
)

// ND-230: on Wi-Fi with the details hidden, the check says so in plain words, in the JSON and the
// plain output, with no "turn" in the words.
func TestHiddenWiFiIsSaidOnWiFi(t *testing.T) {
	r := Check(context.Background(), hiddenOn(core.WiFi), Options{Quick: true})
	if r.WiFiHidden == nil {
		t.Fatal("the check should say the Wi-Fi details are hidden")
	}
	if r.WiFiHidden.Headline != wantHeadline || r.WiFiHidden.Why != wantWhy {
		t.Fatalf("got %+v", *r.WiFiHidden)
	}
	if strings.Contains(strings.ToLower(r.WiFiHidden.Headline+r.WiFiHidden.Why), "turn") {
		t.Fatal("the words must not tell the person to turn anything on")
	}
	if len(r.LinkAdvice) != 0 {
		t.Fatalf("no Wi-Fi advice from blank details: %v", r.LinkAdvice)
	}
	for _, n := range r.Notes {
		if strings.Contains(n, "Wi-Fi facts") || strings.Contains(n, "hiding") {
			t.Fatalf("the hidden details are not an error note: %v", r.Notes)
		}
	}
	if r.Finding.Cause != core.CauseNone {
		t.Fatalf("hidden details must not change the finding: %+v", r.Finding)
	}
	var out bytes.Buffer
	Run(context.Background(), []string{"check", "--json", "--quick"}, hiddenOn(core.WiFi), t.TempDir(), &out)
	var raw map[string]any
	if err := json.Unmarshal(out.Bytes(), &raw); err != nil {
		t.Fatal(err)
	}
	h, _ := raw["wifi_hidden"].(map[string]any)
	if h["headline"] != wantHeadline || h["why"] != wantWhy {
		t.Fatalf("JSON wifi_hidden: %v", raw["wifi_hidden"])
	}
	out.Reset()
	Run(context.Background(), []string{"check", "--quick"}, hiddenOn(core.WiFi), t.TempDir(), &out)
	if !strings.Contains(out.String(), wantHeadline) {
		t.Fatalf("the plain output should say it too:\n%s", out.String())
	}
}

// ND-230: with the Wi-Fi down, only "The Wi-Fi is not connected" is said.
func TestHiddenWiFiIsNotSaidWhenTheWiFiIsDown(t *testing.T) {
	m := hiddenOn(core.WiFi)
	m.adapter.Up = false
	m.netP = collect.PingResult{LossPct: 100}
	r := Check(context.Background(), m, Options{Quick: true})
	if r.WiFiHidden != nil || r.Reading.WiFiHidden {
		t.Fatalf("Wi-Fi down must not carry it: %+v", r.WiFiHidden)
	}
	if r.Finding.Summary != "The Wi-Fi is not connected." {
		t.Fatalf("got %q", r.Finding.Summary)
	}
}

// ND-230: the reading carries the flag, so a slow router with the details hidden says a weak
// signal cannot be ruled out, and the cause stays the router.
func TestHiddenWiFiReachesTheDiagnosis(t *testing.T) {
	m := hiddenOn(core.WiFi)
	m.routerP = collect.PingResult{AvgMs: 90, LossPct: 4}
	r := Check(context.Background(), m, Options{Quick: true})
	if !r.Reading.WiFiHidden || r.Finding.Cause != core.CauseRouter || !strings.Contains(r.Finding.Summary, "cannot tell which") {
		t.Fatalf("got %+v %+v", r.Reading.WiFiHidden, r.Finding)
	}
}

// readWiFi is a pretend machine where the Wi-Fi read works.
type readWiFi struct{ *fake }

func (readWiFi) WiFi(context.Context) (collect.WiFiInfo, error) {
	return collect.WiFiInfo{SignalPct: 80, Band: "5 GHz", Channel: 36}, nil
}

// ND-230: when the read works, nothing is said about hiding.
func TestHiddenWiFiIsNotSaidWhenTheReadWorks(t *testing.T) {
	f := healthy()
	f.adapter.Kind = core.WiFi
	r := Check(context.Background(), readWiFi{f}, Options{Quick: true})
	if r.WiFiHidden != nil || r.Reading.SignalPct != 80 {
		t.Fatalf("got hidden=%v signal=%d", r.WiFiHidden, r.Reading.SignalPct)
	}
}

// ND-230: a wired connection never carries it, even if the Wi-Fi read would say hidden.
func TestHiddenWiFiIsNeverSaidOnWired(t *testing.T) {
	r := Check(context.Background(), hiddenOn(core.Wired), Options{Quick: true})
	if r.WiFiHidden != nil {
		t.Fatalf("wired must not carry it: %+v", *r.WiFiHidden)
	}
	var out bytes.Buffer
	Run(context.Background(), []string{"check", "--json", "--quick"}, hiddenOn(core.Wired), t.TempDir(), &out)
	if strings.Contains(out.String(), "wifi_hidden") {
		t.Fatal("wired JSON must not have the field")
	}
}
