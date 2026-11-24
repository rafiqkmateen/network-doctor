package app

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ND-248: the card says where the internet leaves from, and adds a plain note when that place changed.

func TestND248PlaceWords(t *testing.T) {
	for code, want := range map[string]string{
		"FR": "France", "US": "the United States", "GB": "the United Kingdom", "DE": "Germany",
		"ZZ": "a country with the code ZZ", "": "",
	} {
		if got := placeWords(code); got != want {
			t.Errorf("placeWords(%q) = %q, want %q", code, got, want)
		}
	}
}

func TestND248StepFirstReadingSaysNothing(t *testing.T) {
	st, n := exitStep(exitState{}, "FR", "2026-11-17")
	if n != nil || st.Country != "FR" || st.Since != "2026-11-17" || st.Previous != "" {
		t.Errorf("a first reading is only remembered: %+v %v", st, n)
	}
}

func TestND248StepSamePlaceSaysNothing(t *testing.T) {
	in := exitState{Country: "FR", Since: "2026-11-01"}
	st, n := exitStep(in, "FR", "2026-11-17")
	if n != nil || st != in {
		t.Errorf("same place, no change: %+v %v", st, n)
	}
}

func TestND248StepChangeGivesANoteWithBothPlaces(t *testing.T) {
	st, n := exitStep(exitState{Country: "FR", Since: "2026-11-01"}, "US", "2026-11-17")
	if st.Country != "US" || st.Previous != "FR" || st.Since != "2026-11-17" {
		t.Fatalf("state after a change: %+v", st)
	}
	if n == nil || !strings.Contains(n.Headline, "the United States") || !strings.Contains(n.Why, "France") {
		t.Fatalf("the note must name both places: %+v", n)
	}
	for _, w := range []string{"VPN", "2026-11-17"} {
		if !strings.Contains(n.Why, w) {
			t.Errorf("the reason must mention %q: %q", w, n.Why)
		}
	}
	if strings.Contains(strings.ToLower(n.Headline+n.Why), "you are using a vpn") {
		t.Error("a change is a fact, not a VPN verdict")
	}
}

func TestND248StepNoteStaysForTwoWeeksThenGoes(t *testing.T) {
	in := exitState{Country: "US", Previous: "FR", Since: "2026-11-17"}
	if _, n := exitStep(in, "US", "2026-11-30"); n == nil {
		t.Error("the note stays for 14 days")
	}
	st, n := exitStep(in, "US", "2026-12-01")
	if n != nil || st.Previous != "" {
		t.Errorf("after 14 days the note goes and the old place is forgotten: %+v %v", st, n)
	}
}

type exitFake struct {
	*fake
	country string
}

func (e exitFake) SpeedCountry(context.Context) string { return e.country }

func TestND248CheckShowsPlaceAndNote(t *testing.T) {
	dir := t.TempDir()
	saveExit(dir, exitState{Country: "FR", Since: "2026-11-01"})
	rep := runCheck(context.Background(), exitFake{healthy(), "US"}, dir, Options{Quick: true})
	if rep.ExitPlace != "the United States" {
		t.Errorf("exit place: %q", rep.ExitPlace)
	}
	if rep.VPNHint == nil || !strings.Contains(rep.VPNHint.Why, "France") {
		t.Errorf("the change note is missing: %+v", rep.VPNHint)
	}
	if got := loadExit(dir); got.Country != "US" || got.Previous != "FR" {
		t.Errorf("the new place must be remembered: %+v", got)
	}
}

func TestND248CheckOfflineChangesNothing(t *testing.T) {
	dir := t.TempDir()
	saveExit(dir, exitState{Country: "FR", Since: "2026-11-01"})
	f := healthy()
	f.netP.LossPct = 100
	rep := runCheck(context.Background(), exitFake{f, "US"}, dir, Options{Quick: true})
	if rep.ExitPlace != "" || rep.VPNHint != nil {
		t.Errorf("offline: nothing to say: %q %+v", rep.ExitPlace, rep.VPNHint)
	}
	if got := loadExit(dir); got.Country != "FR" {
		t.Errorf("offline must not touch the saved place: %+v", got)
	}
}

func TestND248CheckWithoutAReadingKeepsTheOldPlace(t *testing.T) {
	dir := t.TempDir()
	saveExit(dir, exitState{Country: "FR", Since: "2026-11-01"})
	rep := runCheck(context.Background(), exitFake{healthy(), ""}, dir, Options{Quick: true})
	if rep.ExitPlace != "" || rep.VPNHint != nil {
		t.Errorf("no country given, nothing said: %q %+v", rep.ExitPlace, rep.VPNHint)
	}
	if got := loadExit(dir); got.Country != "FR" || got.Since != "2026-11-01" {
		t.Errorf("an empty reading must not erase the saved place: %+v", got)
	}
	if _, err := filepath.Abs(dir); err != nil {
		t.Fatal(err)
	}
}

// ND-248, audit: a remembered date that is empty, not a date, or in the future must not keep the note for good.
func TestND248StepBadOrFutureDateRestartsTheWindow(t *testing.T) {
	for _, since := range []string{"", "not a date", "2027-11-17"} {
		in := exitState{Country: "US", Previous: "FR", Since: since}
		st, n := exitStep(in, "US", "2026-11-17")
		if st.Since != "2026-11-17" || n == nil {
			t.Errorf("since %q: the window must restart today and still show the note: %+v %v", since, st, n)
		}
		if _, later := exitStep(st, "US", "2026-12-01"); later != nil {
			t.Errorf("since %q: 14 days after the restart the note must go", since)
		}
	}
}

// ND-248, audit: the place is remembered even when the data folder does not exist yet, and the file is whole.
func TestND248SaveCreatesTheFolderAndLeavesNoTempFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "not", "there", "yet")
	saveExit(dir, exitState{Country: "US", Since: "2026-11-17", Previous: "FR"})
	if got := loadExit(dir); got.Country != "US" || got.Previous != "FR" {
		t.Errorf("saved place not read back: %+v", got)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 || entries[0].Name() != exitFile {
		t.Errorf("only the one file should be left: %v %v", entries, err)
	}
}

// ND-248, audit: the plain text report says the place and the note too, not only the JSON.
func TestND248TextReportShowsPlaceAndNote(t *testing.T) {
	rep := Report{ExitPlace: "the United States", VPNHint: &Note{Headline: "Your internet now leaves from the United States.", Why: "Before that it left from France."}}
	var b strings.Builder
	printReport(&b, rep)
	for _, w := range []string{"Your internet leaves from the United States.", "Your internet now leaves from the United States.", "Before that it left from France."} {
		if !strings.Contains(b.String(), w) {
			t.Errorf("the text report is missing %q", w)
		}
	}
}
