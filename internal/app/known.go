package app

import (
	"fmt"
	"io"
	"path/filepath"

	"github.com/rafiqkmateen/network-doctor/internal/store"
)

// knownList joins the tool's own known list (known-devices.txt) with the list named by the
// known_from setting, such as a trusted-devices file. The check and the
// watcher both use it, so they agree on who is known.
type knownList struct {
	ownPath  string
	fromPath string
	out      io.Writer // where the one plain note goes; nil means say nothing

	lastGood map[string]bool // the other list as it last read well
	noted    bool            // the note about an unreadable file has been said
}

func newKnownList(dir, from string, out io.Writer) *knownList {
	return &knownList{ownPath: filepath.Join(dir, "known-devices.txt"), fromPath: from, out: out}
}

// Load reads both lists again and returns the joined list. When the other file exists but
// cannot be read or understood, the last list that read well is kept (none if it never read
// well) and one plain note is printed, not again until the file reads well once more.
func (k *knownList) Load() map[string]bool {
	known, _ := store.LoadKnown(k.ownPath)
	from, err := store.LoadKnownFrom(k.fromPath)
	if err != nil {
		if !k.noted && k.out != nil {
			if len(k.lastGood) > 0 {
				fmt.Fprintf(k.out, "Note: the list of trusted devices at %s could not be read right now (it may be in the middle of being saved). Using the last list that read well until it can be read again.\n", k.fromPath)
			} else {
				fmt.Fprintf(k.out, "Note: the list of trusted devices at %s could not be read right now (it may be in the middle of being saved). Only your own known list counts until it can be read.\n", k.fromPath)
			}
		}
		k.noted = true
		from = k.lastGood
	} else {
		k.noted = false
		k.lastGood = from
	}
	for m := range from {
		known[m] = true
	}
	return known
}

// States reads every choice on the tool's own known list (mine, not mine, ignored) with its name.
func (k *knownList) States() map[string]store.KnownEntry { return store.LoadKnownStates(k.ownPath) }
