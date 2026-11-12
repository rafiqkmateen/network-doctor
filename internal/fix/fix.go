// Package fix is Stage 2: small fixes that only run after the user says yes to
// that one fix. The check itself never calls this package.
package fix

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// ProxyState is the per-user web proxy switch and its address.
type ProxyState struct {
	Enabled bool   `json:"enabled"`
	Server  string `json:"server"`
}

// System is the computer the fixes act on. Tests use a pretend one.
type System interface {
	FlushDNS() error
	Proxy() (ProxyState, error)
	SetProxy(ProxyState) error
	PowerSaving() (string, error) // "on" or "off"
	SetPowerSaving(string) error
	// PowerSavingDropsLink is true when turning it off could drop the link in use.
	PowerSavingDropsLink() bool
	// Elevate asks for administrator rights. It is called only at a yes.
	Elevate() error
}

// Options say how the tool runs. Unattended is a server nobody sits at.
type Options struct {
	Unattended bool
}

var (
	ErrNotApproved   = errors.New("not applied: you have not said yes to this fix")
	ErrServerOff     = errors.New("not applied: fixes are off on this machine. The owner can switch them on for this machine only")
	ErrNothingToUndo = errors.New("nothing to put back for this fix")
	ErrUnknown       = errors.New("there is no fix with that name")
)

// RefusedError is a fix the tool will not make, with the reason in plain words.
type RefusedError struct {
	Name   string
	Reason string
}

func (e *RefusedError) Error() string { return "refused: " + e.Name + ". " + e.Reason }

// Info is one line of the fix list.
type Info struct {
	Name, Title, Now string
	NeedsAdmin       bool
}

// Plan is what you see before you say yes.
type Plan struct {
	Before, After string
}

// Result is what happened after a yes.
type Result struct {
	Name, Before, After string
}

const (
	nameCache = "name-cache"
	proxy     = "proxy"
	power     = "power-saving"
)

// never lists things that are out of bounds for every version, with the reason.
var never = map[string]string{
	"driver":      "This tool never changes drivers. A bad driver change can leave the computer with no network.",
	"firewall":    "This tool never changes security protections.",
	"security":    "This tool never changes security protections.",
	"vpn":         "This tool never changes a VPN. It could cut off the connection you are using.",
	"dns-servers": "A wrong DNS server cuts off websites, so this tool will not change it. Change it by hand if you need to.",
}

// Manager applies and undoes fixes on one computer.
type Manager struct {
	sys  System
	dir  string
	opts Options
}

// NewManager keeps its saved values and its log in dir.
func NewManager(sys System, dir string, o Options) *Manager {
	return &Manager{sys: sys, dir: dir, opts: o}
}

// Dir is the results folder that holds the saved values and the log.
func (m *Manager) Dir() string { return m.dir }

func (m *Manager) savedPath() string   { return filepath.Join(m.dir, "fix-saved.json") }
func (m *Manager) logPath() string     { return filepath.Join(m.dir, "fix-log.txt") }
func (m *Manager) enabledPath() string { return filepath.Join(m.dir, "fix-enabled-here.txt") }

// EnableHere switches fixes on for this machine. Only a user does this.
func (m *Manager) EnableHere() error {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(m.enabledPath(), []byte("Fixes are switched on for this machine.\n"), 0o644)
}

func (m *Manager) allowed() bool {
	if !m.opts.Unattended {
		return true
	}
	_, err := os.Stat(m.enabledPath())
	return err == nil
}

// powerByHand is what to do when the tool must not, or cannot, change the setting.
const powerByHand = "Change it by hand: right-click Start, open Device Manager, open Network adapters, double-click your adapter, open the Power Management tab, clear the box \"Allow the computer to turn off this device to save power\", and press OK."

func proxyText(s ProxyState) string {
	if !s.Enabled {
		return "off"
	}
	return "on, using " + s.Server
}

func powerText(v string) string {
	if v == "on" {
		return "on (the adapter may sleep to save power)"
	}
	return "off (the adapter stays awake)"
}

// List shows every fix and its setting now. It changes nothing.
func (m *Manager) List() []Info {
	var out []Info
	for _, n := range []string{nameCache, proxy, power} {
		pl, err := m.Preview(n)
		now := pl.Before
		if err != nil {
			now = "could not read: " + err.Error()
		}
		out = append(out, Info{Name: n, Title: title(n), Now: now, NeedsAdmin: n == power})
	}
	return out
}

func title(n string) string {
	switch n {
	case nameCache:
		return "Clear the saved list of website addresses"
	case proxy:
		return "Turn off the web proxy"
	case power:
		return "Turn off power saving on the network adapter"
	}
	return n
}

// Preview shows the setting now and what it will be. It changes nothing.
func (m *Manager) Preview(name string) (Plan, error) {
	switch name {
	case nameCache:
		return Plan{Before: "saved website addresses are kept", After: "the list is cleared and the computer rebuilds it"}, nil
	case proxy:
		s, err := m.sys.Proxy()
		if err != nil {
			return Plan{}, err
		}
		return Plan{Before: proxyText(s), After: "off"}, nil
	case power:
		v, err := m.sys.PowerSaving()
		if err != nil {
			// Some drivers have the setting but will not let Windows read it.
			return Plan{Before: "I cannot read this adapter's power saving setting (its driver will not tell me)", After: powerByHand}, nil
		}
		return Plan{Before: powerText(v), After: powerText("off")}, nil
	}
	return Plan{}, m.unknown(name)
}

func (m *Manager) unknown(name string) error {
	if why, ok := never[name]; ok {
		return &RefusedError{Name: name, Reason: why}
	}
	return ErrUnknown
}

// Apply makes one fix, only when approved.
func (m *Manager) Apply(name string, approved bool) (Result, error) {
	if why, ok := never[name]; ok {
		return Result{}, &RefusedError{Name: name, Reason: why}
	}
	if name != nameCache && name != proxy && name != power {
		return Result{}, ErrUnknown
	}
	if !approved {
		return Result{}, ErrNotApproved
	}
	if !m.allowed() {
		return Result{}, ErrServerOff
	}
	pl, err := m.Preview(name)
	if err != nil {
		return Result{}, err
	}
	if name == power {
		if _, rerr := m.sys.PowerSaving(); rerr != nil {
			return Result{}, &RefusedError{Name: name, Reason: "I cannot read this adapter's setting, so I will not change it. " + powerByHand}
		}
		if m.sys.PowerSavingDropsLink() {
			return Result{}, &RefusedError{Name: name, Reason: "Turning this off could drop the connection you are using. " + powerByHand}
		}
	}
	if name == power {
		if err := m.sys.Elevate(); err != nil {
			return Result{}, fmt.Errorf("not applied: administrator rights were not given (%v)", err)
		}
	}
	switch name {
	case nameCache:
		err = m.sys.FlushDNS()
	case proxy:
		var s ProxyState
		if s, err = m.sys.Proxy(); err == nil {
			if err = m.save(name, mustJSON(s)); err == nil {
				err = m.sys.SetProxy(ProxyState{Enabled: false, Server: s.Server})
			}
		}
	case power:
		var v string
		if v, err = m.sys.PowerSaving(); err == nil {
			if err = m.save(name, v); err == nil {
				err = m.sys.SetPowerSaving("off")
			}
		}
	}
	if err != nil {
		return Result{}, fmt.Errorf("could not apply: %w", err)
	}
	m.log("apply", name, pl.Before, pl.After)
	return Result{Name: name, Before: pl.Before, After: pl.After}, nil
}

// Undo puts the saved old value back, with one action.
func (m *Manager) Undo(name string) error {
	if name == nameCache {
		return ErrNothingToUndo
	}
	if name != proxy && name != power {
		return m.unknown(name)
	}
	saved := m.loadSaved()
	old, ok := saved[name]
	if !ok {
		return ErrNothingToUndo
	}
	cur, err := m.Preview(name)
	if err != nil {
		return err
	}
	var restored string
	switch name {
	case proxy:
		var s ProxyState
		if err := json.Unmarshal([]byte(old), &s); err != nil {
			return fmt.Errorf("the saved value is damaged: %w", err)
		}
		if err := m.sys.SetProxy(s); err != nil {
			return err
		}
		restored = proxyText(s)
	case power:
		if err := m.sys.Elevate(); err != nil {
			return fmt.Errorf("not undone: administrator rights were not given (%v)", err)
		}
		if err := m.sys.SetPowerSaving(old); err != nil {
			return err
		}
		restored = powerText(old)
	}
	delete(saved, name)
	if err := m.writeSaved(saved); err != nil {
		return err
	}
	m.log("undo", name, cur.After, restored)
	return nil
}

func mustJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (m *Manager) loadSaved() map[string]string {
	out := map[string]string{}
	if b, err := os.ReadFile(m.savedPath()); err == nil {
		_ = json.Unmarshal(b, &out)
	}
	return out
}

func (m *Manager) writeSaved(s map[string]string) error {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	return os.WriteFile(m.savedPath(), b, 0o644)
}

func (m *Manager) save(name, val string) error {
	s := m.loadSaved()
	s[name] = val
	return m.writeSaved(s)
}

// log appends one line: time, action, setting, old value, new value.
func (m *Manager) log(action, name, old, new string) {
	if err := os.MkdirAll(m.dir, 0o755); err != nil {
		return
	}
	f, err := os.OpenFile(m.logPath(), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	defer f.Close()
	fmt.Fprintf(f, "%s\t%s\t%s\t%s\t%s\n", time.Now().Format("2006-01-02 15:04:05"), action, name, old, new)
}
