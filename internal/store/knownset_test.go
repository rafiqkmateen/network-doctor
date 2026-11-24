package store

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// ND-264: marking a device that is already on the list updates its name and never adds it twice;
// an empty name keeps the name it had; other lines and notes in the file are kept.
func TestAddKnownUpdatesInsteadOfDuplicating(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known-devices.txt")
	os.WriteFile(p, []byte("# my list\n02:00:00:00:00:31 tv\n"), 0o644)
	if err := AddKnown(p, "02-00-00-00-00-20", "phone"); err != nil {
		t.Fatal(err)
	}
	if err := AddKnown(p, "02:00:00:00:00:20", "phone"); err != nil {
		t.Fatal(err)
	}
	if err := AddKnown(p, "02:00:00:00:00:20", "kitchen tablet"); err != nil {
		t.Fatal(err)
	}
	if err := AddKnown(p, "02:00:00:00:00:31", ""); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(p)
	s := string(b)
	if n := strings.Count(s, "02:00:00:00:00:20"); n != 1 {
		t.Errorf("the address must be on the list once, found %d times:\n%s", n, s)
	}
	if n := strings.Count(s, "02:00:00:00:00:31"); n != 1 {
		t.Errorf("an address already listed must not be added again:\n%s", s)
	}
	names := LoadKnownNames(p)
	if names["02:00:00:00:00:20"] != "kitchen tablet" {
		t.Errorf("a new name must replace the old one: %v", names)
	}
	if names["02:00:00:00:00:31"] != "tv" {
		t.Errorf("an empty name must keep the name it had: %v", names)
	}
	if !strings.HasPrefix(s, "# my list\n") {
		t.Errorf("notes in the file must be kept:\n%s", s)
	}
	k, _ := LoadKnown(p)
	if len(k) != 2 {
		t.Errorf("want 2 known devices, got %v", k)
	}
}

// ND-264: the name saved is plain text: cut to 40 characters, control characters removed, so a
// name can never add a second line to the file.
func TestAddKnownCleansTheName(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known-devices.txt")
	if err := AddKnown(p, "02:00:00:00:00:20", "tv\n02:00:00:00:00:99 fake"); err != nil {
		t.Fatal(err)
	}
	k, _ := LoadKnown(p)
	if len(k) != 1 || k["02:00:00:00:00:99"] {
		t.Errorf("a line break in a name must not add a device: %v", k)
	}
	AddKnown(p, "02:00:00:00:00:21", strings.Repeat("x", 80))
	if n := len([]rune(LoadKnownNames(p)["02:00:00:00:00:21"])); n != 40 {
		t.Errorf("a long name must be cut to 40 characters, got %d", n)
	}
	if err := AddKnown(p, "not-an-address", "x"); err == nil {
		t.Error("an address that is not a real hardware address must be refused")
	}
}

// ND-264: each device has one state, mine, not mine or ignored; a new choice replaces the old one
// and undecided takes the device off the list. LoadKnown still gives only the devices that are mine.
func TestKnownStates(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known-devices.txt")
	mac := "02:00:00:00:00:20"
	steps := []KnownState{Mine, NotMine, Ignored, Mine, Ignored}
	for _, st := range steps {
		if err := SetKnown(p, mac, st, "tv"); err != nil {
			t.Fatal(err)
		}
		got := LoadKnownStates(p)[mac]
		if got.State != st {
			t.Errorf("after %s the state is %q", st, got.State)
		}
		b, _ := os.ReadFile(p)
		if n := strings.Count(string(b), mac); n != 1 {
			t.Errorf("after %s the address is in the file %d times", st, n)
		}
		k, _ := LoadKnown(p)
		if k[mac] != (st == Mine) {
			t.Errorf("after %s LoadKnown says %v; only mine is known", st, k[mac])
		}
	}
	if err := SetKnown(p, mac, Undecided, ""); err != nil {
		t.Fatal(err)
	}
	if _, ok := LoadKnownStates(p)[mac]; ok {
		t.Error("undecided must take the device off the list")
	}
	if err := SetKnown(p, mac, KnownState("owner"), ""); err == nil {
		t.Error("an unknown state must be refused")
	}
}

// ND-264: a list written by an earlier version (address and name on each line) still loads, and
// every device on it is mine.
func TestOldKnownListStillLoads(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known-devices.txt")
	os.WriteFile(p, []byte("# devices\r\n02-00-00-00-00-20 my phone\r\n02:00:00:00:00:31\r\n"), 0o644)
	k, err := LoadKnown(p)
	if err != nil || len(k) != 2 || !k["02:00:00:00:00:20"] || !k["02:00:00:00:00:31"] {
		t.Fatalf("old list: %v %v", k, err)
	}
	st := LoadKnownStates(p)
	if st["02:00:00:00:00:20"] != (KnownEntry{State: Mine, Name: "my phone"}) || st["02:00:00:00:00:31"].State != Mine {
		t.Errorf("old lines are mine: %v", st)
	}
}

// ND-265: warnings about new devices are on unless the person turned them off; a settings file
// with no such key means on.
func TestNewDeviceWarningsDefaultOn(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "config.json")
	c, _ := LoadConfig(p)
	if !c.NewDeviceWarnings() {
		t.Error("no settings file means warnings on")
	}
	os.WriteFile(p, []byte(`{"plan_mbps":300}`), 0o644)
	c, _ = LoadConfig(p)
	if !c.NewDeviceWarnings() {
		t.Error("a settings file without the key means warnings on")
	}
	off := false
	c.WarnNewDevices = &off
	if err := SaveConfig(p, c); err != nil {
		t.Fatal(err)
	}
	c, _ = LoadConfig(p)
	if c.NewDeviceWarnings() || c.PlanMbps != 300 {
		t.Errorf("turned off must stay off and keep the rest: %+v", c)
	}
}

// ND-264: a name that starts like a tag never changes the choice: every line carries its own tag,
// a name never starts with "[", and old untagged lines still mean mine.
func TestNameThatLooksLikeATagStaysMine(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known-devices.txt")
	if err := SetKnown(p, "02:00:00:00:00:20", Mine, "[ignored] my phone"); err != nil {
		t.Fatal(err)
	}
	if err := AddKnown(p, "02:00:00:00:00:21", "[not-mine] x"); err != nil {
		t.Fatal(err)
	}
	st := LoadKnownStates(p)
	for _, m := range []string{"02:00:00:00:00:20", "02:00:00:00:00:21"} {
		if st[m].State != Mine || strings.HasPrefix(st[m].Name, "[") {
			t.Errorf("%s must stay mine with a name not starting with [: %+v", m, st[m])
		}
	}
	k, _ := LoadKnown(p)
	if !k["02:00:00:00:00:20"] || !k["02:00:00:00:00:21"] {
		t.Errorf("known list must keep both: %v", k)
	}
	b, _ := os.ReadFile(p)
	if !strings.Contains(string(b), "02:00:00:00:00:20 [mine] ") {
		t.Errorf("a mine line must carry its own tag:\n%s", b)
	}
}

// ND-264: only the first word after the address is a tag, read the same way every time.
func TestTwoTagsReadTheSameEveryTime(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known-devices.txt")
	os.WriteFile(p, []byte("02:00:00:00:00:20 [ignored] [not-mine] tv\n02:00:00:00:00:21 [not-mine] [ignored]\n"), 0o644)
	for i := 0; i < 50; i++ {
		st := LoadKnownStates(p)
		if st["02:00:00:00:00:20"] != (KnownEntry{State: Ignored, Name: "[not-mine] tv"}) || st["02:00:00:00:00:21"].State != NotMine {
			t.Fatalf("run %d read %v", i, st)
		}
	}
}

// ND-264: several choices saved at once are all saved, or none when one is bad.
func TestSetKnownManyIsAllOrNothing(t *testing.T) {
	p := filepath.Join(t.TempDir(), "known-devices.txt")
	AddKnown(p, "02:00:00:00:00:20", "tv")
	before, _ := os.ReadFile(p)
	err := SetKnownMany(p, []KnownChange{{MAC: "02:00:00:00:00:21", State: Mine}, {MAC: "bad", State: Mine}})
	if err == nil {
		t.Fatal("a bad address in the batch must refuse the whole batch")
	}
	after, _ := os.ReadFile(p)
	if string(before) != string(after) {
		t.Errorf("a refused batch must change nothing:\n%s", after)
	}
	if err := SetKnownMany(p, []KnownChange{{MAC: "02:00:00:00:00:21", State: Mine, Name: "a"}, {MAC: "02:00:00:00:00:22", State: Ignored}}); err != nil {
		t.Fatal(err)
	}
	if st := LoadKnownStates(p); len(st) != 3 || st["02:00:00:00:00:22"].State != Ignored {
		t.Errorf("a good batch saves every change: %v", st)
	}
}

// ND-264 and ND-265: saves running at the same time never leave a half-written file, and no
// change to a different device is lost.
func TestSavesAtTheSameTimeAreSafe(t *testing.T) {
	dir := t.TempDir()
	kp, cp := filepath.Join(dir, "known-devices.txt"), filepath.Join(dir, "config.json")
	const n = 24
	stop := make(chan struct{})
	bad := make(chan string, 1)
	go func() { // a reader loading all the time
		last := 0
		for {
			select {
			case <-stop:
				return
			default:
			}
			if b, err := os.ReadFile(kp); err == nil && len(b) > 0 {
				if b[len(b)-1] != '\n' {
					select {
					case bad <- "known list read half written":
					default:
					}
				}
				c := len(LoadKnownStates(kp))
				if c < last {
					select {
					case bad <- "known list lost a device while saving":
					default:
					}
				}
				last = c
			}
			if b, err := os.ReadFile(cp); err == nil && len(b) > 0 && !json.Valid(b) {
				select {
				case bad <- "settings file read half written":
				default:
				}
			}
		}
	}()
	var wg sync.WaitGroup
	for i := 0; i < n; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			if err := SetKnown(kp, fmt.Sprintf("02:00:00:00:01:%02x", i), Mine, "d"); err != nil {
				t.Error(err)
			}
		}(i)
		go func(i int) {
			defer wg.Done()
			on := i%2 == 0
			if err := SaveConfig(cp, Config{PlanMbps: float64(i), WarnNewDevices: &on}); err != nil {
				t.Error(err)
			}
		}(i)
	}
	wg.Wait()
	close(stop)
	select {
	case msg := <-bad:
		t.Error(msg)
	default:
	}
	if got := len(LoadKnownStates(kp)); got != n {
		t.Errorf("want %d devices after the saves, got %d", n, got)
	}
	if _, err := LoadConfig(cp); err != nil {
		t.Errorf("settings must read whole after the saves: %v", err)
	}
	if left, _ := filepath.Glob(filepath.Join(dir, "*.tmp*")); len(left) != 0 {
		t.Errorf("no temporary files may be left: %v", left)
	}
}
