package collect

import "strings"

// ExtraMakers lets the user add address prefixes from a file next to the program.
var ExtraMakers = map[string]string{}

var makers = map[string]string{
	"00:50:56": "VMware", "00:0C:29": "VMware", "00:05:69": "VMware",
	"08:00:27": "VirtualBox", "00:15:5D": "Microsoft (Hyper-V)",
	"B8:27:EB": "Raspberry Pi", "DC:A6:32": "Raspberry Pi", "E4:5F:01": "Raspberry Pi", "D8:3A:DD": "Raspberry Pi",
	"00:03:93": "Apple", "00:0A:27": "Apple", "00:0A:95": "Apple", "00:1C:B3": "Apple", "00:1E:C2": "Apple", "00:25:00": "Apple",
	"00:1A:11": "Google", "3C:5A:B4": "Google", "F4:F5:D8": "Google",
}

// Maker names the maker of a hardware address when it is on the list.
// A set "locally administered" bit means the phone or laptop is hiding its real address.
func Maker(mac string) string {
	m := strings.ToUpper(strings.ReplaceAll(mac, "-", ":"))
	if len(m) < 8 {
		return "unknown"
	}
	p := m[:8]
	if v, ok := ExtraMakers[p]; ok {
		return v
	}
	if v, ok := makers[p]; ok {
		return v
	}
	if b := hexByte(m[:2]); b&0x02 != 0 {
		return "private address (device hides its maker)"
	}
	return "unknown"
}

func hexByte(s string) int {
	v := 0
	for _, c := range strings.ToLower(s) {
		v <<= 4
		switch {
		case c >= '0' && c <= '9':
			v |= int(c - '0')
		case c >= 'a' && c <= 'f':
			v |= int(c-'a') + 10
		}
	}
	return v
}
