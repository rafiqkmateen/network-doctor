package collect

import (
	"reflect"
	"testing"
)

// ND-246: the list of nearby networks must be read on a Windows set to another language. The channel is the
// whole number on the line after the radio type (a value that starts with 802.11), and a block starts at a
// BSSID line, so no English label is needed.
func TestND246NearbyChannelsOtherLanguages(t *testing.T) {
	german := "SSID 1 : Heim\r\n    Netzwerktyp            : Infrastruktur\r\n    Authentifizierung      : WPA2-Personal\r\n" +
		"    BSSID 1                : aa:bb:cc:dd:ee:01\r\n         Signal             : 80%\r\n         Funktyp            : 802.11n\r\n" +
		"         Kanal              : 6\r\n         Basisraten (MBit/s): 1 2 5.5 11\r\n" +
		"SSID 2 : Nachbar\r\n    BSSID 1                : aa:bb:cc:dd:ee:02\r\n         Signal             : 60%\r\n" +
		"         Funktyp            : 802.11ac\r\n         Kanal              : 6\r\n         Basisraten (MBit/s): 6\r\n" +
		"SSID 3 : Weit\r\n    BSSID 1                : aa:bb:cc:dd:ee:03\r\n         Signal             : 30%\r\n" +
		"         Funktyp            : 802.11n\r\n         Kanal              : 11\r\n         Basisraten (MBit/s): 6 12\r\n"
	spanish := "SSID 1 : Casa\r\n    BSSID 1                : aa:bb:cc:dd:ee:01\r\n         Señal              : 80%\r\n" +
		"         Tipo de radio      : 802.11n\r\n         Banda              : 2,4 GHz\r\n         Canal              : 1\r\n" +
		"SSID 2 : Vecino\r\n    BSSID 1                : aa:bb:cc:dd:ee:02\r\n         Señal              : 50%\r\n" +
		"         Tipo de radio      : 802.11n\r\n         Banda              : 2,4 GHz\r\n         Canal              : 1\r\n" +
		"SSID 3 : Lejos\r\n    BSSID 1                : aa:bb:cc:dd:ee:03\r\n         Señal              : 40%\r\n" +
		"         Tipo de radio      : 802.11ac\r\n         Banda              : 5 GHz\r\n         Canal              : 36\r\n"
	french := "SSID 1 : Maison\r\n    BSSID 1                : aa:bb:cc:dd:ee:01\r\n         Signal             : 90%\r\n" +
		"         Type de radio      : 802.11n\r\n         Canal              : 11\r\n" +
		"SSID 2 : Voisin\r\n    BSSID 1                : aa:bb:cc:dd:ee:02\r\n         Signal             : 55%\r\n" +
		"         Type de radio      : 802.11n\r\n         Canal              : 6\r\n"
	for _, c := range []struct {
		name string
		out  string
		own  int
		want []int
	}{
		{"german, own channel 6 left out once", german, 6, []int{6, 11}},
		{"spanish with a band line, own channel 1", spanish, 1, []int{1, 36}},
		{"french, own channel 11", french, 11, []int{6}},
	} {
		got := ParseWlanChannels(c.out, c.own)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: want %v, got %v", c.name, c.want, got)
		}
	}
}

// ND-246: a block with no radio type line is not guessed at: a rates line that is one whole number must not
// be taken for a channel.
func TestND246NoRadioTypeNoGuess(t *testing.T) {
	out := "SSID 1 : X\r\n    BSSID 1                : aa:bb:cc:dd:ee:01\r\n         Signal             : 80%\r\n" +
		"         Basisraten (MBit/s): 6\r\n"
	if got := ParseWlanChannels(out, 0); len(got) != 0 {
		t.Errorf("nothing to read, nothing to give: %v", got)
	}
}

// ND-246: English still reads as before, including the short fixture older tests use.
func TestND246EnglishStillReads(t *testing.T) {
	english := "SSID 1 : Home\r\n    BSSID 1                : aa:bb:cc:dd:ee:01\r\n         Signal             : 80%\r\n" +
		"         Radio type         : 802.11n\r\n         Band               : 2.4 GHz\r\n         Channel            : 6\r\n" +
		"         Basic rates (Mbps) : 1 2 5.5 11\r\n         Other rates (Mbps) : 6 9 12 18 24 36 48 54\r\n" +
		"SSID 2 : Next\r\n    BSSID 1                : aa:bb:cc:dd:ee:02\r\n         Signal             : 50%\r\n" +
		"         Radio type         : 802.11n\r\n         Channel            : 11\r\n"
	if got := ParseWlanChannels(english, 6); !reflect.DeepEqual(got, []int{11}) {
		t.Errorf("want [11], got %v", got)
	}
}
