package collect

import (
	"os"
	"regexp"
	"strings"
)

var localeRe = regexp.MustCompile(`^[A-Za-z]{2,3}[_-]([A-Za-z]{2})(?:[.@].*)?$`)

// RegionFromLocale reads the country from a locale such as "en_US.UTF-8"; "" when it has none.
func RegionFromLocale(s string) string {
	m := localeRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return ""
	}
	return strings.ToUpper(m[1])
}

// regionFromEnv reads the region from the usual locale settings of Mac and Linux.
func regionFromEnv() string {
	for _, k := range []string{"LC_ALL", "LC_CTYPE", "LANG"} {
		if r := RegionFromLocale(os.Getenv(k)); r != "" {
			return r
		}
	}
	return ""
}

// regionCode keeps only a two-letter country code.
func regionCode(s string) string {
	s = strings.ToUpper(strings.TrimSpace(s))
	if len(s) != 2 || s[0] < 'A' || s[0] > 'Z' || s[1] < 'A' || s[1] > 'Z' {
		return ""
	}
	return s
}
