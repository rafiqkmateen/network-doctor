package app

// Check for the first tool's name in speed_recheck, written before the
// build: when the second test (Cloudflare) is higher and replaces the shown pair, the first pair
// keeps the name of the tool that measured it.

import (
	"context"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
	"github.com/rafiqkmateen/network-doctor/internal/core"
)

func TestSpeedRecheckKeepsTheFirstToolsName(t *testing.T) {
	for _, first := range []string{collect.ToolOokla, collect.ToolCloudflare} {
		s := ndSpeed(10, 100, collect.SpeedPair{Down: 280, Up: 95})
		rep := &Report{SpeedTool: first}
		rd := &core.Reading{Kind: core.Wired, AdapterUp: true, Online: true, SpeedMeasured: true, DownMbps: 10, UpMbps: 100}
		fillSpeedExtras(context.Background(), rep, rd, s, Options{})
		rc := rep.SpeedRecheck
		if rc == nil || !rc.Finished {
			t.Fatalf("first %s: the second test must run and finish: %+v", first, rc)
		}
		if rc.FirstTool != first {
			t.Errorf("first %s: first_tool %q, want the first test's tool", first, rc.FirstTool)
		}
		if rc.Tool != collect.ToolCloudflare {
			t.Errorf("first %s: tool %q, want Cloudflare for the second test", first, rc.Tool)
		}
		if rep.SpeedTool != collect.ToolCloudflare || rd.DownMbps != 280 {
			t.Errorf("first %s: the higher second pair is the one shown: tool %q, down %v", first, rep.SpeedTool, rd.DownMbps)
		}
	}
}
