package app

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// choicesFile sits beside the program. The installer writes it and says which parts
// the person chose; Khadijah's card reads it to show only those parts.
const choicesFile = "choices.json"

// Choices are the parts the person chose at install. The check on demand (Core) is always there.
type Choices struct {
	Core      bool `json:"core"`
	Scheduled bool `json:"scheduled"` // the weekly check
	Proactive bool `json:"proactive"` // the alerts helper
	Ookla     bool `json:"ookla"`     // Ookla's tool was fetched by the installer, with the person's agreement
}

// LoadChoices reads the choices file in dir. A missing or damaged file says nothing.
func LoadChoices(dir string) (Choices, bool) {
	b, err := os.ReadFile(filepath.Join(dir, choicesFile))
	if err != nil {
		return Choices{}, false
	}
	var c Choices
	if json.Unmarshal(b, &c) != nil {
		return Choices{}, false
	}
	return c, true
}

// SaveChoices writes the choices file in dir.
func SaveChoices(dir string, c Choices) error {
	b, err := json.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, choicesFile), b, 0o644)
}

// syncScheduledIn keeps the weekly choice in step when the person turns the weekly
// check on or off by hand. It does nothing when the installer wrote no file.
func syncScheduledIn(dir string, on bool) {
	c, found := LoadChoices(dir)
	if !found || c.Scheduled == on {
		return
	}
	c.Scheduled = on
	_ = SaveChoices(dir, c) // best effort: the program folder may not be writable
}

// syncScheduled does the same for the folder that holds the running program.
func syncScheduled(on bool) {
	if exe, err := os.Executable(); err == nil {
		syncScheduledIn(filepath.Dir(exe), on)
	}
}
