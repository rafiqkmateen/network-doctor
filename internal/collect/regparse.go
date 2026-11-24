package collect

import "strings"

// ParseRegValue reads one REG_SZ value from what `reg query <key> /v <name>` prints.
// The type word REG_SZ is the same in every Windows language.
func ParseRegValue(out, name string) (string, bool) {
	for _, l := range lines(out) {
		f := strings.Fields(l)
		if len(f) >= 3 && strings.EqualFold(f[0], name) && f[1] == "REG_SZ" {
			return strings.Join(f[2:], " "), true
		}
	}
	return "", false
}
