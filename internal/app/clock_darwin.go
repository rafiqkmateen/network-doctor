//go:build darwin

package app

// readClock24h reads the person's own Mac setting for a 24-hour clock (ND-298).
func readClock24h() *bool { return parseMacClock(readSetting(macClockCommand)) }
