package store

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// ND-264: a lock is removed only by the save that made it, never by one whose lock was taken over.
func TestLockIsRemovedOnlyByItsOwner(t *testing.T) {
	lock := filepath.Join(t.TempDir(), "x.lock")
	unlock, err := lockFile(lock)
	if err != nil {
		t.Fatal(err)
	}
	os.WriteFile(lock, []byte("someone-else"), 0o644) // another save took it over
	unlock()
	if _, err := os.Stat(lock); err != nil {
		t.Error("a lock that now belongs to another save must not be removed")
	}
}

// ND-264: several saves racing to take over a lock left behind by a stopped program never run at
// the same time, so no change is lost.
func TestRaceOnAStaleLockLosesNothing(t *testing.T) {
	dir := t.TempDir()
	lock, counter := filepath.Join(dir, "c.lock"), filepath.Join(dir, "c.txt")
	os.WriteFile(lock, []byte("stopped-program"), 0o644)
	old := time.Now().Add(-time.Minute)
	os.Chtimes(lock, old, old)
	os.WriteFile(counter, []byte("0"), 0o644)
	const n = 12
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			unlock, err := lockFile(lock)
			if err != nil {
				t.Error(err)
				return
			}
			b, _ := os.ReadFile(counter)
			v, _ := strconv.Atoi(string(b))
			time.Sleep(2 * time.Millisecond)
			os.WriteFile(counter, []byte(strconv.Itoa(v+1)), 0o644)
			unlock()
		}()
	}
	wg.Wait()
	if b, _ := os.ReadFile(counter); string(b) != strconv.Itoa(n) {
		t.Errorf("want %d changes, got %s: two saves ran at once", n, b)
	}
}

// makeUnwritable takes away write permission on dir, or skips when this system cannot.
func makeUnwritable(t *testing.T, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		if err := exec.Command("icacls", dir, "/deny", "*S-1-1-0:(W,AD,WD)").Run(); err != nil {
			t.Skip("icacls could not change the folder")
		}
		t.Cleanup(func() { exec.Command("icacls", dir, "/remove:d", "*S-1-1-0").Run() })
	} else {
		os.Chmod(dir, 0o500)
		t.Cleanup(func() { os.Chmod(dir, 0o755) })
	}
	if err := os.WriteFile(filepath.Join(dir, "probe"), nil, 0o644); err == nil {
		os.Remove(filepath.Join(dir, "probe"))
		t.Skip("the folder is still writable here (for example as an administrator)")
	}
}

// ND-264: a folder that cannot be written says so at once, and does not blame another program.
func TestUnwritableFolderFailsFastAndHonestly(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "ro")
	os.Mkdir(dir, 0o755)
	makeUnwritable(t, dir)
	start := time.Now()
	err := SetKnown(filepath.Join(dir, "known-devices.txt"), "02:00:00:00:00:20", Mine, "")
	if err == nil {
		t.Fatal("saving into a folder that cannot be written must fail")
	}
	if time.Since(start) > time.Second {
		t.Errorf("it must fail at once, took %v", time.Since(start))
	}
	if strings.Contains(err.Error(), "another program") {
		t.Errorf("the error must say what is wrong, not blame another program: %v", err)
	}
}

// ND-264: temporary files left by a save that stopped are cleaned up once they are a minute old.
func TestOldTempFilesAreCleaned(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "known-devices.txt")
	oldTmp, newTmp := p+".tmp111", p+".tmp222"
	os.WriteFile(oldTmp, []byte("x"), 0o644)
	os.WriteFile(newTmp, []byte("x"), 0o644)
	old := time.Now().Add(-2 * time.Minute)
	os.Chtimes(oldTmp, old, old)
	if err := SetKnown(p, "02:00:00:00:00:20", Mine, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(oldTmp); err == nil {
		t.Error("a temporary file over a minute old must be removed")
	}
	if _, err := os.Stat(newTmp); err != nil {
		t.Error("a fresh temporary file may belong to a save in progress and must stay")
	}
}

// ND-265: settings changed at the same time are each kept: the change is made to the file as it is
// under the lock, so none is lost.
func TestConfigUpdatesAreNotLost(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	const n = 16
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			if err := UpdateConfig(p, func(c *Config) error { c.Allow = append(c.Allow, fmt.Sprintf("10.0.0.%d", i)); return nil }); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	c, err := LoadConfig(p)
	if err != nil || len(c.Allow) != n {
		t.Errorf("want %d changes kept, got %d (%v)", n, len(c.Allow), err)
	}
}

// ND-264: the list reads the same with a byte-order mark at the start and tags in any case, and a
// name that really starts with a tag keeps its words.
func TestKnownListReadingIsForgiving(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known-devices.txt")
	os.WriteFile(p, []byte("\xEF\xBB\xBF02:00:00:00:00:20 [IGNORED] printer\n02:00:00:00:00:21 [Not-Mine]\n"), 0o644)
	st := LoadKnownStates(p)
	if st["02:00:00:00:00:20"] != (KnownEntry{State: Ignored, Name: "printer"}) || st["02:00:00:00:00:21"].State != NotMine {
		t.Errorf("BOM and tag case: %v", st)
	}
	q := filepath.Join(t.TempDir(), "known-devices.txt")
	SetKnown(q, "02:00:00:00:00:22", Mine, "[mine] x")
	SetKnown(q, "02:00:00:00:00:23", Mine, "[ignored]tv")
	st = LoadKnownStates(q)
	if st["02:00:00:00:00:22"] != (KnownEntry{State: Mine, Name: "(mine) x"}) || st["02:00:00:00:00:23"] != (KnownEntry{State: Mine, Name: "(ignored)tv"}) {
		t.Errorf("a name starting with a tag keeps its words in round brackets: %v", st)
	}
}

// makeUnremovable stops lock being renamed or removed, or skips when this system cannot.
func makeUnremovable(t *testing.T, lock string) {
	t.Helper()
	dir := filepath.Dir(lock)
	if runtime.GOOS == "windows" {
		e1 := exec.Command("icacls", lock, "/deny", "*S-1-1-0:(D)").Run()
		e2 := exec.Command("icacls", dir, "/deny", "*S-1-1-0:(DC)").Run()
		t.Cleanup(func() {
			exec.Command("icacls", dir, "/remove:d", "*S-1-1-0").Run()
			exec.Command("icacls", lock, "/remove:d", "*S-1-1-0").Run()
		})
		if e1 != nil || e2 != nil {
			t.Skip("icacls could not change the lock")
		}
	} else {
		os.Chmod(dir, 0o500)
		t.Cleanup(func() { os.Chmod(dir, 0o755) })
	}
	if err := os.Rename(lock, lock+".probe"); err == nil {
		os.Rename(lock+".probe", lock)
		t.Skip("the lock can still be removed here (for example as an administrator)")
	}
}

// ND-264: a stale lock that cannot be removed ends the save within the 5-second limit with a
// clear message, and the next save in the same program still works once it is gone.
func TestStaleLockThatCannotBeRemovedFailsInTime(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "d")
	os.Mkdir(dir, 0o755)
	p := filepath.Join(dir, "known-devices.txt")
	lock := p + ".lock"
	os.WriteFile(lock, []byte("stopped-program"), 0o644)
	old := time.Now().Add(-time.Minute)
	os.Chtimes(lock, old, old)
	makeUnremovable(t, lock)
	start := time.Now()
	done := make(chan error, 1)
	go func() { done <- SetKnown(p, "02:00:00:00:00:20", Mine, "") }()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("the save cannot succeed while the old lock stays")
		}
		if d := time.Since(start); d > 6500*time.Millisecond {
			t.Errorf("took %v, want about 5 seconds at most", d)
		}
		if !strings.Contains(err.Error(), lock) {
			t.Errorf("the message must name the lock to remove: %v", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the save kept trying past its limit")
	}
	// Put things right, as a person would, and save again in the same program.
	if runtime.GOOS == "windows" {
		exec.Command("icacls", dir, "/remove:d", "*S-1-1-0").Run()
		exec.Command("icacls", lock, "/remove:d", "*S-1-1-0").Run()
	} else {
		os.Chmod(dir, 0o755)
	}
	os.Remove(lock)
	if err := SetKnown(p, "02:00:00:00:00:20", Mine, ""); err != nil {
		t.Errorf("the next save must work: %v", err)
	}
}

// ND-264: another program's fresh takeover beside a stale lock does not stretch the wait past the
// 5-second limit.
func TestFreshTakeoverKeepsTheLimit(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known-devices.txt")
	lock := p + ".lock"
	os.WriteFile(lock, []byte("stopped-program"), 0o644)
	old := time.Now().Add(-time.Minute)
	os.Chtimes(lock, old, old)
	stop := make(chan struct{})
	defer close(stop)
	go func() { // keep the takeover guard fresh, as a program in the middle of a takeover would
		for {
			select {
			case <-stop:
				return
			default:
				os.WriteFile(lock+".takeover", nil, 0o644)
				time.Sleep(50 * time.Millisecond)
			}
		}
	}()
	start := time.Now()
	SetKnown(p, "02:00:00:00:00:20", Mine, "")
	if d := time.Since(start); d > 6500*time.Millisecond {
		t.Errorf("took %v, want about 5 seconds at most", d)
	}
}
