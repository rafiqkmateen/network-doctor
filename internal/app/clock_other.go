//go:build !windows && !darwin && !linux

package app

// readClock24h is unknown on the other systems: the page then uses the browser's own format.
func readClock24h() *bool { return nil }
