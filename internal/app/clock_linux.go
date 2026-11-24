//go:build linux

package app

// readClock24h reads the desktop's clock format (ND-299).
func readClock24h() *bool { return parseLinuxClock(readSetting(linuxClockCommand)) }
