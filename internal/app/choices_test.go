package app

import (
	"os"
	"path/filepath"
	"testing"
)

// ND-207: the installer's choices file says which parts the person chose; a missing or damaged file says nothing.
func TestChoicesFileRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if _, found := LoadChoices(dir); found {
		t.Error("no file means no recorded choices")
	}
	// The exact text the installer writes.
	text := `{"core":true,"scheduled":true,"proactive":false,"ookla":false}`
	if err := os.WriteFile(filepath.Join(dir, "choices.json"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
	c, found := LoadChoices(dir)
	if !found || !c.Core || !c.Scheduled || c.Proactive || c.Ookla {
		t.Errorf("the installer's text must read back exactly: %+v found=%v", c, found)
	}
	if err := SaveChoices(dir, Choices{Core: true, Proactive: true}); err != nil {
		t.Fatal(err)
	}
	c, _ = LoadChoices(dir)
	if !c.Core || c.Scheduled || !c.Proactive {
		t.Errorf("a saved choice must read back: %+v", c)
	}
	if err := os.WriteFile(filepath.Join(dir, "choices.json"), []byte("{broken"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, found := LoadChoices(dir); found {
		t.Error("a damaged file says nothing")
	}
}

// ND-208: turning the weekly check on or off by hand keeps the choices file in step, but only when the installer wrote one.
func TestScheduleChangeKeepsChoicesInStep(t *testing.T) {
	dir := t.TempDir()
	syncScheduledIn(dir, true)
	if _, found := LoadChoices(dir); found {
		t.Error("no installer file means nothing is written")
	}
	if err := SaveChoices(dir, Choices{Core: true, Scheduled: true, Proactive: true}); err != nil {
		t.Fatal(err)
	}
	syncScheduledIn(dir, false)
	c, _ := LoadChoices(dir)
	if c.Scheduled || !c.Proactive || !c.Core {
		t.Errorf("only the weekly choice changes: %+v", c)
	}
}
