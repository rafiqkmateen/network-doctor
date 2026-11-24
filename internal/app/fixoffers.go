package app

import (
	"github.com/rafiqkmateen/network-doctor/internal/core"
	"github.com/rafiqkmateen/network-doctor/internal/fix"
)

// FixOffer is one fix the check says fits what it found, with what it would change.
// Reading it changes nothing.
type FixOffer struct {
	Name       string `json:"name"`
	Title      string `json:"title"`
	Now        string `json:"now"`
	After      string `json:"after"`
	NeedsAdmin bool   `json:"needs_admin"`
}

// fixOffers shows, for each named fix, what the setting is now and what it would become.
// A fix whose setting cannot be read, or that the program would refuse today, is left out,
// so a button is never shown for a fix that cannot work.
func fixOffers(names []string, dir string) []FixOffer {
	out := []FixOffer{}
	if len(names) == 0 {
		return out
	}
	sys := newFixSystem()
	m := fix.NewManager(sys, dir, fix.Options{})
	for _, n := range names {
		if n == core.FixPowerSaving && sys.PowerSavingDropsLink() {
			continue
		}
		pl, err := m.Preview(n)
		if err != nil {
			continue
		}
		if n == core.FixProxy && pl.Before == "off" {
			continue // the fix would be refused (ND-224): on Linux the proxy may be set only in the environment
		}
		out = append(out, FixOffer{Name: n, Title: fix.Title(n), Now: pl.Before, After: pl.After, NeedsAdmin: m.NeedsAdmin(n)})
	}
	return out
}

// FixUndo is one fix the check says can be put back, with the setting now and what Undo restores.
// Reading it changes nothing.
type FixUndo struct {
	Name       string `json:"name"`
	Title      string `json:"title"`
	Now        string `json:"now"`
	After      string `json:"after"`
	NeedsAdmin bool   `json:"needs_admin"`
}

// fixUndos lists the fixes that can be put back from the saved values in dir, the same folder
// "fix undo" reads. A fix offered in this check is left out, so one fix is never offered both ways.
// With no folder it lists nothing.
func fixUndos(dir string, offered []string) []FixUndo {
	out := []FixUndo{}
	if dir == "" {
		return out
	}
	skip := map[string]bool{}
	for _, n := range offered {
		skip[n] = true
	}
	for _, u := range fix.NewManager(newFixSystem(), dir, fix.Options{}).Undos() {
		if skip[u.Name] {
			continue
		}
		out = append(out, FixUndo{Name: u.Name, Title: u.Title, Now: u.Now, After: u.After, NeedsAdmin: u.NeedsAdmin})
	}
	return out
}
