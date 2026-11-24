// Package store keeps the tool's own small files on the local machine.
// Nothing here leaves the machine.
package store

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// InstalledMarker is the small file the installer writes beside the program.
// Its presence means "installed by setup", so results go to the shared folder.
const InstalledMarker = "installed.marker"

// DefaultDir is the folder for the tool's files. An installed copy saves in
// ProgramData for the whole computer; a plain unzipped copy saves per person.
func DefaultDir() string {
	exeDir := "."
	if exe, err := os.Executable(); err == nil {
		exeDir = filepath.Dir(exe)
	}
	userBase, _ := os.UserConfigDir()
	return dirFor(exeDir, os.Getenv("ProgramData"), userBase)
}

// dirFor picks the folder from where the program sits and the two base folders.
func dirFor(exeDir, programData, userBase string) string {
	if programData != "" {
		if _, err := os.Stat(filepath.Join(exeDir, InstalledMarker)); err == nil {
			return filepath.Join(programData, "NetworkDoctor")
		}
	}
	if userBase == "" {
		userBase = "."
	}
	return filepath.Join(userBase, "NetworkDoctor")
}

// Row is one weekly speed record.
type Row struct {
	Time                   time.Time
	Down, Up, PingMs, Plan float64
}

var weeklyHeader = []string{"time", "download_mbps", "upload_mbps", "ping_ms", "plan_mbps"}

func f(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// AppendWeekly adds a row, writing the header first if the file is new.
func AppendWeekly(path string, r Row) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	_, statErr := os.Stat(path)
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	w := csv.NewWriter(fh)
	if errors.Is(statErr, os.ErrNotExist) {
		w.Write(weeklyHeader)
	}
	w.Write([]string{r.Time.Format(time.RFC3339), f(r.Down), f(r.Up), f(r.PingMs), f(r.Plan)})
	w.Flush()
	return w.Error()
}

// ReadWeekly reads every row.
func ReadWeekly(path string) ([]Row, error) {
	fh, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer fh.Close()
	recs, err := csv.NewReader(fh).ReadAll()
	if err != nil {
		return nil, err
	}
	var rows []Row
	for i, rec := range recs {
		if i == 0 || len(rec) < 5 {
			continue
		}
		t, e := time.Parse(time.RFC3339, rec[0])
		if e != nil {
			continue
		}
		var r Row
		r.Time = t
		r.Down, _ = strconv.ParseFloat(rec[1], 64)
		r.Up, _ = strconv.ParseFloat(rec[2], 64)
		r.PingMs, _ = strconv.ParseFloat(rec[3], 64)
		r.Plan, _ = strconv.ParseFloat(rec[4], 64)
		rows = append(rows, r)
	}
	return rows, nil
}

// KnownState is the person's choice for one device on the known list (ND-264).
type KnownState string

// The choices. Undecided is never written: it takes the device off the list.
const (
	Mine      KnownState = "mine"
	NotMine   KnownState = "not-mine"
	Ignored   KnownState = "ignored"
	Undecided KnownState = "undecided"
)

// KnownEntry is one device's choice and the name saved with it.
type KnownEntry struct {
	State KnownState
	Name  string
}

// The known list is one device per line: the address, a tag for the choice, then the name, such as
// "02:00:00:00:00:20 [ignored] printer". Every line written carries its tag, mine too. A line with
// no tag (written by an earlier version) means mine. Only the first word after the address can be
// a tag, and a saved name never starts with "[", so a name can never be taken for a choice.
var stateTags = map[KnownState]string{Mine: "[mine]", NotMine: "[not-mine]", Ignored: "[ignored]"}

// tagStates is the same table the other way round, for reading.
var tagStates = map[string]KnownState{"[mine]": Mine, "[not-mine]": NotMine, "[ignored]": Ignored}

// parseKnownLine reads one line of the known list; ok is false for a blank line or a note.
func parseKnownLine(l string) (mac string, e KnownEntry, ok bool) {
	l = strings.TrimSpace(l)
	if l == "" || strings.HasPrefix(l, "#") {
		return "", e, false
	}
	first := strings.Fields(l)[0]
	rest := strings.TrimSpace(l[len(first):])
	e.State = Mine
	if f := strings.Fields(rest); len(f) > 0 {
		if st, tagged := tagStates[strings.ToLower(f[0])]; tagged {
			e.State, rest = st, strings.TrimSpace(rest[len(f[0]):])
		}
	}
	e.Name = rest
	return core.NormalizeMAC(first), e, true
}

// cleanKnownName is core.CleanName without a leading "[", so the name cannot look like a tag.
func cleanKnownName(s string) string {
	s = core.CleanName(s)
	if !strings.HasPrefix(s, "[") {
		return s
	}
	// "[mine] x" keeps its words as "(mine) x": the bracket that opens it, and the one that closes
	// it within the first word, become round ones.
	first := strings.Fields(s)[0]
	fixed := "(" + strings.Replace(first[1:], "]", ")", 1)
	return fixed + s[len(first):]
}

func knownLines(path string) ([]string, error) {
	b, err := readSoon(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	return splitLines(b), nil
}

// utf8BOM is the byte-order mark some editors put at the start of a text file.
var utf8BOM = []byte{0xEF, 0xBB, 0xBF}

// splitLines splits a file into lines, with Windows line ends taken off and no empty last line.
func splitLines(b []byte) []string {
	b = bytes.TrimPrefix(b, utf8BOM) // a byte-order mark some editors add
	s := strings.TrimRight(strings.ReplaceAll(string(b), "\r", ""), "\n")
	if s == "" {
		return nil
	}
	return strings.Split(s, "\n")
}

// LoadKnownStates reads every device on the known list with its choice and name. A device listed
// twice (an earlier version could do that) keeps its first line.
func LoadKnownStates(path string) map[string]KnownEntry {
	out := map[string]KnownEntry{}
	lines, _ := knownLines(path)
	for _, l := range lines {
		if mac, e, ok := parseKnownLine(l); ok {
			if _, seen := out[mac]; !seen {
				out[mac] = e
			}
		}
	}
	return out
}

// LoadKnownNames gives the name saved for each device on the known list, where there is one.
func LoadKnownNames(path string) map[string]string {
	out := map[string]string{}
	for mac, e := range LoadKnownStates(path) {
		if e.Name != "" {
			out[mac] = e.Name
		}
	}
	return out
}

// LoadKnown reads the list of devices the user has marked as known (mine).
func LoadKnown(path string) (map[string]bool, error) {
	known := map[string]bool{}
	if _, err := knownLines(path); err != nil {
		return known, err
	}
	for mac, e := range LoadKnownStates(path) {
		if e.State == Mine {
			known[mac] = true
		}
	}
	return known, nil
}

// KnownChange is one device's new choice, for SetKnownMany.
type KnownChange struct {
	MAC   string
	State KnownState
	Name  string
}

// SetKnown records one device's choice on the known list (see SetKnownMany).
func SetKnown(path, mac string, state KnownState, name string) error {
	return SetKnownMany(path, []KnownChange{{MAC: mac, State: state, Name: name}})
}

// SetKnownMany records several choices with one write: all of them, or none when one is not a real
// hardware address or not a choice. Each device is listed once: a new choice replaces its line, an
// empty name keeps the name it had, and Undecided takes it off the list. Notes and other lines are
// kept. Names are kept as plain text. Saves are safe between processes (see saveFile).
func SetKnownMany(path string, changes []KnownChange) error {
	want := map[string]KnownChange{}
	var order []string
	for _, c := range changes {
		if !core.ValidMAC(strings.TrimSpace(c.MAC)) {
			return errors.New("not a hardware address: " + c.MAC)
		}
		if _, tagged := stateTags[c.State]; !tagged && c.State != Undecided {
			return errors.New("not a choice: " + string(c.State))
		}
		c.MAC, c.Name = core.NormalizeMAC(c.MAC), cleanKnownName(c.Name)
		if _, seen := want[c.MAC]; !seen {
			order = append(order, c.MAC)
		}
		want[c.MAC] = c
	}
	line := func(c KnownChange, name string) string {
		l := c.MAC + " " + stateTags[c.State]
		if name != "" {
			l += " " + name
		}
		return l
	}
	return saveFile(path, func(old []byte) ([]byte, error) {
		lines := splitLines(old)
		var out []string
		done := map[string]bool{}
		for _, l := range lines {
			m, e, ok := parseKnownLine(l)
			c, change := want[m]
			if !ok || !change {
				out = append(out, l)
				continue
			}
			if done[m] || c.State == Undecided {
				done[m] = true
				continue // a second line for the same device, or the device taken off
			}
			n := c.Name
			if n == "" {
				n = cleanKnownName(e.Name)
			}
			out, done[m] = append(out, line(c, n)), true
		}
		for _, m := range order {
			if c := want[m]; !done[m] && c.State != Undecided {
				out = append(out, line(c, c.Name))
			}
		}
		if len(out) == 0 {
			return []byte{}, nil
		}
		return []byte(strings.Join(out, "\n") + "\n"), nil
	})
}

// LoadKnownFrom reads a list of trusted devices kept by another program: a JSON object whose
// keys are hardware addresses (dashes or colons, any case), such as another program might keep.
// Only the keys are used. An empty path or a missing file gives no one and no error; a file
// that cannot be read or understood, such as one half written, is an error.
func LoadKnownFrom(path string) (map[string]bool, error) {
	known := map[string]bool{}
	if path == "" {
		return known, nil
	}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return known, nil
		}
		return known, err
	}
	b = bytes.TrimPrefix(b, []byte("\xEF\xBB\xBF"))
	var keys map[string]json.RawMessage
	if err := json.Unmarshal(b, &keys); err != nil {
		return known, err
	}
	if keys == nil {
		return known, errors.New("the file does not hold a list of devices")
	}
	for k := range keys {
		if m := core.NormalizeMAC(k); m != "" {
			known[m] = true
		}
	}
	return known, nil
}

// AddKnown marks a device as known (mine). Marking it again updates its name, never adds it twice.
func AddKnown(path, mac, name string) error { return SetKnown(path, mac, Mine, name) }

// Config is the user's settings.
type Config struct {
	PlanMbps   float64      `json:"plan_mbps"`
	Name       string       `json:"name"`
	WeeklyDay  time.Weekday `json:"weekly_day"`
	WeeklyHour int          `json:"weekly_hour"`
	QuietFrom  int          `json:"quiet_from_hour"`
	QuietTo    int          `json:"quiet_to_hour"`
	Sinks      []string     `json:"alert_sinks"`
	AlertsOff  []string     `json:"alerts_off"`
	Allow      []string     `json:"allow"`
	KnownFrom  string       `json:"known_from,omitempty"`
	// WarnNewDevices is the page's "Warn me about new devices" switch; missing means on (ND-265).
	WarnNewDevices *bool `json:"warn_new_devices,omitempty"`
	// SinksSaved is true once the person has chosen where alerts go. Until then the
	// places are a default, and the alerts part of the installer may turn on more.
	SinksSaved bool `json:"-"`
}

// NewDeviceWarnings is true unless the person turned warnings about new devices off.
func (c Config) NewDeviceWarnings() bool { return c.WarnNewDevices == nil || *c.WarnNewDevices }

// LoadConfig reads the settings file, or the defaults when there is none.
func LoadConfig(path string) (Config, error) {
	b, err := readSoon(path) // tried again briefly while a save puts the new file in place
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return parseConfig(nil)
		}
		c, _ := parseConfig(nil)
		return c, err
	}
	return parseConfig(b)
}

// parseConfig reads the settings from the file's bytes; no bytes gives the defaults.
func parseConfig(b []byte) (Config, error) {
	c := Config{WeeklyDay: time.Monday, WeeklyHour: 9, Sinks: []string{"console"}}
	if len(b) == 0 {
		return c, nil
	}
	err := json.Unmarshal(b, &c)
	var keys map[string]json.RawMessage
	if json.Unmarshal(b, &keys) == nil {
		// An earlier version always wrote the console-only default, so that list alone is no choice.
		// A list with anything else in it, or one saved with the chosen mark, is the person's own.
		var chosen bool
		_ = json.Unmarshal(keys["sinks_chosen"], &chosen)
		_, has := keys["alert_sinks"]
		onlyConsole := len(c.Sinks) == 1 && c.Sinks[0] == "console"
		c.SinksSaved = has && (chosen || (len(c.Sinks) > 0 && !onlyConsole))
	}
	return c, err
}

// configBytes is the settings as written to the file.
func configBytes(c Config) []byte {
	b, _ := json.MarshalIndent(c, "", "  ")
	var keys map[string]json.RawMessage
	if json.Unmarshal(b, &keys) == nil {
		if c.SinksSaved {
			keys["sinks_chosen"] = json.RawMessage("true") // so a chosen console-only list is not mistaken for the old default
		} else {
			delete(keys, "alert_sinks") // no choice of places yet: write none, so a default can still apply later
		}
		b, _ = json.MarshalIndent(keys, "", "  ")
	}
	return b
}

// SaveConfig writes the settings file. A whole new file is put in place, so another program never
// reads it half written.
func SaveConfig(path string, c Config) error {
	return saveFile(path, func([]byte) ([]byte, error) { return configBytes(c), nil })
}

// UpdateConfig changes the settings file as it is at the moment of saving: change runs under the
// file's lock on the settings just read, so a change made meanwhile by another program is not lost.
// An error from change saves nothing.
func UpdateConfig(path string, change func(*Config) error) error {
	return saveFile(path, func(old []byte) ([]byte, error) {
		c, err := parseConfig(old)
		if err != nil {
			return nil, err
		}
		if err := change(&c); err != nil {
			return nil, err
		}
		return configBytes(c), nil
	})
}
