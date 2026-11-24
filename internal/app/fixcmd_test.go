package app

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rafiqkmateen/network-doctor/internal/fix"
)

type cliSimulated struct {
	proxy   fix.ProxyState
	writes  int
	readErr error // the proxy cannot be read
}

func (p *cliSimulated) FlushDNS() error                { return nil }
func (p *cliSimulated) Proxy() (fix.ProxyState, error) { return p.proxy, p.readErr }
func (p *cliSimulated) SetProxy(s fix.ProxyState) error {
	p.writes++
	p.proxy = s
	return nil
}
func (p *cliSimulated) PowerSaving() (string, error) { return "on", nil }
func (p *cliSimulated) SetPowerSaving(string) error  { return nil }
func (p *cliSimulated) PowerSavingDropsLink() bool   { return false }
func (p *cliSimulated) Elevate() error               { return nil }

func runFix(t *testing.T, p *cliSimulated, dir string, args ...string) (string, int) {
	t.Helper()
	old := newFixSystem
	newFixSystem = func() fix.System { return p }
	defer func() { newFixSystem = old }()
	var out bytes.Buffer
	code := Run(context.Background(), append([]string{"fix"}, args...), healthy(), dir, &out)
	return out.String(), code
}

// ND-90, ND-91: apply without --yes shows before and after and changes nothing; with --yes it changes that one fix.
func TestFixApplyShowsThenNeedsYes(t *testing.T) {
	p := &cliSimulated{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	dir := t.TempDir()
	out, code := runFix(t, p, dir, "apply", "proxy")
	if code != 0 || p.writes != 0 {
		t.Fatalf("no yes means no change: code %d, writes %d", code, p.writes)
	}
	if !strings.Contains(out, "10.0.0.9:8080") || !strings.Contains(out, "--yes") {
		t.Errorf("must show the setting now and how to say yes:\n%s", out)
	}
	out, code = runFix(t, p, dir, "apply", "proxy", "--yes")
	if code != 0 || p.proxy.Enabled {
		t.Fatalf("with yes the proxy goes off: code %d\n%s", code, out)
	}
	if !strings.Contains(out, "fix undo proxy") {
		t.Errorf("must say how to put it back:\n%s", out)
	}
	out, code = runFix(t, p, dir, "undo", "proxy")
	if code != 0 || !p.proxy.Enabled {
		t.Errorf("undo must restore: code %d\n%s", code, out)
	}
}

// ND-95: a refused fix exits with a plain reason and a non-zero code.
func TestFixRefusalExitsWithReason(t *testing.T) {
	out, code := runFix(t, &cliSimulated{}, t.TempDir(), "apply", "driver", "--yes")
	if code == 0 || !strings.Contains(out, "never changes drivers") {
		t.Errorf("code %d\n%s", code, out)
	}
}

// ND-96: with NETDOCTOR_UNATTENDED set, fixes stay off until enable-here.
func TestFixServerSwitch(t *testing.T) {
	t.Setenv("NETDOCTOR_UNATTENDED", "1")
	p := &cliSimulated{proxy: fix.ProxyState{Enabled: true, Server: "x:1"}}
	dir := t.TempDir()
	if _, code := runFix(t, p, dir, "apply", "proxy", "--yes"); code == 0 || p.writes != 0 {
		t.Fatalf("server must refuse by default: code %d writes %d", code, p.writes)
	}
	runFix(t, p, dir, "enable-here")
	if _, code := runFix(t, p, dir, "apply", "proxy", "--yes"); code != 0 || p.writes != 1 {
		t.Errorf("after enable-here it must work: code %d writes %d", code, p.writes)
	}
}

// ND-316: `fix undo-all`, what the Windows uninstaller runs, puts back every fix still applied and
// exits 0; with one it cannot put back it exits 1 with the plain lines, also written to --note.
func TestFixUndoAll(t *testing.T) {
	p := &cliSimulated{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	dir := t.TempDir()
	if out, code := runFix(t, p, dir, "undo-all"); code != 0 || p.writes != 0 {
		t.Fatalf("nothing applied: code %d writes %d\n%s", code, p.writes, out)
	}
	runFix(t, p, dir, "apply", "proxy", "--yes")
	if out, code := runFix(t, p, dir, "undo-all"); code != 0 || !p.proxy.Enabled {
		t.Fatalf("put back: code %d %+v\n%s", code, p.proxy, out)
	}
	os.WriteFile(filepath.Join(dir, "fix-saved.json"), []byte("not notes"), 0o644)
	note := filepath.Join(t.TempDir(), "fix-not-undone.txt")
	out, code := runFix(t, p, dir, "undo-all", "--note", note)
	if code != 1 || !strings.Contains(out, "could not put back") || !strings.Contains(out, dir) {
		t.Fatalf("damaged notes: code %d\n%s", code, out)
	}
	if b, err := os.ReadFile(note); err != nil || strings.TrimSpace(string(b)) != strings.TrimSpace(out) {
		t.Errorf("the note file must hold the same lines: %v %q", err, b)
	}
}

// ND-316: at removal a setting changed by hand since the fix is settled: undo-all leaves it, drops
// its note, exits 0 and says it was left as set (also in --note); mixed with a fix that fails it
// exits 1 and keeps only that one's note. `fix undo` itself still refuses a changed setting.
func TestFixUndoAllSettlesAChangedSetting(t *testing.T) {
	p := &cliSimulated{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	dir := t.TempDir()
	runFix(t, p, dir, "apply", "proxy", "--yes")
	p.proxy = fix.ProxyState{Enabled: true, Server: "changed.example:1"}
	if out, code := runFix(t, p, dir, "undo", "proxy"); code == 0 {
		t.Fatalf("fix undo must still refuse a changed setting:\n%s", out)
	}
	note := filepath.Join(t.TempDir(), "fix-not-undone.txt")
	out, code := runFix(t, p, dir, "undo-all", "--note", note)
	if code != 0 || !strings.Contains(out, "left as you set it") || strings.Contains(out, "could not put back") {
		t.Fatalf("settled: code %d\n%s", code, out)
	}
	if b, err := os.ReadFile(note); err != nil || !strings.Contains(string(b), "left as you set it") {
		t.Errorf("the note file must say it was left as set: %v %q", err, b)
	}
	if b, _ := os.ReadFile(filepath.Join(dir, "fix-saved.json")); strings.Contains(string(b), "proxy") {
		t.Errorf("the settled note must be dropped: %s", b)
	}
	if p.proxy.Server != "changed.example:1" || p.writes != 1 {
		t.Error("a setting changed by hand must be left alone")
	}

	// Mixed: the changed proxy and a name-server note this computer cannot put back.
	os.WriteFile(filepath.Join(dir, "fix-saved.json"),
		[]byte(cliNotes(t, fix.CurrentAccount(), map[string]string{"proxy": `{"enabled":true,"server":"10.0.0.9:8080"}`, "public-dns": "[]"})), 0o644)
	out, code = runFix(t, p, dir, "undo-all")
	if code != 1 || !strings.Contains(out, "left as you set it") || !strings.Contains(out, "could not put back") {
		t.Fatalf("mixed: code %d\n%s", code, out)
	}
	b, _ := os.ReadFile(filepath.Join(dir, "fix-saved.json"))
	if strings.Contains(string(b), "proxy") || !strings.Contains(string(b), "public-dns") {
		t.Errorf("mixed: only the failed note may stay: %s", b)
	}
}

// ND-316: the uninstaller names the results folder with --dir, so a NETDOCTOR_DIR pointing
// elsewhere never makes undo-all read other notes and report success; and with --kept the note
// says the results are kept, not deleted.
func TestFixUndoAllFolderAndKept(t *testing.T) {
	p := &cliSimulated{proxy: fix.ProxyState{Enabled: true, Server: "10.0.0.9:8080"}}
	real, other := t.TempDir(), t.TempDir()
	os.WriteFile(filepath.Join(real, "fix-saved.json"), []byte("not notes"), 0o644) // a genuine failure
	note := filepath.Join(t.TempDir(), "fix-not-undone.txt")
	out, code := runFix(t, p, other, "undo-all", "--dir", real, "--note", note)
	if code != 1 || !strings.Contains(out, real) || strings.Contains(out, other) {
		t.Fatalf("the named folder must be used, not the program's own: code %d\n%s", code, out)
	}
	if !strings.Contains(out, "Everything else it saved is deleted.") {
		t.Errorf("without --kept the note says the rest is deleted:\n%s", out)
	}
	out, code = runFix(t, p, other, "undo-all", "--dir", real, "--kept", "--note", note)
	b, _ := os.ReadFile(note)
	for _, text := range []string{out, string(b)} {
		if code != 1 || !strings.Contains(text, "Your saved results are kept too.") || strings.Contains(text, "deleted") {
			t.Errorf("with --kept the note must say the results are kept: code %d\n%s", code, text)
		}
	}
	// The folder named by --dir with nothing recorded is a success, even if the other one has notes.
	empty := t.TempDir()
	if out, code := runFix(t, p, real, "undo-all", "--dir", empty); code != 0 {
		t.Errorf("an empty named folder: code %d\n%s", code, out)
	}
	if out, code := runFix(t, p, real, "undo-all", "--dir"); code != 2 {
		t.Errorf("--dir with no folder must be refused: code %d\n%s", code, out)
	}
}

// cliNotes builds undo notes with each fix's account beside it, as Apply writes them; an empty
// account leaves it out, as notes from before it was recorded.
func cliNotes(t *testing.T, account string, notes map[string]string) string {
	t.Helper()
	all := map[string]string{}
	for k, v := range notes {
		all[k] = v
		if account != "" {
			all[k+"@account"] = account
		}
	}
	b, err := json.Marshal(all)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// ND-316: in undo-all, what the Windows uninstaller runs, a proxy that cannot be read, a note of
// another account, or an old note with no account is a failure, never settled: exit 1, note kept.
func TestFixUndoAllReadErrorOrOtherAccountIsAFailure(t *testing.T) {
	for _, c := range []struct {
		name, account string
		readErr       error
	}{
		{"read error", fix.CurrentAccount(), errors.New("could not read the proxy")},
		{"another account", "PC\\someone-else (S-1-5-21-9)", nil},
		{"no account", "", nil},
	} {
		p := &cliSimulated{proxy: fix.ProxyState{Enabled: true, Server: "changed.example:1"}, readErr: c.readErr}
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "fix-saved.json"),
			[]byte(cliNotes(t, c.account, map[string]string{"proxy": `{"enabled":true,"server":"10.0.0.9:8080"}`})), 0o644)
		out, code := runFix(t, p, dir, "undo-all")
		b, _ := os.ReadFile(filepath.Join(dir, "fix-saved.json"))
		if code != 1 || strings.Contains(out, "left as you set it") || !strings.Contains(string(b), `"proxy"`) || p.writes != 0 {
			t.Errorf("%s: code %d writes %d notes %s\n%s", c.name, code, p.writes, b, out)
		}
	}
}

// cliDNS is a simulated Windows computer that can read and set the name servers.
type cliDNS struct {
	cliSimulated
	dns  []fix.DNSState
	sets int
}

func (d *cliDNS) DNS() ([]fix.DNSState, error)   { return d.dns, nil }
func (d *cliDNS) SetDNS(in []fix.DNSState) error { d.sets++; d.dns = in; return nil }
func (d *cliDNS) Renew() error                   { return nil }
func (d *cliDNS) ResetAdapter() error            { return nil }

// ND-316: in undo-all, what the Windows uninstaller runs, the name servers are settled only when the
// adapter the fix changed is still listed and its servers differ; an adapter that is gone stays a
// failure, exit 1 and the note kept.
func TestFixUndoAllMissingAdapterIsAFailure(t *testing.T) {
	note := cliNotes(t, fix.CurrentAccount(), map[string]string{
		"public-dns": `[{"adapter":"Wi-Fi","if_index":12,"servers":["192.168.1.1"],"automatic":true,"in_use":true}]`})
	for _, gone := range []bool{true, false} {
		d := &cliDNS{}
		if gone {
			d.dns = []fix.DNSState{{Adapter: "Other", IfIndex: 99, Servers: []string{"10.9.9.9"}, InUse: true}}
		} else {
			d.dns = []fix.DNSState{{Adapter: "Wi-Fi", IfIndex: 12, Servers: []string{"9.9.9.9"}, InUse: true}}
		}
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "fix-saved.json"), []byte(note), 0o644)
		old := newFixSystem
		newFixSystem = func() fix.System { return d }
		var out bytes.Buffer
		code := Run(context.Background(), []string{"fix", "undo-all"}, healthy(), dir, &out)
		newFixSystem = old
		b, _ := os.ReadFile(filepath.Join(dir, "fix-saved.json"))
		if d.sets != 0 {
			t.Errorf("gone %v: the name servers were changed", gone)
		}
		if gone && (code != 1 || !strings.Contains(out.String(), "could not put back") || !strings.Contains(string(b), "public-dns")) {
			t.Errorf("adapter gone: code %d notes %s\n%s", code, b, out.String())
		}
		if !gone && (code != 0 || !strings.Contains(out.String(), "left as you set it") || strings.Contains(string(b), "public-dns")) {
			t.Errorf("present and changed: code %d notes %s\n%s", code, b, out.String())
		}
	}
}

// The plain list names each fix and its setting now, and changes nothing.
func TestFixList(t *testing.T) {
	p := &cliSimulated{proxy: fix.ProxyState{Enabled: true, Server: "x:1"}}
	out, code := runFix(t, p, t.TempDir(), "list")
	if code != 0 || p.writes != 0 {
		t.Fatalf("code %d writes %d", code, p.writes)
	}
	for _, want := range []string{"name-cache", "proxy", "power-saving"} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %s:\n%s", want, out)
		}
	}
}
