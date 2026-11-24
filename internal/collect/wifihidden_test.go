package collect

import (
	"errors"
	"testing"
)

// The exact words Windows printed on a Windows computer with Location services off.
const locationRefusal = "Network shell commands need location permission to access WLAN information. Turn on Location services on the Location page in Privacy & security settings.\r\n"

// ND-230: only Windows' location refusal with a failed exit counts as hidden; a normal report,
// one naming a network "location permission", and any other failure do not.
func TestWlanHidden(t *testing.T) {
	// The real Location consent must not decide the answer (ND-242).
	old := LocationConsentDenied
	t.Cleanup(func() { LocationConsentDenied = old })
	LocationConsentDenied = func() (bool, bool) { return false, false }
	exit1 := errors.New("exit status 1")
	normal := "    Name                   : Wi-Fi\r\n    State                  : connected\r\n    Band                   : 2.4 GHz\r\n    Channel                : 6\r\n    Signal                 : 35%\r\n"
	cases := []struct {
		name string
		out  string
		err  error
		want bool
	}{
		{"the location refusal", locationRefusal, exit1, true},
		{"the refusal in other capitals", "network shell commands need LOCATION PERMISSION to access WLAN information.", exit1, true},
		{"a normal report", normal, nil, false},
		{"a network named with the phrase, read fine", "    Name                   : Wi-Fi\r\n    SSID                   : location permission\r\n    Signal                 : 80%\r\n", nil, false},
		{"the wireless service not running", "The Wireless AutoConfig Service (wlansvc) is not running.\r\n", exit1, false},
		{"no output and an error", "", exit1, false},
	}
	for _, c := range cases {
		if got := WlanHidden(c.out, c.err); got != c.want {
			t.Errorf("%s: got %v, want %v", c.name, got, c.want)
		}
	}
}
