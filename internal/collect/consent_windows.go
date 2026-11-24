//go:build windows

package collect

import (
	"context"
	"strings"
	"time"
)

// locationConsentKeys are the machine and user places Windows keeps the Location consent.
var locationConsentKeys = []string{
	`HKLM\SOFTWARE\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\location`,
	`HKCU\SOFTWARE\Microsoft\Windows\CurrentVersion\CapabilityAccessManager\ConsentStore\location`,
}

// readLocationConsent reads the two consent values with `reg query`, which only reads.
func readLocationConsent() (denied, known bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	for _, k := range locationConsentKeys {
		out, err := runExe(ctx, "reg", "query", k, "/v", "Value")
		if err != nil {
			continue
		}
		v, ok := ParseRegValue(out, "Value")
		if !ok {
			continue
		}
		known = true
		if strings.EqualFold(v, "Deny") {
			denied = true
		}
	}
	return denied, known
}
