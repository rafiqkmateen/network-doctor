//go:build !windows

package collect

import (
	"context"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// Generic does the checks that work the same everywhere (ping, speed, DNS).
// The collectors that read adapter, log, device and settings facts for Mac and
// Linux are not built yet; they report "not available on this system yet".
type Generic struct{}

// NewSource returns the collector for this system.
func NewSource() Source { return Generic{} }

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
