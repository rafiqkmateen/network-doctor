package store

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ND-226 (store part): the other program's file is a JSON object whose keys are hardware
// addresses in any form; a leading byte order mark is allowed. Empty setting or a missing
// file is no error and adds no one; a half-written file is an error.
func TestLoadKnownFrom(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "trusted.json")
	body := "\xEF\xBB\xBF{\"aa-bb-cc-00-00-77\": \"Phone\", \"11:22:33:44:55:66\": {\"name\": \"TV\"}}"
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	got, err := LoadKnownFrom(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || !got["AA:BB:CC:00:00:77"] || !got["11:22:33:44:55:66"] {
		t.Fatalf("want both addresses, made the same form as the tool's own list: %v", got)
	}

	for _, empty := range []string{"", filepath.Join(dir, "missing.json")} {
		got, err := LoadKnownFrom(empty)
		if err != nil || len(got) != 0 {
			t.Fatalf("%q: want no one and no error, got %v %v", empty, got, err)
		}
	}

	half := filepath.Join(dir, "half.json")
	os.WriteFile(half, []byte("{\"aa-bb-cc-00-00-77\": \"Ph"), 0o644)
	if _, err := LoadKnownFrom(half); err == nil {
		t.Fatal("a half-written file must be an error, not an empty list")
	}
	os.WriteFile(half, nil, 0o644)
	if _, err := LoadKnownFrom(half); err == nil {
		t.Fatal("an empty file must be an error, not an empty list")
	}
}

// ND-229 (store part): the setting is saved as known_from and left out when empty.
func TestKnownFromSetting(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	c, _ := LoadConfig(p)
	c.KnownFrom = filepath.Join("x", "trusted.json")
	if err := SaveConfig(p, c); err != nil {
		t.Fatal(err)
	}
	back, err := LoadConfig(p)
	if err != nil || back.KnownFrom != c.KnownFrom {
		t.Fatalf("saved %q, read %q (%v)", c.KnownFrom, back.KnownFrom, err)
	}
	back.KnownFrom = ""
	SaveConfig(p, back)
	b, _ := os.ReadFile(p)
	if strings.Contains(string(b), "known_from") {
		t.Fatalf("an empty setting must not be written: %s", b)
	}
}
