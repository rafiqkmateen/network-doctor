package store

import (
	"os"
	"path/filepath"
	"testing"
)

// ND-116: an installed copy saves in ProgramData; an unzipped copy keeps saving per person.
func TestDirForInstalledCopyUsesProgramData(t *testing.T) {
	exeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(exeDir, InstalledMarker), []byte("installed by setup\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := dirFor(exeDir, `C:\ProgramData`, `C:\Users\pat\AppData\Roaming`)
	want := filepath.Join(`C:\ProgramData`, "NetworkDoctor")
	if got != want {
		t.Fatalf("installed copy: got %q, want %q", got, want)
	}
}

func TestDirForUnzippedCopyStaysPerPerson(t *testing.T) {
	exeDir := t.TempDir() // no marker file
	got := dirFor(exeDir, `C:\ProgramData`, `C:\Users\pat\AppData\Roaming`)
	want := filepath.Join(`C:\Users\pat\AppData\Roaming`, "NetworkDoctor")
	if got != want {
		t.Fatalf("unzipped copy: got %q, want %q", got, want)
	}
}

func TestDirForFallsBackWhenFoldersAreUnknown(t *testing.T) {
	exeDir := t.TempDir()
	if got := dirFor(exeDir, "", ""); got != filepath.Join(".", "NetworkDoctor") {
		t.Fatalf("no folders known: got %q", got)
	}
	_ = os.WriteFile(filepath.Join(exeDir, InstalledMarker), nil, 0o644)
	if got := dirFor(exeDir, "", `C:\Users\pat\AppData\Roaming`); got != filepath.Join(`C:\Users\pat\AppData\Roaming`, "NetworkDoctor") {
		t.Fatalf("installed but no ProgramData: got %q", got)
	}
}
