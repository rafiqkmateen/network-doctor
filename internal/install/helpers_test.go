package install

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/fix"
	"github.com/rafiqkmateen/network-doctor/internal/jobs"
)

// simulated records every outside command and every job call, and answers with set replies.
type simulated struct {
	mu       sync.Mutex
	cmds     [][]string
	replies  map[string]string // command name to its output
	fails    map[string]bool   // command names that fail
	plans    []jobs.Plan
	removes  int
	stops    int
	applyErr error
}

func (p *simulated) run(name string, args ...string) (string, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.cmds = append(p.cmds, append([]string{name}, args...))
	if p.fails[filepath.Base(name)] {
		return "", os.ErrNotExist
	}
	return p.replies[filepath.Base(name)], nil
}

func (p *simulated) ran(name string) [][]string {
	p.mu.Lock()
	defer p.mu.Unlock()
	var got [][]string
	for _, c := range p.cmds {
		if filepath.Base(c[0]) == name {
			got = append(got, c)
		}
	}
	return got
}

// scratch makes a simulated computer under one temporary folder: home is the person's home
// folder, setup holds the setup program and its guide (outside home, read only).
type scratch struct {
	root, home, setup string
	env               Env
	p                 *simulated
	fixes             *simulatedFixes
}

func newScratch(t *testing.T, goos string) *scratch {
	t.Helper()
	root := t.TempDir()
	s := &scratch{root: root, home: filepath.Join(root, "home"), setup: filepath.Join(root, "setup"),
		p: &simulated{replies: map[string]string{}, fails: map[string]bool{}}, fixes: &simulatedFixes{goos: goos}}
	for _, d := range []string{s.home, s.setup} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	exe := filepath.Join(s.setup, "netdoctor")
	os.WriteFile(exe, []byte("simulated program"), 0o755)
	os.WriteFile(filepath.Join(s.setup, "GUIDE.md"), []byte("# Guide\n"), 0o644)
	results := filepath.Join(s.home, ".config", "NetworkDoctor")
	if goos == "darwin" {
		results = filepath.Join(s.home, "Library", "Application Support", "NetworkDoctor")
	}
	s.p.fails["xdg-user-dir"] = true // most tests: no such helper, so ~/Desktop
	s.p.fails["osascript"] = true    // the Finder alias fails, so the Desktop gets a link
	s.env = Env{
		GOOS: goos, GOARCH: "amd64", Home: s.home, Exe: exe, ResultsDir: results, Version: "9.9.9",
		Getenv: func(string) string { return "" },
		Run:    s.p.run,
		RunContext: func(_ context.Context, name string, args ...string) (string, error) {
			return s.p.run(name, args...)
		},
		Apply: func(g, h string, pl jobs.Plan, r jobs.Runner) error {
			s.p.mu.Lock()
			defer s.p.mu.Unlock()
			s.p.plans = append(s.p.plans, pl)
			return s.p.applyErr
		},
		Remove: func(g, h string, r jobs.Runner) error {
			s.p.mu.Lock()
			s.p.removes++
			s.p.mu.Unlock()
			return nil
		},
		StopHelper: func(g string, r jobs.Runner) error {
			s.p.mu.Lock()
			s.p.stops++
			s.p.mu.Unlock()
			return nil
		},
		HelperRunning: func(string, jobs.Runner) bool { return false },
		StartHelper:   func(string, string, jobs.Runner) error { return nil },
		OpenURL:       func(string) error { return nil },
		AliasWait:     2 * time.Second,
		// Windows makes links only with extra rights, so the tests note the link in a small file.
		Symlink: func(target, link string) error { return os.WriteFile(link, []byte("link to "+target), 0o644) },
		Fixes:   s.fixes,
	}
	return s
}

// simulatedFixes is a simulated computer for the fixes: it records every read and change, and says,
// as the real Mac and Linux ones do, which fixes may ask for administrator rights there.
type simulatedFixes struct {
	mu      sync.Mutex
	goos    string
	proxy   fix.ProxyState
	setErr  error
	readErr error // the proxy cannot be read
	calls   []string
	program string // the installed program, to see whether it was still there at a change
	whenSet []bool // whether the program was still there at each change
}

func (f *simulatedFixes) note(c string) { f.mu.Lock(); f.calls = append(f.calls, c); f.mu.Unlock() }
func (f *simulatedFixes) FlushDNS() error {
	f.note("FlushDNS")
	return nil
}
func (f *simulatedFixes) Proxy() (fix.ProxyState, error) { f.note("Proxy"); return f.proxy, f.readErr }
func (f *simulatedFixes) SetProxy(s fix.ProxyState) error {
	f.note("SetProxy")
	f.whenSet = append(f.whenSet, f.program != "" && exists(f.program))
	if f.setErr != nil {
		return f.setErr
	}
	f.proxy = s
	return nil
}
func (f *simulatedFixes) PowerSaving() (string, error) { f.note("PowerSaving"); return "off", nil }
func (f *simulatedFixes) SetPowerSaving(string) error  { f.note("SetPowerSaving"); return nil }
func (f *simulatedFixes) PowerSavingDropsLink() bool   { return false }
func (f *simulatedFixes) Elevate() error               { f.note("Elevate"); return nil }
func (f *simulatedFixes) MayAskAdmin(name string) bool {
	switch f.goos {
	case "darwin":
		return name == "proxy" || name == "public-dns"
	case "linux":
		return name == "public-dns"
	}
	return name == "power-saving" || name == "public-dns"
}

// simulatedDNS is a simulated computer that can also read and set the name servers, as Windows does,
// and never asks for administrator rights, so removal runs the name-server undo itself.
type simulatedDNS struct {
	*simulatedFixes
	dns  []fix.DNSState
	sets int
}

func (d *simulatedDNS) MayAskAdmin(string) bool        { return false }
func (d *simulatedDNS) DNS() ([]fix.DNSState, error)   { d.note("DNS"); return d.dns, nil }
func (d *simulatedDNS) SetDNS(in []fix.DNSState) error { d.sets++; d.dns = in; return nil }
func (d *simulatedDNS) Renew() error                   { return nil }
func (d *simulatedDNS) ResetAdapter() error            { return nil }

// notesFor builds undo notes, each fix's note with the account given beside it, as Apply writes
// them; an empty account leaves the account out, as notes from before it was recorded.
func notesFor(t *testing.T, account string, notes map[string]string) string {
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

// writeNotes puts undo notes in the results folder, as an applied fix leaves them.
func writeNotes(t *testing.T, s *scratch, text string) {
	t.Helper()
	os.MkdirAll(s.env.ResultsDir, 0o755)
	if err := os.WriteFile(filepath.Join(s.env.ResultsDir, "fix-saved.json"), []byte(text), 0o644); err != nil {
		t.Fatal(err)
	}
}

func read(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return string(b)
}

// runnable says whether the file can be run; Windows keeps no such mark, so it always passes there.
func runnable(t *testing.T, path string) bool {
	t.Helper()
	fi, err := os.Stat(path)
	if err != nil {
		return false
	}
	return runtime.GOOS == "windows" || fi.Mode()&0o111 != 0
}

// tree lists every path under root with its size and time, for before-and-after checks.
func tree(t *testing.T, root string) map[string]string {
	t.Helper()
	got := map[string]string{}
	filepath.Walk(root, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		got[p] = fi.ModTime().String() + "/" + strings.Repeat("x", int(fi.Size()%7))
		return nil
	})
	return got
}

// execArgs splits a desktop entry Exec value into its arguments, following the Desktop Entry
// rules: the string escapes first (\\ \s \n \t \r), then double quotes with \" \` \$ \\ inside.
func execArgs(t *testing.T, v string) []string {
	t.Helper()
	var s strings.Builder
	for i := 0; i < len(v); i++ {
		if v[i] == '\\' && i+1 < len(v) {
			switch v[i+1] {
			case '\\':
				s.WriteByte('\\')
				i++
				continue
			case 's':
				s.WriteByte(' ')
				i++
				continue
			}
		}
		s.WriteByte(v[i])
	}
	in := s.String()
	var args []string
	var cur strings.Builder
	quoted, have := false, false
	for i := 0; i < len(in); i++ {
		c := in[i]
		switch {
		case quoted && c == '\\' && i+1 < len(in) && strings.IndexByte("\"`$\\", in[i+1]) >= 0:
			cur.WriteByte(in[i+1])
			i++
		case c == '"':
			quoted, have = !quoted, true
		case !quoted && c == ' ':
			if have {
				args = append(args, cur.String())
				cur.Reset()
				have = false
			}
		default:
			cur.WriteByte(c)
			have = true
		}
	}
	if quoted {
		t.Fatalf("unclosed quote in Exec: %s", v)
	}
	if have {
		args = append(args, cur.String())
	}
	return args
}

// entry reads one key of a desktop entry's [Desktop Entry] group.
func entry(text, key string) string {
	for _, l := range strings.Split(text, "\n") {
		if strings.HasPrefix(l, key+"=") {
			return strings.TrimPrefix(l, key+"=")
		}
	}
	return ""
}
