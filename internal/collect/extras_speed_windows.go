//go:build windows

package collect

import "context"

// SpeedCloudflare runs one Cloudflare speed test, never Ookla's (ND-234).
func (Windows) SpeedCloudflare(ctx context.Context) (SpeedPair, error) { return speedCloudflare(ctx) }

// SpeedCountry asks the speed host which country the connection appears to come from (ND-241).
func (Windows) SpeedCountry(ctx context.Context) string { return speedCountry(ctx) }
