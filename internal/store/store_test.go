package store

import (
	"path/filepath"
	"testing"
	"time"
)

// ND-71: one row per weekly check, in a file a spreadsheet opens.
func TestWeeklyCSVRoundTrip(t *testing.T) {
	p := filepath.Join(t.TempDir(), "weekly.csv")
	when := time.Date(2026, 11, 9, 9, 0, 0, 0, time.Local)
	if err := AppendWeekly(p, Row{Time: when, Down: 280.5, Up: 20.25, PingMs: 12, Plan: 300}); err != nil {
		t.Fatal(err)
	}
	if err := AppendWeekly(p, Row{Time: when.AddDate(0, 0, 7), Down: 100, Up: 10, PingMs: 40, Plan: 300}); err != nil {
		t.Fatal(err)
	}
	rows, err := ReadWeekly(p)
	if err != nil || len(rows) != 2 {
		t.Fatalf("rows %v err %v", rows, err)
	}
	if rows[0].Down != 280.5 || rows[0].Plan != 300 || !rows[0].Time.Equal(when) {
		t.Fatalf("bad row %+v", rows[0])
	}
}

func TestKnownDevices(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known.txt")
	if err := AddKnown(p, "aa-bb-cc-dd-ee-ff", "my phone"); err != nil {
		t.Fatal(err)
	}
	k, err := LoadKnown(p)
	if err != nil || !k["AA:BB:CC:DD:EE:FF"] {
		t.Fatalf("%v %v", k, err)
	}
	if k2, _ := LoadKnown(filepath.Join(t.TempDir(), "none.txt")); len(k2) != 0 {
		t.Fatal("missing file means nobody is known")
	}
}

func TestConfigDefaults(t *testing.T) {
	c, err := LoadConfig(filepath.Join(t.TempDir(), "config.json"))
	if err != nil {
		t.Fatal(err)
	}
	if c.WeeklyDay != time.Monday || c.WeeklyHour != 9 {
		t.Fatal("default weekly slot is Monday 0900")
	}
	if len(c.Sinks) != 1 || c.Sinks[0] != "console" {
		t.Fatal("default sink is the console only")
	}
}
