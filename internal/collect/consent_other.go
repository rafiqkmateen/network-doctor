//go:build !windows

package collect

// readLocationConsent: only Windows keeps a Location consent value for the Wi-Fi details.
func readLocationConsent() (denied, known bool) { return false, false }
