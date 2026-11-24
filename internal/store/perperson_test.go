package store

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

// ND-294: an installed copy on a Mac or Linux saves per person, in the same folder an unzipped copy
// uses, because only Windows sets ProgramData.
func TestInstalledCopyOnMacAndLinuxSavesPerPerson(t *testing.T) {
	home := t.TempDir()
	exeDir := filepath.Join(home, "prog")
	os.MkdirAll(exeDir, 0o755)
	os.WriteFile(filepath.Join(exeDir, InstalledMarker), nil, 0o644)
	for name, base := range map[string]string{
		"mac":            filepath.Join(home, "Library", "Application Support"),
		"linux":          filepath.Join(home, ".config"),
		"linux with XDG": filepath.Join(home, "xdg"),
	} {
		if got, want := dirFor(exeDir, "", base), filepath.Join(base, "NetworkDoctor"); got != want {
			t.Errorf("%s: %s, want %s", name, got, want)
		}
	}
	// On a real Mac or Linux computer, the system's own per-person folder is the one used.
	switch runtime.GOOS {
	case "darwin":
		t.Setenv("HOME", home)
		b, _ := os.UserConfigDir()
		if b != filepath.Join(home, "Library", "Application Support") {
			t.Errorf("Mac base %s", b)
		}
	case "linux":
		t.Setenv("HOME", home)
		t.Setenv("XDG_CONFIG_HOME", "")
		if b, _ := os.UserConfigDir(); b != filepath.Join(home, ".config") {
			t.Errorf("Linux base %s", b)
		}
		t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, "xdg"))
		if b, _ := os.UserConfigDir(); b != filepath.Join(home, "xdg") {
			t.Errorf("Linux XDG base %s", b)
		}
	}
}
