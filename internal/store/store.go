// Package store keeps the tool's own small files on the local machine.
// Nothing here leaves the machine.
package store

import (
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

// DefaultDir is the folder for the tool's files.
func DefaultDir() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base = "."
	}
	return filepath.Join(base, "NetworkDoctor")
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

// LoadKnown reads the list of devices the user has marked as known.
func LoadKnown(path string) (map[string]bool, error) {
	known := map[string]bool{}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return known, nil
		}
		return known, err
	}
	for _, l := range strings.Split(strings.ReplaceAll(string(b), "\r", ""), "\n") {
		l = strings.TrimSpace(l)
		if l == "" || strings.HasPrefix(l, "#") {
			continue
		}
		known[core.NormalizeMAC(strings.Fields(l)[0])] = true
	}
	return known, nil
}

// AddKnown marks a device as known.
func AddKnown(path, mac, name string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	fh, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer fh.Close()
	_, err = fh.WriteString(core.NormalizeMAC(mac) + " " + name + "\n")
	return err
}

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
}

// LoadConfig reads the settings file, or the defaults when there is none.
func LoadConfig(path string) (Config, error) {
	c := Config{WeeklyDay: time.Monday, WeeklyHour: 9, Sinks: []string{"console"}}
	b, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return c, nil
		}
		return c, err
	}
	err = json.Unmarshal(b, &c)
	return c, err
}

// SaveConfig writes the settings file.
func SaveConfig(path string, c Config) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile(path, b, 0o644)
}
