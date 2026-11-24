package app

import "strings"

// systemClock24h tells whether this computer's own time setting is a 24-hour clock, or nil when
// that is not known (a setting that cannot be read, ND-298, ND-299). Tests replace it (ND-262).
var systemClock24h = readClock24h

// parseWindowsClock reads the Windows time setting from the person's own settings. The short time
// format (sShortTime) is the clock the page shows, so it decides first; then the long one
// (sTimeFormat); in a format H is a 24-hour hour and h a 12-hour one, and letters inside single
// quotes are plain text. When neither format says, the older switch iTime does (1 is 24-hour, 0 is
// 12-hour). Anything else is unknown.
func parseWindowsClock(sShortTime, sTimeFormat, iTime string) *bool {
	yes, no := true, false
	for _, f := range []string{sShortTime, sTimeFormat} {
		f = unquoted(f)
		switch {
		case strings.Contains(f, "H"):
			return &yes
		case strings.Contains(f, "h"):
			return &no
		}
	}
	switch strings.TrimSpace(iTime) {
	case "1":
		return &yes
	case "0":
		return &no
	}
	return nil
}

// unquoted drops the text inside single quotes from a Windows time format (two quotes in a row
// inside quoted text stand for a quote, and are still text).
func unquoted(f string) string {
	var b strings.Builder
	in := false
	for _, r := range f {
		if r == '\'' {
			in = !in
			continue
		}
		if !in {
			b.WriteRune(r)
		}
	}
	return b.String()
}
