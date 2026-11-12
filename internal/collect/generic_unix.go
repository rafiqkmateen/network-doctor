//go:build !windows

package collect

import (
	"context"
	"os/exec"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// Generic does the checks that work the same everywhere (ping, speed, DNS).
// On systems with no reader of their own it reports "not available on this
// system yet" for adapter, log, device and settings facts.
type Generic struct{}

func (Generic) Adapter(context.Context) (AdapterInfo, error) {
	return AdapterInfo{Kind: core.Unknown, Up: true}, ErrUnsupported
}
func (Generic) WiFi(context.Context) (WiFiInfo, error) { return WiFiInfo{}, ErrUnsupported }
func (Generic) Ping(ctx context.Context, host string, count int) (PingResult, error) {
	return doPing(ctx, host, count)
}
func (Generic) Throughput(context.Context, int) (Throughput, error) {
	return Throughput{}, ErrUnsupported
}
func (Generic) Events(context.Context, int) ([]core.Event, error) { return nil, ErrUnsupported }
func (Generic) Devices(context.Context, string, int) ([]core.Device, error) {
	return nil, ErrUnsupported
}
func (Generic) Settings(context.Context) (core.Settings, error) {
	return core.Settings{}, ErrUnsupported
}
func (Generic) Speed(ctx context.Context) (float64, float64, error) { return doSpeed(ctx) }
func (Generic) DNSWorks(ctx context.Context) bool                   { return doDNSWorks(ctx) }

// runCmd runs a read-only system command and returns what it printed.
// Every command a Mac or Linux reader runs must be listed in that reader's
// command list so a test can check that it only reads.
func runCmd(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	hide(cmd)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
