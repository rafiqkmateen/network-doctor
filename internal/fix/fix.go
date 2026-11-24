// Package fix is Stage 2: small fixes that only run after the user says yes to
// that one fix. The check itself never calls this package.
package fix

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// ProxyState is the per-user web proxy switch and its address.
type ProxyState struct {
	Enabled bool   `json:"enabled"`
	Server  string `json:"server"`
	// Mode is the desktop's own proxy mode where the system has one (Linux: "none", "manual" or
	// "auto"), so an undo puts back exactly that mode. Empty where there is none (Windows, Mac).
	Mode string `json:"mode,omitempty"`
}

// leftBy is the proxy state the fix leaves: off, with the same server, and the mode "none" where
// the system has a mode.
func leftBy(s ProxyState) ProxyState {
	l := ProxyState{Enabled: false, Server: s.Server}
	if s.Mode != "" {
		l.Mode = "none"
	}
	return l
}

// System is the computer the fixes act on. Tests use a simulated one.
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

// ToolChecker is optional. A system that has it says, before anything is read or changed, why a
// fix cannot work there (a program it needs is missing, or the fix is never made there), or ""
// when it can (ND-307). Such a fix is refused in plain words and never offered.
type ToolChecker interface {
	MissingTool(name string) string
}

// missing refuses a known fix whose programs the system does not have.
func (m *Manager) missing(name string) error {
	if tc, ok := m.sys.(ToolChecker); ok && isFix(name) {
		if why := tc.MissingTool(name); why != "" {
			return &RefusedError{Name: name, Reason: why}
		}
	}
	return nil
}

func isFix(name string) bool {
	return name == nameCache || name == proxy || name == power || isRoundTwo(name)
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
	// ChangedByHand marks a refusal because the person changed the setting by hand since the fix,
	// read and seen on the very adapter or setting the fix changed. Removal settles such a fix
	// (ND-316); the mark, never the words, decides that.
	ChangedByHand bool
}

// changedByHand says whether err is a refusal marked as a setting changed by hand since the fix.
func changedByHand(err error) bool {
	var r *RefusedError
	return errors.As(err, &r) && r.ChangedByHand
}

func (e *RefusedError) Error() string { return "refused: " + e.Name + ". " + e.Reason }

// Info is one line of the fix list.
type Info struct {
	Name, Title, Now string
	NeedsAdmin       bool
	// Undoable is true when `fix undo` can put the fix back.
	Undoable bool
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
	sys     System
	dir     string
	opts    Options
	account func() string // the account this runs as, recorded beside each note (ND-316)
}

// NewManager keeps its saved values and its log in dir.
func NewManager(sys System, dir string, o Options) *Manager {
	return &Manager{sys: sys, dir: dir, opts: o, account: CurrentAccount}
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
	if s.Server == "" {
		return "on, with no server set" // a Linux automatic setup with no address, say
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
	for _, n := range []string{nameCache, proxy, power, publicDNS, renewAddress, resetAdapter} {
		pl, err := m.Preview(n)
		now := pl.Before
		if err != nil {
			now = "could not read: " + err.Error()
		}
		out = append(out, Info{Name: n, Title: title(n), Now: now, NeedsAdmin: m.NeedsAdmin(n), Undoable: undoable(n)})
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
	return roundTwoTitle(n)
}

// undoable is true for the fixes "fix undo" can put back.
func undoable(n string) bool { return n == proxy || n == power || n == publicDNS }

// Undoable is true for the fixes "fix undo" can put back.
func Undoable(name string) bool { return undoable(name) }

// Title is the plain name of a fix.
func Title(name string) string { return title(name) }

// AdminAsker is optional: a system that has it says itself whether a fix may ask for
// administrator rights there (on a Mac networksetup may; on Linux NetworkManager may).
type AdminAsker interface {
	MayAskAdmin(name string) bool
}

// NeedsAdmin is true when the fix may ask for administrator rights on this computer: the system's
// own answer when it gives one, else the list below.
func (m *Manager) NeedsAdmin(name string) bool {
	if a, ok := m.sys.(AdminAsker); ok {
		return a.MayAskAdmin(name)
	}
	return NeedsAdmin(name)
}

// NeedsAdmin is true for the fixes that ask for administrator rights at the yes (on Windows).
func NeedsAdmin(name string) bool {
	return name == power || isRoundTwo(name)
}

// Preview shows the setting now and what it will be. It changes nothing.
func (m *Manager) Preview(name string) (Plan, error) {
	if err := m.missing(name); err != nil {
		return Plan{}, err
	}
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
	if isRoundTwo(name) {
		return m.previewRoundTwo(name)
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
	if name != nameCache && name != proxy && name != power && !isRoundTwo(name) {
		return Result{}, ErrUnknown
	}
	if err := m.missing(name); err != nil {
		return Result{}, err
	}
	if !approved {
		return Result{}, ErrNotApproved
	}
	if isRoundTwo(name) {
		x, err := m.extra(name)
		if err != nil {
			return Result{}, err
		}
		if !m.allowed() {
			return Result{}, ErrServerOff
		}
		pl, err := m.applyRoundTwo(name, x)
		if err != nil {
			return Result{}, err
		}
		m.log("apply", name, pl.Before, pl.After)
		return Result{Name: name, Before: pl.Before, After: pl.After}, nil
	}
	if !m.allowed() {
		return Result{}, ErrServerOff
	}
	pl, err := m.Preview(name)
	if err != nil {
		return Result{}, err
	}
	if name == proxy && pl.Before == "off" {
		// Saving "off" now would replace the only saved old value, so the fix is refused.
		return Result{}, &RefusedError{Name: name, Reason: "The web proxy is already off, so there is nothing to turn off. Nothing was changed."}
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
				err = m.sys.SetProxy(leftBy(s))
			}
		}
	case power:
		var v string
		if v, err = m.sys.PowerSaving(); err == nil {
			if err = m.save(name, v); err == nil {
				err = m.sys.SetPowerSaving("off")
				if err == nil {
					// Read the setting back: a yes from Windows is not proof (ND-243).
					if now, rerr := m.sys.PowerSaving(); rerr != nil || now != "off" {
						err = fmt.Errorf("Windows said yes, but power saving is not off when read back")
						if rerr == nil && now == v {
							s := m.loadSaved()
							delete(s, name)
							delete(s, name+accountSuffix)
							_ = m.writeSaved(s)
						}
					}
				}
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
	if err := m.missing(name); err != nil && name != nameCache && name != renewAddress && name != resetAdapter {
		return err
	}
	if name == nameCache {
		return ErrNothingToUndo
	}
	if name == renewAddress || name == resetAdapter {
		return &RefusedError{Name: name, Reason: noUndoReason}
	}
	if name == publicDNS {
		return m.undoPublicDNS()
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
	still, _, _, err := m.undoState(name, old)
	if err != nil {
		return err
	}
	if !still {
		now, left, err := m.changedWords(name, old)
		if err != nil {
			return err
		}
		return &RefusedError{Name: name, ChangedByHand: true, Reason: "The setting was changed since the fix. It is now " + now + ", but the fix left it " + left + ". Putting back the old value could undo that change, so nothing was changed."}
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
		if now, rerr := m.sys.PowerSaving(); rerr != nil || now != old {
			return fmt.Errorf("not undone: Windows said yes, but power saving is not %s when read back", old)
		}
		restored = powerText(old)
	}
	delete(saved, name)
	delete(saved, name+accountSuffix)
	if err := m.writeSaved(saved); err != nil {
		return err
	}
	m.log("undo", name, cur.After, restored)
	return nil
}

// Reasons a recorded fix was not put back by UndoAll.
const (
	ReasonAdmin  = "admin"  // it may need administrator rights, and the caller asked not to try
	ReasonFailed = "failed" // the computer did not make the change, or the setting could not be read
	ReasonNotes  = "notes"  // the undo notes themselves could not be read
	// ReasonAccount: the setting looked changed, but the note was written by another account, or
	// carries no account, so it cannot be told apart from that account's own setting.
	ReasonAccount = "account"
)

// UndoFailure is one recorded fix that UndoAll did not put back. Err is the system's own error,
// for a log only.
type UndoFailure struct {
	Name, Reason string
	Err          error
}

// UndoAll is what removal runs first (ND-316): it puts back, one by one with Undo, every fix
// recorded in the undo notes. skip, when not nil, names a fix not to try (one that may ask for
// administrator rights where the caller must never ask); it counts as not put back. A fix whose
// setting the person changed by hand since is settled: the person's setting wins, its note is
// dropped and its name is returned in settled, not as a failure (Undo itself still refuses it).
// With no notes, or empty ones, nothing is read or changed. Notes that cannot be read count as not
// put back and are left as they are.
func (m *Manager) UndoAll(skip func(name string) bool) (failures []UndoFailure, settled []string) {
	if m.dir == "" {
		return nil, nil
	}
	b, err := os.ReadFile(m.savedPath())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	saved := map[string]string{}
	if err == nil {
		err = json.Unmarshal(b, &saved)
	}
	if err != nil {
		return []UndoFailure{{Reason: ReasonNotes, Err: err}}, nil
	}
	accounts := map[string]string{}
	for k, v := range saved {
		if isAccountNote(k) {
			accounts[strings.TrimSuffix(k, accountSuffix)] = v
			delete(saved, k)
		}
	}
	me := ""
	if m.account != nil {
		me = m.account()
	}
	var names []string
	for _, n := range []string{proxy, power, publicDNS} {
		if _, ok := saved[n]; ok {
			names = append(names, n)
			delete(saved, n)
		}
	}
	for n := range saved {
		names = append(names, n) // a note of no known fix: Undo refuses it, so it is reported
	}
	for _, n := range names {
		if skip != nil && skip(n) {
			failures = append(failures, UndoFailure{Name: n, Reason: ReasonAdmin})
			continue
		}
		err := m.Undo(n)
		if err == nil || errors.Is(err, ErrNothingToUndo) {
			continue
		}
		if changedByHand(err) {
			// Settled only when the note was written by the account this runs as: another
			// account's setting, or an old note with no account, cannot be told apart from it.
			if rec := accounts[n]; rec == "" || me == "" || rec != me {
				failures = append(failures, UndoFailure{Name: n, Reason: ReasonAccount, Err: err})
				continue
			}
			if derr := m.dropSaved(n); derr != nil {
				failures = append(failures, UndoFailure{Name: n, Reason: ReasonFailed, Err: derr})
				continue
			}
			m.log("settled", n, "changed by hand since the fix", "left as it is")
			settled = append(settled, n)
			continue
		}
		failures = append(failures, UndoFailure{Name: n, Reason: ReasonFailed, Err: err})
	}
	return failures, settled
}

// dropSaved removes one fix's note from the undo notes. When the notes cannot be read again it
// writes nothing, so the other fixes' notes are never wiped.
func (m *Manager) dropSaved(name string) error {
	b, err := os.ReadFile(m.savedPath())
	if err != nil {
		return err
	}
	s := map[string]string{}
	if err := json.Unmarshal(b, &s); err != nil {
		return err
	}
	delete(s, name)
	delete(s, name+accountSuffix)
	return m.writeSaved(s)
}

// UndoTitle is the plain name of putting a fix back.
func UndoTitle(name string) string { return undoTitle(name) }

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

// save keeps a fix's old value and, beside it, the account that applied it (ND-316).
func (m *Manager) save(name, val string) error {
	s := m.loadSaved()
	s[name] = val
	if m.account != nil {
		if a := m.account(); a != "" {
			s[name+accountSuffix] = a
		}
	}
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

// UndoInfo is one fix the program can put back: the setting now (what the fix left) and what
// Undo would put back.
type UndoInfo struct {
	Name, Title, Now, After string
	NeedsAdmin              bool
}

// undoTitle is the plain name of putting a fix back.
func undoTitle(n string) string {
	switch n {
	case proxy:
		return "Turn the web proxy back on"
	case power:
		return "Turn power saving on the network adapter back on"
	case publicDNS:
		return "Put back the name servers from before"
	}
	return n
}

// proxyDetail names the proxy server even when the proxy is off, so a changed server shows.
func proxyDetail(s ProxyState) string {
	if s.Enabled {
		return "on, using " + s.Server
	}
	if s.Server == "" {
		return "off, with no server set"
	}
	return "off, with the server " + s.Server + " still set"
}

// changedWords gives, for a refused undo, the setting now and what the fix left, in words that
// show the difference. It reads the setting and changes nothing.
func (m *Manager) changedWords(name, old string) (now, left string, err error) {
	switch name {
	case proxy:
		var s ProxyState
		if err = json.Unmarshal([]byte(old), &s); err != nil {
			return "", "", fmt.Errorf("the saved value is damaged: %w", err)
		}
		cur, rerr := m.sys.Proxy()
		if rerr != nil {
			return "", "", rerr
		}
		return proxyDetail(cur), proxyDetail(leftBy(s)), nil
	case power:
		cur, rerr := m.sys.PowerSaving()
		if rerr != nil {
			return "", "", rerr
		}
		return powerText(cur), powerText("off"), nil
	}
	return "", "", ErrUnknown
}

// undoState says, for one saved old value, what the fix left, whether the setting today still is
// exactly that, and the words for now and for after an undo. It reads the setting and changes nothing.
func (m *Manager) undoState(name, old string) (still bool, now, after string, err error) {
	switch name {
	case proxy:
		var s ProxyState
		if err = json.Unmarshal([]byte(old), &s); err != nil {
			return false, "", "", fmt.Errorf("the saved value is damaged: %w", err)
		}
		cur, rerr := m.sys.Proxy()
		if rerr != nil {
			return false, "", "", rerr
		}
		left := leftBy(s)
		return cur == left, proxyText(cur), proxyText(s), nil
	case power:
		cur, rerr := m.sys.PowerSaving()
		if rerr != nil {
			return false, "", "", rerr
		}
		return cur == "off", powerText(cur), powerText(old), nil
	}
	return false, "", "", ErrUnknown
}

// Undos lists the fixes the program can put back right now. Power saving is never listed: that fix
// is never offered, so its undo is not offered either. A fix is listed
// only when its old value is saved, the setting still is exactly what the fix left, and putting it back would change
// something. A setting changed by hand since is left out, so a stale value is never offered. It reads
// only; a stale saved value stays on file (the next fix of that name replaces it). With no folder
// known it reads nothing and lists nothing.
func (m *Manager) Undos() []UndoInfo {
	out := []UndoInfo{}
	if m.dir == "" {
		return out
	}
	saved := m.loadSaved()
	for _, n := range []string{proxy} {
		old, ok := saved[n]
		if !ok {
			continue
		}
		still, now, after, err := m.undoState(n, old)
		if err != nil || !still || now == after {
			continue
		}
		out = append(out, UndoInfo{Name: n, Title: undoTitle(n), Now: now, After: after, NeedsAdmin: m.NeedsAdmin(n)})
	}
	if old, ok := saved[publicDNS]; ok {
		if ok, now, after := m.publicDNSUndo(old); ok {
			out = append(out, UndoInfo{Name: publicDNS, Title: undoTitle(publicDNS), Now: now, After: after, NeedsAdmin: m.NeedsAdmin(publicDNS)})
		}
	}
	return out
}
