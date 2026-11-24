package fix

import (
	"encoding/json"
	"fmt"
	"strings"
)

// The round-two fixes (ND-235 to ND-238, ND-304 to ND-306). Only a system that has ExtraSystem (Windows,
// and Mac and Linux through UnixSystem) can make them; any other refuses them and stays read-only.
const (
	publicDNS    = "public-dns"
	renewAddress = "renew-address"
	resetAdapter = "reset-adapter"
)

// cloudflareDNS is what public-dns sets on the adapter in use.
var cloudflareDNS = []string{"1.1.1.1", "1.0.0.1"}

// noExtraReason is why a system without the round-two abilities refuses those fixes.
const noExtraReason = "This fix does not work on this computer. Here the tool only reads, so nothing was changed."

// noUndoReason is what "fix undo" says for a fix that has no undo.
const noUndoReason = "This fix has no undo. It changed nothing that could be put back."

func isRoundTwo(name string) bool {
	return name == publicDNS || name == renewAddress || name == resetAdapter
}

// extra gives the round-two abilities of the system, or a plain refusal when it has none.
func (m *Manager) extra(name string) (ExtraSystem, error) {
	x, ok := m.sys.(ExtraSystem)
	if !ok {
		return nil, &RefusedError{Name: name, Reason: noExtraReason}
	}
	return x, nil
}

func roundTwoTitle(n string) string {
	switch n {
	case publicDNS:
		return "Look up websites through Cloudflare (1.1.1.1), a third party"
	case renewAddress:
		return "Ask the router for a new address (no undo)"
	case resetAdapter:
		return "Turn the network adapter off and on (no undo)"
	}
	return n
}

// dnsWords says one adapter's name servers in plain words.
func dnsWords(s DNSState) string {
	name := s.Adapter
	if name == "" {
		name = fmt.Sprintf("adapter %d", s.IfIndex)
	}
	if s.Automatic {
		if len(s.Servers) > 0 {
			return name + ": set by the router (" + strings.Join(s.Servers, ", ") + ")"
		}
		return name + ": set by the router"
	}
	if len(s.Servers) == 0 {
		return name + ": none set"
	}
	return name + ": " + strings.Join(s.Servers, ", ")
}

func dnsListWords(in []DNSState) string {
	if len(in) == 0 {
		return "no network adapter in use was found"
	}
	parts := make([]string, len(in))
	for i, s := range in {
		parts[i] = dnsWords(s)
	}
	return strings.Join(parts, "; ")
}

const publicDNSAfter = "1.1.1.1 and 1.0.0.1 (Cloudflare). All website lookups then go to a third party, Cloudflare. The saved list of website addresses is cleared. You can undo it."

// sameDNS is true when got matches want. An adapter set by the router is matched by that alone,
// since the router may hand out other servers after a change.
func sameDNS(want, got DNSState) bool {
	if want.IgnoreAuto != nil && got.IgnoreAuto != nil && *want.IgnoreAuto != *got.IgnoreAuto {
		return false
	}
	if want.Automatic {
		return got.Automatic
	}
	if got.Automatic || len(want.Servers) != len(got.Servers) {
		return false
	}
	for i := range want.Servers {
		if want.Servers[i] != got.Servers[i] {
			return false
		}
	}
	return true
}

// allMatch is true when every adapter in want is found in got by its number and matches.
func allMatch(want, got []DNSState) bool {
	byIndex := map[int]DNSState{}
	for _, s := range got {
		byIndex[s.IfIndex] = s
	}
	for _, w := range want {
		g, ok := byIndex[w.IfIndex]
		if !ok || !sameDNS(w, g) {
			return false
		}
	}
	return true
}

// allPresent is true when every adapter in want is found in got by its number, and want is not
// empty. Where a NetworkManager connection was recorded (Linux), the same connection must be on
// that device too: another connection on it is not the adapter the fix changed (ND-316).
func allPresent(want, got []DNSState) bool {
	byIndex := map[int]DNSState{}
	for _, s := range got {
		byIndex[s.IfIndex] = s
	}
	for _, w := range want {
		g, ok := byIndex[w.IfIndex]
		if !ok || (w.ConnUUID != "" && g.ConnUUID != w.ConnUUID) {
			return false
		}
	}
	return len(want) > 0
}

// cloudflareFor is the state public-dns sets on each of the given adapters.
func cloudflareFor(in []DNSState) []DNSState {
	out := make([]DNSState, len(in))
	for i, s := range in {
		out[i] = DNSState{Adapter: s.Adapter, ConnUUID: s.ConnUUID, IfIndex: s.IfIndex, Servers: append([]string(nil), cloudflareDNS...)}
	}
	return out
}

// noAdapterInUse is the refusal when no adapter carries the connection.
const noAdapterInUse = "No network adapter is carrying the connection right now, so nothing was changed."

// inUse gives the adapter that carries the connection. A list of one adapter from an older
// system that does not mark it counts as that adapter.
func inUse(in []DNSState) (DNSState, bool) {
	for _, s := range in {
		if s.InUse {
			return s, true
		}
	}
	if len(in) == 1 {
		return in[0], true
	}
	return DNSState{}, false
}

// readInUse reads the name servers of the adapter in use, or refuses when there is none.
func (m *Manager) readInUse(x ExtraSystem) (DNSState, error) {
	all, err := x.DNS()
	if err != nil {
		return DNSState{}, err
	}
	s, ok := inUse(all)
	if !ok {
		return DNSState{}, &RefusedError{Name: publicDNS, Reason: noAdapterInUse}
	}
	return s, nil
}

// conditions reads the situation now when the system can (ND-244). ok is false when it cannot.
func (m *Manager) conditions() (c Conditions, has bool, err error) {
	cs, ok := m.sys.(ConditionSystem)
	if !ok {
		return Conditions{}, false, nil
	}
	c, err = cs.Conditions()
	return c, true, err
}

func answers(b bool) string {
	if b {
		return "answers"
	}
	return "does not answer"
}

// nowWords says the situation read now for renew-address and reset-adapter, never fixed text.
func (m *Manager) nowWords(name string) string {
	c, has, err := m.conditions()
	switch {
	case !has:
		return "not read: this computer cannot check this right now"
	case err != nil:
		return "could not be read right now"
	case !c.AdapterUp:
		return "the adapter in use is off"
	case name == renewAddress && c.HasAddress:
		return "the adapter in use has a usable address"
	case name == renewAddress:
		return "the adapter in use has no usable address"
	}
	return "the router " + answers(c.RouterAnswers) + ", and the internet " + answers(c.InternetAnswers)
}

func (m *Manager) previewRoundTwo(name string) (Plan, error) {
	x, err := m.extra(name)
	if err != nil {
		return Plan{}, err
	}
	switch name {
	case publicDNS:
		cur, err := m.readInUse(x)
		if err != nil {
			return Plan{}, err
		}
		return Plan{Before: dnsWords(cur), After: publicDNSAfter}, nil
	case renewAddress:
		return Plan{Before: m.nowWords(name), After: "The computer asks the router for a new address. Nothing is saved, so this has no undo. None is needed."}, nil
	case resetAdapter:
		return Plan{Before: m.nowWords(name), After: "The adapter is turned off and back on. This has no undo. None is needed, because this fix only runs when the line is down."}, nil
	}
	return Plan{}, ErrUnknown
}

// stillNeeded reads the situation again at the press and refuses a fix whose trouble is gone (ND-244).
func (m *Manager) stillNeeded(name string) error {
	c, has, err := m.conditions()
	if !has {
		return nil
	}
	if err != nil {
		return &RefusedError{Name: name, Reason: "I could not read the connection right now, so nothing was changed. Try again in a moment."}
	}
	if name == resetAdapter && (c.RouterAnswers || c.InternetAnswers) {
		return &RefusedError{Name: name, Reason: "The router or the internet answers now, so the trouble is gone. Nothing was changed."}
	}
	if name == renewAddress && c.HasAddress {
		return &RefusedError{Name: name, Reason: "The adapter has a usable address now, so the trouble is gone. Nothing was changed."}
	}
	return nil
}

// applyRoundTwo makes one round-two fix after the yes. The system's abilities were checked.
func (m *Manager) applyRoundTwo(name string, x ExtraSystem) (Plan, error) {
	if err := m.stillNeeded(name); err != nil {
		return Plan{}, err
	}
	pl, err := m.previewRoundTwo(name)
	if err != nil {
		return Plan{}, err
	}
	var cur []DNSState
	if name == publicDNS {
		one, err := m.readInUse(x)
		if err != nil {
			return Plan{}, err
		}
		// Only the adapter in use is saved and set (ND-244).
		cur = []DNSState{one}
		if allMatch(cloudflareFor(cur), cur) {
			// Saving now would replace the first saved servers, so the fix is refused.
			return Plan{}, &RefusedError{Name: name, Reason: "The adapter in use already looks up websites through 1.1.1.1 and 1.0.0.1. Nothing was changed."}
		}
	}
	if err := m.sys.Elevate(); err != nil {
		return Plan{}, fmt.Errorf("not applied: administrator rights were not given (%v)", err)
	}
	switch name {
	case publicDNS:
		if err := m.save(name, mustJSON(cur)); err != nil {
			return Plan{}, fmt.Errorf("could not apply: %w", err)
		}
		want := cloudflareFor(cur)
		err := x.SetDNS(want)
		back, rerr := x.DNS()
		if err == nil && rerr == nil && !allMatch(want, back) {
			err = fmt.Errorf("the computer said yes, but the servers did not change when read back")
		}
		if err == nil && rerr != nil {
			err = fmt.Errorf("the servers could not be read back: %w", rerr)
		}
		if err != nil {
			if rerr == nil && allMatch(cur, back) {
				// Nothing changed, so nothing needs putting back.
				s := m.loadSaved()
				delete(s, name)
				delete(s, name+accountSuffix)
				_ = m.writeSaved(s)
			}
			return Plan{}, fmt.Errorf("could not apply: %w", err)
		}
		_ = m.sys.FlushDNS()
	case renewAddress:
		if err := x.Renew(); err != nil {
			return Plan{}, fmt.Errorf("could not apply: %w", err)
		}
	case resetAdapter:
		if err := x.ResetAdapter(); err != nil {
			return Plan{}, fmt.Errorf("could not apply: %w", err)
		}
	}
	return pl, nil
}

// undoPublicDNS puts back exactly the saved servers, and refuses when they changed since.
func (m *Manager) undoPublicDNS() error {
	x, err := m.extra(publicDNS)
	if err != nil {
		return err
	}
	saved := m.loadSaved()
	old, ok := saved[publicDNS]
	if !ok {
		return ErrNothingToUndo
	}
	var orig []DNSState
	if err := json.Unmarshal([]byte(old), &orig); err != nil {
		return fmt.Errorf("the saved value is damaged: %w", err)
	}
	cur, err := x.DNS()
	if err != nil {
		return err
	}
	if !allMatch(cloudflareFor(orig), cur) {
		// Changed by hand only when every adapter the fix changed is still listed; one that is gone
		// (offline, or another adapter in use) cannot be seen, so it is not settled (ND-316).
		return &RefusedError{Name: publicDNS, ChangedByHand: allPresent(orig, cur), Reason: "The name servers were changed since the fix. They are now " + dnsListWords(cur) + ", but the fix left them on 1.1.1.1 and 1.0.0.1. Putting back the old servers could undo that change, so nothing was changed."}
	}
	if err := m.sys.Elevate(); err != nil {
		return fmt.Errorf("not undone: administrator rights were not given (%v)", err)
	}
	if err := x.SetDNS(orig); err != nil {
		return fmt.Errorf("not undone: %w", err)
	}
	back, err := x.DNS()
	if err != nil {
		return fmt.Errorf("not undone: the servers could not be read back: %w", err)
	}
	if !allMatch(orig, back) {
		return fmt.Errorf("not undone: the computer said yes, but the old servers are not there when read back")
	}
	delete(saved, publicDNS)
	delete(saved, publicDNS+accountSuffix)
	if err := m.writeSaved(saved); err != nil {
		return err
	}
	m.log("undo", publicDNS, dnsListWords(cur), dnsListWords(orig))
	return nil
}

// publicDNSUndo says whether public-dns can be put back now, with the words for now and after.
func (m *Manager) publicDNSUndo(old string) (ok bool, now, after string) {
	x, isX := m.sys.(ExtraSystem)
	if !isX {
		return false, "", ""
	}
	var orig []DNSState
	if json.Unmarshal([]byte(old), &orig) != nil {
		return false, "", ""
	}
	cur, err := x.DNS()
	if err != nil || !allMatch(cloudflareFor(orig), cur) {
		return false, "", ""
	}
	return true, dnsListWords(cur), dnsListWords(orig)
}
