package core

import (
	"strings"
	"testing"
)

// with the Wi-Fi details hidden, a slow router may really be a
// weak signal, so the finding says it cannot tell which. Nothing else changes.
func TestDiagnoseSlowRouterWithHiddenWiFi(t *testing.T) {
	hidden := Reading{Kind: WiFi, Online: true, AdapterUp: true, RouterMs: 90, RouterLoss: 4, WiFiHidden: true}
	f := Diagnose(hidden)
	if f.Cause != CauseRouter {
		t.Fatalf("cause should stay router: %q", f.Cause)
	}
	if f.Summary != "The router is slow to answer, or your Wi-Fi signal is weak. Windows is hiding the signal, so this check cannot tell which." {
		t.Fatalf("summary: %q", f.Summary)
	}
	if f.First != "Move closer to the router and run the check again. If it is still slow, restart the router." {
		t.Fatalf("first: %q", f.First)
	}
	if strings.Contains(f.Summary+f.First, "Location") {
		t.Fatal("must not mention Location")
	}
	oldSummary, oldFirst := "The router is slow to answer.", "Restart the router and see how many devices use it."
	for name, r := range map[string]Reading{
		"wired":          {Kind: Wired, Online: true, AdapterUp: true, RouterMs: 90, RouterLoss: 4, LinkMbps: 1000, Duplex: "full"},
		"wifi read fine": {Kind: WiFi, Online: true, AdapterUp: true, RouterMs: 90, RouterLoss: 4, SignalPct: 80},
	} {
		if g := Diagnose(r); g.Cause != CauseRouter || g.Summary != oldSummary || g.First != oldFirst {
			t.Errorf("%s: should keep the old words, got %+v", name, g)
		}
	}
	if g := Diagnose(Reading{Kind: WiFi, Online: true, AdapterUp: true, RouterMs: 1, InternetMs: 20, WiFiHidden: true}); g.Cause != CauseNone {
		t.Errorf("a healthy line with hidden details stays healthy: %+v", g)
	}
}
