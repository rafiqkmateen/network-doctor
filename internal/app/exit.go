package app

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/collect"
)

// Where the internet leaves from (ND-248). The check asks the speed host one tiny question, which country the
// connection appears to come from, shows it in plain words, and remembers it. When it differs from the place
// remembered, a plain note says so for two weeks. It states the change and never guesses a VPN: with one link the
// program has no way to know where the person really is.

const (
	exitFile       = "exit-place.json"
	exitNoteDays   = 14
	exitDateLayout = "2006-01-02"
)

// exitState is the remembered place: the current country, the date it was first seen, and the one before it.
type exitState struct {
	Country  string `json:"country"`
	Since    string `json:"since"`
	Previous string `json:"previous,omitempty"`
}

// countryWords names the countries people meet most; any other code is said as a code.
var countryWords = map[string]string{
	"AR": "Argentina", "AU": "Australia", "AT": "Austria", "BE": "Belgium", "BR": "Brazil", "CA": "Canada",
	"CL": "Chile", "CN": "China", "CO": "Colombia", "CR": "Costa Rica", "CZ": "Czechia", "DK": "Denmark",
	"DO": "the Dominican Republic", "EC": "Ecuador", "EG": "Egypt", "FI": "Finland", "FR": "France",
	"DE": "Germany", "GR": "Greece", "GT": "Guatemala", "HK": "Hong Kong", "HU": "Hungary", "IN": "India",
	"ID": "Indonesia", "IE": "Ireland", "IL": "Israel", "IT": "Italy", "JP": "Japan", "KE": "Kenya",
	"KR": "South Korea", "MX": "Mexico", "NL": "the Netherlands", "NZ": "New Zealand", "NG": "Nigeria",
	"NO": "Norway", "PA": "Panama", "PE": "Peru", "PH": "the Philippines", "PL": "Poland", "PT": "Portugal",
	"PR": "Puerto Rico", "RO": "Romania", "RU": "Russia", "SA": "Saudi Arabia", "SG": "Singapore",
	"ZA": "South Africa", "ES": "Spain", "SE": "Sweden", "CH": "Switzerland", "TW": "Taiwan", "TH": "Thailand",
	"TR": "Turkey", "UA": "Ukraine", "AE": "the United Arab Emirates", "GB": "the United Kingdom",
	"US": "the United States", "UY": "Uruguay", "VE": "Venezuela", "VN": "Vietnam",
}

// placeWords is the country in plain words, or "" when there is none.
func placeWords(code string) string {
	if code == "" {
		return ""
	}
	if w, ok := countryWords[code]; ok {
		return w
	}
	return "a country with the code " + code
}

// exitStep takes the remembered state and today's reading and gives the new state and the note to show, if any.
func exitStep(st exitState, now, today string) (exitState, *Note) {
	switch {
	case now == "":
		return st, nil
	case st.Country == "":
		return exitState{Country: now, Since: today}, nil
	case now != st.Country:
		st = exitState{Country: now, Since: today, Previous: st.Country}
	case st.Previous != "" && !exitDated(st.Since, today):
		st.Since = today // an empty, damaged or future date (a clock that was wrong) starts the two weeks again
	case st.Previous != "" && exitDays(st.Since, today) >= exitNoteDays:
		st.Previous = ""
	}
	if st.Previous == "" {
		return st, nil
	}
	return st, &Note{
		Headline: "Your internet now leaves from " + placeWords(st.Country) + ".",
		Why: "Since " + st.Since + " your internet has left from " + placeWords(st.Country) + ". Before that it left from " +
			placeWords(st.Previous) + ". A VPN, on this computer or on your router, can do this, and so can a change at your provider. " +
			"Speed and reply times may describe that path, not your own line.",
	}
}

// exitDated says whether since is a real date that is not after today.
func exitDated(since, today string) bool {
	a, errA := time.Parse(exitDateLayout, since)
	b, errB := time.Parse(exitDateLayout, today)
	return errA == nil && errB == nil && !a.After(b)
}

// exitDays is the whole days from one date to another, or 0 when either is not a date.
func exitDays(from, to string) int {
	a, errA := time.Parse(exitDateLayout, from)
	b, errB := time.Parse(exitDateLayout, to)
	if errA != nil || errB != nil {
		return 0
	}
	return int(b.Sub(a).Hours() / 24)
}

func loadExit(dir string) exitState {
	var st exitState
	if b, err := os.ReadFile(filepath.Join(dir, exitFile)); err == nil {
		_ = json.Unmarshal(b, &st)
	}
	return st
}

// saveExit writes the place whole or not at all (a temporary file, then a rename), and makes the folder if it is
// missing, so a check running at the same moment never reads a half-written file.
func saveExit(dir string, st exitState) {
	b, err := json.Marshal(st)
	if err != nil || os.MkdirAll(dir, 0o755) != nil {
		return
	}
	tmp, err := os.CreateTemp(dir, exitFile+".*.tmp")
	if err != nil {
		return
	}
	_, werr := tmp.Write(b)
	cerr := tmp.Close()
	if werr != nil || cerr != nil || os.Rename(tmp.Name(), filepath.Join(dir, exitFile)) != nil {
		_ = os.Remove(tmp.Name())
	}
}

// fillExit sets the exit place and the change note on the report. It asks only when the check is online, says
// nothing when the host names no country, and leaves the remembered place alone in both cases.
func fillExit(ctx context.Context, rep *Report, src collect.Source, dir string) {
	if !rep.Reading.Online {
		return
	}
	cs, ok := src.(interface{ SpeedCountry(context.Context) string })
	if !ok {
		return
	}
	code := cs.SpeedCountry(ctx)
	if code == "" {
		return
	}
	rep.ExitPlace = placeWords(code)
	st, note := exitStep(loadExit(dir), code, rep.Time.Format(exitDateLayout))
	saveExit(dir, st)
	rep.VPNHint = note
}
