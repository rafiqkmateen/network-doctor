package collect

// Checks for ND-242 (the Wi-Fi reader on a Windows set to another language), written by the
// before the build. The three reports below are styled on `netsh wlan show
// interfaces` in German, Spanish and Turkish; they were not captured from a real machine. Each
// holds traps a reader that goes by units and shape could grab by mistake: the number in the
// header line, the receive and send rates, and a network name holding "5 GHz" and "%".
//

import (
	"errors"
	"strings"
	"testing"
)

const ndGerman = "\r\n" +
	"Es ist 1 Schnittstelle auf dem System vorhanden:\r\n" +
	"\r\n" +
	"    Name                   : WLAN\r\n" +
	"    Beschreibung           : Intel(R) Wi-Fi 6 AX201 160MHz\r\n" +
	"    GUID                   : 3f2a8c41-1d2e-4b5a-9c7d-0e1f2a3b4c5d\r\n" +
	"    Physische Adresse      : a4:b1:c2:d3:e4:f5\r\n" +
	"    Status                 : Verbunden\r\n" +
	"    SSID                   : Cafe 5 GHz Gast-100%\r\n" +
	"    BSSID                  : 11:22:33:44:55:66\r\n" +
	"    Netzwerktyp            : Infrastruktur\r\n" +
	"    Funktyp                : 802.11ax\r\n" +
	"    Authentifizierung      : WPA2-Personal\r\n" +
	"    Verschlüsselung        : CCMP\r\n" +
	"    Verbindungsmodus       : Automatische Verbindung mit einem Profil\r\n" +
	"    Band                   : 5 GHz\r\n" +
	"    Kanal                  : 36\r\n" +
	"    Empfangsrate (MBit/s)  : 866,7\r\n" +
	"    Übertragungsrate (MBit/s) : 866,7\r\n" +
	"    Signal                 : 82%\r\n" +
	"    Profil                 : Cafe 5 GHz Gast-100%\r\n" +
	"\r\n" +
	"    Status des gehosteten Netzwerks  : Nicht verfügbar\r\n"

const ndSpanish = "\r\n" +
	"Hay 1 interfaz en el sistema:\r\n" +
	"\r\n" +
	"    Nombre                 : Wi-Fi\r\n" +
	"    Descripción            : Realtek RTL8822CE 802.11ac PCIe Adapter\r\n" +
	"    Estado                 : conectado\r\n" +
	"    SSID                   : MiRed 2.4 GHz 99%\r\n" +
	"    Tipo de radio          : 802.11ac\r\n" +
	"    Autenticación          : WPA2-Personal\r\n" +
	"    Banda                  : 5 GHz\r\n" +
	"    Canal                  : 44\r\n" +
	"    Velocidad de recepción (Mbps)  : 433.3\r\n" +
	"    Velocidad de transmisión (Mbps) : 433.3\r\n" +
	"    Señal                  : 64%\r\n" +
	"    Perfil                 : MiRed 2.4 GHz 99%\r\n"

const ndTurkish = "\r\n" +
	"Sistemde 1 arabirim var:\r\n" +
	"\r\n" +
	"    Ad                     : Wi-Fi\r\n" +
	"    Açıklama               : Qualcomm Atheros QCA9377 Wireless Network Adapter\r\n" +
	"    Durum                  : bağlandı\r\n" +
	"    SSID                   : Ev 5 GHz %90\r\n" +
	"    Radyo türü             : 802.11n\r\n" +
	"    Bant                   : 2,4 GHz\r\n" +
	"    Kanal                  : 6\r\n" +
	"    Alma hızı (Mbps)       : 72,2\r\n" +
	"    İletim hızı (Mbps)     : 72,2\r\n" +
	"    Sinyal                 : %40\r\n" +
	"    Profil                 : Ev 5 GHz %90\r\n"

// A Spanish report from an older Windows with no band line: the band comes from the channel.
const ndSpanishNoBand = "\r\n" +
	"Hay 1 interfaz en el sistema:\r\n" +
	"\r\n" +
	"    Nombre                 : Wi-Fi\r\n" +
	"    Estado                 : conectado\r\n" +
	"    SSID                   : Oficina\r\n" +
	"    Canal                  : 149\r\n" +
	"    Velocidad de recepción (Mbps)  : 300\r\n" +
	"    Señal                  : 51%\r\n"

// ND-242: signal, band and channel are found in a German, Spanish and Turkish report, and the
// traps are not taken.
func TestND242WiFiReaderOtherLanguages(t *testing.T) {
	cases := []struct {
		name    string
		out     string
		signal  int
		band    string
		channel int
	}{
		{"German", ndGerman, 82, "5 GHz", 36},
		{"Spanish", ndSpanish, 64, "5 GHz", 44},
		{"Turkish, percent sign first, decimal comma", ndTurkish, 40, "2.4 GHz", 6},
		{"Spanish with no band line", ndSpanishNoBand, 51, "5 GHz", 149},
	}
	for _, c := range cases {
		got := ParseWlanInterfaces(c.out)
		if got.SignalPct != c.signal || got.Band != c.band || got.Channel != c.channel {
			t.Errorf("%s: got signal %d, band %q, channel %d; want %d, %q, %d",
				c.name, got.SignalPct, got.Band, got.Channel, c.signal, c.band, c.channel)
		}
		if got.Hidden {
			t.Errorf("%s: a report that was read is never hidden", c.name)
		}
	}
}

// ND-242: the English report still reads the same (nothing old breaks).
func TestND242EnglishStillReads(t *testing.T) {
	en := "\r\nThere is 1 interface on the system:\r\n\r\n" +
		"    Name                   : Wi-Fi\r\n" +
		"    State                  : connected\r\n" +
		"    SSID                   : Home 5 GHz 100%\r\n" +
		"    Band                   : 2.4 GHz\r\n" +
		"    Channel                : 11\r\n" +
		"    Receive rate (Mbps)    : 144.4\r\n" +
		"    Transmit rate (Mbps)   : 144.4\r\n" +
		"    Signal                 : 35%\r\n"
	got := ParseWlanInterfaces(en)
	if got.SignalPct != 35 || got.Band != "2.4 GHz" || got.Channel != 11 {
		t.Errorf("English: %+v", got)
	}
}

// ND-242: a failure in another language that does not carry the English words is not called
// hidden from the words alone (the registry consent decides; see the report for the missing seam),
// and a failed run never yields made-up numbers.
func TestND242OtherLanguageFailureIsNotGuessed(t *testing.T) {
	german := "Für Netzwerkshellbefehle ist eine Standortberechtigung erforderlich, um auf WLAN-Informationen zuzugreifen.\r\n"
	got := ParseWlanInterfaces(german)
	if got.SignalPct != 0 || got.Channel != 0 || got.Band != "" {
		t.Errorf("a refusal holds no Wi-Fi numbers: %+v", got)
	}
	if strings.Contains(strings.ToLower(german), "location permission") {
		t.Fatal("fixture must not hold the English words")
	}
}

// ND-242: a failed netsh call in another language counts as hidden only when the Location
// consent value reads denied; a report that was read never does; the English refusal still does.
// The consent read is replaced, so the real setting never matters.
func TestND242ConsentDecidesHidden(t *testing.T) {
	old := LocationConsentDenied
	t.Cleanup(func() { LocationConsentDenied = old })
	exit1 := errors.New("exit status 1")
	german := "Für Netzwerkshellbefehle ist eine Standortberechtigung erforderlich, um auf WLAN-Informationen zuzugreifen.\r\n"
	english := "Network shell commands need location permission to access WLAN information.\r\n"
	cases := []struct {
		name          string
		denied, known bool
		out           string
		err           error
		want          bool
	}{
		{"other language, failed, consent denied", true, true, german, exit1, true},
		{"other language, failed, consent allowed", false, true, german, exit1, false},
		{"other language, failed, consent unknown", false, false, german, exit1, false},
		{"report read fine, consent denied", true, true, ndGerman, nil, false},
		{"English refusal, consent unknown", false, false, english, exit1, true},
		{"English refusal, consent allowed", false, true, english, exit1, true},
	}
	for _, c := range cases {
		d, k := c.denied, c.known
		LocationConsentDenied = func() (bool, bool) { return d, k }
		if got := WlanHidden(c.out, c.err); got != c.want {
			t.Errorf("%s: hidden=%v, want %v", c.name, got, c.want)
		}
	}
}
