package fix

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// ND-316: each fix's note remembers the account that applied it. At removal a setting is settled as
// changed by hand only when the note's account is the account the undo runs as; another account,
// or an old note with no account, stays a real failure, the note kept.
func TestUndoAllSettlesOnlyForTheSameAccount(t *testing.T) {
	changed := ProxyState{Enabled: true, Server: "changed.example:1"}
	for _, tc := range []struct {
		name      string
		applyAs   string
		undoAs    string
		noAccount bool
		settled   bool
	}{
		{"same account", "PC\\ann (S-1-5-21-1)", "PC\\ann (S-1-5-21-1)", false, true},
		{"another account", "PC\\ann (S-1-5-21-1)", "PC\\admin (S-1-5-21-2)", false, false},
		{"old note with no account", "PC\\ann (S-1-5-21-1)", "PC\\ann (S-1-5-21-1)", true, false},
		{"account unknown at removal", "PC\\ann (S-1-5-21-1)", "", false, false},
	} {
		p := newSimulated()
		m, dir := newManager(t, p, Options{})
		m.account = func() string { return tc.applyAs }
		if _, err := m.Apply(proxy, true); err != nil {
			t.Fatal(err)
		}
		if tc.noAccount {
			s := m.loadSaved()
			delete(s, proxy+accountSuffix)
			m.writeSaved(s)
		}
		m.account = func() string { return tc.undoAs }
		p.proxy = changed
		f, s := m.UndoAll(nil)
		if tc.settled != (len(s) == 1 && len(f) == 0) {
			t.Errorf("%s: failures %+v settled %v", tc.name, f, s)
		}
		b, _ := os.ReadFile(filepath.Join(dir, "fix-saved.json"))
		if tc.settled == strings.Contains(string(b), `"proxy"`) {
			t.Errorf("%s: notes %s", tc.name, b)
		}
		if !tc.settled && (len(f) != 1 || f[0].Reason != ReasonAccount) {
			t.Errorf("%s: want the account reason: %+v", tc.name, f)
		}
		if p.proxy != changed {
			t.Errorf("%s: the setting was changed", tc.name)
		}
	}
}

// ND-316: the account is written beside the note when a fix is applied, and goes with the note
// when the fix is put back; a note's account is never taken for a fix of its own.
func TestNotesRecordTheAccount(t *testing.T) {
	p := newSimulated()
	m, dir := newManager(t, p, Options{})
	m.account = func() string { return "PC\\ann (S-1-5-21-1)" }
	m.Apply(proxy, true)
	var notes map[string]string
	b, _ := os.ReadFile(filepath.Join(dir, "fix-saved.json"))
	json.Unmarshal(b, &notes)
	if notes[proxy+accountSuffix] != "PC\\ann (S-1-5-21-1)" {
		t.Errorf("the account is not recorded: %s", b)
	}
	if f, s := m.UndoAll(nil); len(f) != 0 || len(s) != 0 || !p.proxy.Enabled {
		t.Fatalf("put back: %+v %v", f, s)
	}
	b, _ = os.ReadFile(filepath.Join(dir, "fix-saved.json"))
	if strings.Contains(string(b), "proxy") {
		t.Errorf("the note and its account must go when put back: %s", b)
	}
	if CurrentAccount() == "" {
		t.Error("this computer's account should be readable")
	}
}

// ND-316: a proxy read that fails is an error, never "off with no server": the Windows registry
// answer is read strictly, and the Linux reads give their errors.
func TestProxyReadsKeepTheirErrors(t *testing.T) {
	const found = "\r\nHKEY_CURRENT_USER\\Software\\Microsoft\\Windows\\CurrentVersion\\Internet Settings\r\n    ProxyEnable    REG_DWORD    0x1\r\n\r\n"
	if v, ok, err := parseRegValue(found, nil, "ProxyEnable"); err != nil || !ok || v != "0x1" {
		t.Errorf("found: %q %v %v", v, ok, err)
	}
	missing := "ERROR: The system was unable to find the specified registry key or value.\r\n"
	if v, ok, err := parseRegValue(missing, errors.New("exit status 1"), "ProxyServer"); err != nil || ok || v != "" {
		t.Errorf("a value that is not set is no error: %q %v %v", v, ok, err)
	}
	if _, _, err := parseRegValue("ERROR: Access is denied.\r\n", errors.New("exit status 1"), "ProxyEnable"); err == nil {
		t.Error("a refused read must be an error")
	}
	if _, _, err := parseRegValue("", errors.New("exec: reg: not found"), "ProxyEnable"); err == nil {
		t.Error("a read that did not run must be an error")
	}

	p := newLinux()
	s := p.sys()
	run := s.Run
	s.Run = func(name string, args ...string) (string, error) {
		if name == "gsettings" && strings.Contains(strings.Join(args, " "), "proxy.http host") {
			return "", fail
		}
		return run(name, args...)
	}
	if _, err := s.Proxy(); err == nil {
		t.Error("a Linux proxy host that cannot be read must be an error")
	}
}

// ND-316: on Linux the name servers count as the same adapter only when the NetworkManager
// connection is the same too; another connection on that device stays a failure.
func TestLinuxDNSOtherConnectionIsAFailure(t *testing.T) {
	want := []DNSState{{Adapter: "Home", IfIndex: 4, ConnUUID: "aaaa", Servers: []string{"10.0.0.1"}}}
	if !allPresent(want, []DNSState{{Adapter: "Home", IfIndex: 4, ConnUUID: "aaaa"}}) {
		t.Error("same device and connection is present")
	}
	if allPresent(want, []DNSState{{Adapter: "Cafe", IfIndex: 4, ConnUUID: "bbbb"}}) {
		t.Error("another connection on the same device is not the adapter the fix changed")
	}
	if !allPresent([]DNSState{{IfIndex: 4}}, []DNSState{{IfIndex: 4, ConnUUID: "bbbb"}}) {
		t.Error("with no connection recorded (Windows, Mac) the device number decides")
	}
}
