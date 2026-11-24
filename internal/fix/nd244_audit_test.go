package fix

// Checks for ND-244, written before the build: the situation is read
// again at the press, public-dns touches only the adapter in use, and the "now" words are read
// from the computer. Everything runs on a pretend computer.

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
)

// ndCond is a pretend Windows computer that also reads the situation at the press (ConditionSystem).
type ndCond struct {
	*ndExtra
	cond    Conditions
	condErr error
	asked   int
}

func (p *ndCond) Conditions() (Conditions, error) {
	p.asked++
	return p.cond, p.condErr
}

var _ ConditionSystem = (*ndCond)(nil)

func newNDCond(c Conditions) *ndCond { return &ndCond{ndExtra: newNDExtra(), cond: c} }

// lineDead is a connection where neither the router nor the internet answers.
var lineDead = Conditions{AdapterUp: true, HasAddress: true}

// ND-244: reset-adapter is refused when the router or the internet answers now, and runs when neither does.
func TestND244ResetAdapterRefusedWhenTheLineWorks(t *testing.T) {
	for _, c := range []Conditions{
		{AdapterUp: true, HasAddress: true, RouterAnswers: true},
		{AdapterUp: true, HasAddress: true, InternetAnswers: true},
		{AdapterUp: true, HasAddress: true, RouterAnswers: true, InternetAnswers: true},
	} {
		p := newNDCond(c)
		m := NewManager(p, t.TempDir(), Options{})
		r, err := m.Apply(ndResetAdapter, true)
		var ref *RefusedError
		if !errors.As(err, &ref) || r.Name != "" {
			t.Errorf("%+v: a working line must give a plain refusal, got %+v %v", c, r, err)
		} else if strings.TrimSpace(ref.Reason) == "" {
			t.Errorf("%+v: the refusal needs a plain note", c)
		}
		if p.resets != 0 || p.elevations != 0 {
			t.Errorf("%+v: a refusal runs nothing and asks for nothing: resets %d, elevations %d", c, p.resets, p.elevations)
		}
		if p.asked == 0 {
			t.Errorf("%+v: the situation must be read at the press", c)
		}
	}
	p := newNDCond(lineDead)
	m := NewManager(p, t.TempDir(), Options{})
	if _, err := m.Apply(ndResetAdapter, true); err != nil || p.resets != 1 {
		t.Errorf("a dead line: reset-adapter must run once, got err %v, resets %d", err, p.resets)
	}
}

// ND-244: renew-address is refused when a usable address is there now, and runs when none is.
func TestND244RenewAddressRefusedWhenAnAddressIsThere(t *testing.T) {
	p := newNDCond(Conditions{AdapterUp: true, HasAddress: true})
	m := NewManager(p, t.TempDir(), Options{})
	r, err := m.Apply(ndRenewAddress, true)
	var ref *RefusedError
	if !errors.As(err, &ref) || r.Name != "" || strings.TrimSpace(ref.Reason) == "" {
		t.Errorf("an address is there: want a plain refusal, got %+v %v", r, err)
	}
	if p.renews != 0 || p.elevations != 0 {
		t.Errorf("a refusal runs nothing: renews %d, elevations %d", p.renews, p.elevations)
	}
	p = newNDCond(Conditions{AdapterUp: true})
	m = NewManager(p, t.TempDir(), Options{})
	if _, err := m.Apply(ndRenewAddress, true); err != nil || p.renews != 1 {
		t.Errorf("no address: renew-address must run once, got err %v, renews %d", err, p.renews)
	}
}

// ND-244: when the situation cannot be read, both are refused with a plain note and nothing runs.
func TestND244UnreadableSituationRefusesBoth(t *testing.T) {
	for _, name := range []string{ndRenewAddress, ndResetAdapter} {
		p := newNDCond(Conditions{})
		p.condErr = errors.New("could not read")
		m := NewManager(p, t.TempDir(), Options{})
		_, err := m.Apply(name, true)
		var ref *RefusedError
		if !errors.As(err, &ref) || strings.TrimSpace(ref.Reason) == "" {
			t.Errorf("%s: unreadable situation: want a plain refusal, got %v", name, err)
		}
		if p.renews != 0 || p.resets != 0 || p.elevations != 0 {
			t.Errorf("%s: nothing may run", name)
		}
	}
}

// ND-244: a system without ConditionSystem keeps today's behaviour: the yes runs the fix.
func TestND244WithoutConditionSystemKeepsTodaysBehaviour(t *testing.T) {
	for _, name := range []string{ndRenewAddress, ndResetAdapter} {
		p := newNDExtra()
		m := NewManager(p, t.TempDir(), Options{})
		if _, err := m.Apply(name, true); err != nil || p.renews+p.resets != 1 {
			t.Errorf("%s: without ConditionSystem the yes must run it once: %v", name, err)
		}
	}
}

// ND-244: the "now" words for renew-address and reset-adapter come from the situation read now.
func TestND244NowWordsAreRead(t *testing.T) {
	has := NewManager(newNDCond(Conditions{AdapterUp: true, HasAddress: true, RouterAnswers: true, InternetAnswers: true}), t.TempDir(), Options{})
	pl, err := has.Preview(ndRenewAddress)
	if err != nil {
		t.Fatal(err)
	}
	low := strings.ToLower(pl.Before)
	if strings.Contains(low, "no usable") || strings.Contains(low, "no address") || !strings.Contains(low, "address") {
		t.Errorf("an address is there: the words must say an address is there: %q", pl.Before)
	}
	pl, err = has.Preview(ndResetAdapter)
	if err != nil {
		t.Fatal(err)
	}
	low = strings.ToLower(pl.Before)
	if strings.Contains(low, "nothing answers") || !strings.Contains(low, "router") {
		t.Errorf("the router answers: the words must say so, not that nothing answers: %q", pl.Before)
	}
	none := NewManager(newNDCond(Conditions{AdapterUp: true}), t.TempDir(), Options{})
	pl, err = none.Preview(ndRenewAddress)
	if err != nil {
		t.Fatal(err)
	}
	low = strings.ToLower(pl.Before)
	if !strings.Contains(low, "address") || !strings.Contains(low, "no") {
		t.Errorf("no address: the words must say there is no usable address: %q", pl.Before)
	}
	// fix list carries the same read words.
	for _, in := range has.List() {
		if in.Name == ndRenewAddress && strings.Contains(strings.ToLower(in.Now), "no usable") {
			t.Errorf("fix list must not say no usable address when one is there: %q", in.Now)
		}
		if in.Name == ndResetAdapter && strings.Contains(strings.ToLower(in.Now), "nothing answers") {
			t.Errorf("fix list must not say nothing answers when the router answers: %q", in.Now)
		}
	}
}

// ndRecord records what SetDNS is given.
type ndRecord struct {
	*ndExtra
	given [][]DNSState
}

func (p *ndRecord) SetDNS(in []DNSState) error {
	p.given = append(p.given, ndCopy(in))
	return p.ndExtra.SetDNS(in)
}

func ndTwoAdapters() []DNSState {
	return []DNSState{
		{Adapter: "Ethernet", IfIndex: 7, Servers: []string{"10.0.0.53"}, Automatic: false},
		{Adapter: "Wi-Fi", IfIndex: 12, Servers: []string{"192.168.1.1"}, Automatic: true, InUse: true},
	}
}

// ND-244: public-dns sets, saves and puts back only the adapter in use, and names only it.
func TestND244PublicDNSOnlyTheAdapterInUse(t *testing.T) {
	x := newNDExtra()
	x.dns = ndTwoAdapters()
	p := &ndRecord{ndExtra: x}
	m := NewManager(p, t.TempDir(), Options{})

	pl, err := m.Preview(ndPublicDNS)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(pl.Before, "Wi-Fi") || strings.Contains(pl.Before, "Ethernet") {
		t.Errorf("the now words must name only the adapter in use: %q", pl.Before)
	}
	for _, in := range m.List() {
		if in.Name == ndPublicDNS && (strings.Contains(in.Now, "Ethernet") || !strings.Contains(in.Now, "Wi-Fi")) {
			t.Errorf("fix list must name only the adapter in use: %q", in.Now)
		}
	}

	if _, err := m.Apply(ndPublicDNS, true); err != nil {
		t.Fatalf("apply: %v", err)
	}
	if len(p.given) != 1 || len(p.given[0]) != 1 || p.given[0][0].IfIndex != 12 {
		t.Errorf("SetDNS must get only the adapter in use: %+v", p.given)
	}
	eth := ndByIndex(x.dns)[7]
	if !reflect.DeepEqual(eth.Servers, []string{"10.0.0.53"}) || eth.Automatic {
		t.Errorf("the other adapter must not change: %+v", eth)
	}
	var saved []DNSState
	if err := json.Unmarshal([]byte(m.loadSaved()[ndPublicDNS]), &saved); err != nil {
		t.Fatalf("saved value: %v", err)
	}
	if len(saved) != 1 || saved[0].IfIndex != 12 {
		t.Errorf("only the adapter in use is saved: %+v", saved)
	}

	p.given = nil
	if err := m.Undo(ndPublicDNS); err != nil {
		t.Fatalf("undo: %v", err)
	}
	if len(p.given) != 1 || len(p.given[0]) != 1 || p.given[0][0].IfIndex != 12 {
		t.Errorf("undo must put back only the adapter in use: %+v", p.given)
	}
	if !reflect.DeepEqual(ndByIndex(x.dns), ndByIndex(ndTwoAdapters())) {
		t.Errorf("undo must restore exactly: %+v", x.dns)
	}
}

// ND-244: with no adapter in use, public-dns is refused with a plain note and nothing changes.
func TestND244PublicDNSRefusedWithNoAdapterInUse(t *testing.T) {
	x := newNDExtra()
	x.dns = ndTwoAdapters()
	for i := range x.dns {
		x.dns[i].InUse = false
	}
	m := NewManager(x, t.TempDir(), Options{})
	_, err := m.Apply(ndPublicDNS, true)
	var ref *RefusedError
	if !errors.As(err, &ref) || strings.TrimSpace(ref.Reason) == "" {
		t.Errorf("no adapter in use: want a plain refusal, got %v", err)
	}
	if x.setCalls != 0 || x.elevations != 0 {
		t.Errorf("a refusal changes nothing: set %d, elevations %d", x.setCalls, x.elevations)
	}
	if _, ok := m.loadSaved()[ndPublicDNS]; ok {
		t.Error("a refusal saves nothing")
	}
}
