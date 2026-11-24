package install

import (
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rafiqkmateen/network-doctor/internal/jobs"
)

// A repair restarts an alerts helper that was running, when alerts are still chosen.
func TestRepairRestartsRunningHelper(t *testing.T) {
	for _, tc := range []struct {
		running, alerts, start bool
	}{{true, true, true}, {true, false, false}, {false, true, false}} {
		s := newScratch(t, "linux")
		var order []string
		s.env.HelperRunning = func(string, jobs.Runner) bool { order = append(order, "asked"); return tc.running }
		s.env.StopHelper = func(string, jobs.Runner) error { order = append(order, "stop"); return nil }
		s.env.Apply = func(string, string, jobs.Plan, jobs.Runner) error { order = append(order, "apply"); return nil }
		s.env.StartHelper = func(g, h string, r jobs.Runner) error {
			if h != s.home {
				t.Errorf("home %q", h)
			}
			order = append(order, "start")
			return nil
		}
		Install(s.env, Choice{Alerts: tc.alerts}, io.Discard)
		want := "asked,stop,apply"
		if tc.start {
			want += ",start"
		}
		if strings.Join(order, ",") != want {
			t.Errorf("%+v: %v", tc, order)
		}
	}
}

// fakeOpen is a simulated open page server that answers the retire request.
func fakeOpen(t *testing.T, state, token string, quits bool) (*httptest.Server, *[]http.Header) {
	t.Helper()
	var mu sync.Mutex
	var got []http.Header
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got = append(got, r.Header.Clone())
		mu.Unlock()
		if r.Method != http.MethodPost || r.URL.Path != "/api/retire" || r.Header.Get("X-NetDoctor-Retire") != token {
			http.Error(w, "no", http.StatusForbidden)
			return
		}
		w.WriteHeader(http.StatusNoContent)
		if quits {
			os.Remove(state) // a retired server removes its note as it quits
		}
	}))
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	srv.Listener = ln
	srv.Start()
	t.Cleanup(srv.Close)
	port := strings.TrimPrefix(srv.URL, "http://127.0.0.1:")
	os.MkdirAll(filepath.Dir(state), 0o755)
	b, _ := json.Marshal(map[string]string{"port": port, "token": token})
	os.WriteFile(state, b, 0o600)
	return srv, &got
}

// Uninstall retires a running open page, saying why, and stops it only when it does not go.
func TestUninstallRetiresOpenPage(t *testing.T) {
	for _, goos := range []string{"linux", "darwin"} {
		for _, quits := range []bool{true, false} {
			s := newScratch(t, goos)
			s.env.RetireWait = 300 * time.Millisecond
			s.p.replies["id"] = "1234"
			Install(s.env, Choice{}, io.Discard)
			state := filepath.Join(s.env.ResultsDir, "open-page.json")
			_, got := fakeOpen(t, state, "secret", quits)
			Uninstall(s.env, false, io.Discard)
			if len(*got) != 1 || (*got)[0].Get("X-NetDoctor-Retire-Reason") != "removed" {
				t.Fatalf("%s: retire requests %v", goos, *got)
			}
			kills := s.p.ran("pkill")
			if quits {
				if len(kills) != 0 {
					t.Errorf("%s: a page that went was still stopped: %v", goos, kills)
				}
				continue
			}
			if len(kills) != 1 || kills[0][1] != "-u" || kills[0][2] != "1234" || kills[0][3] != "-f" {
				t.Fatalf("%s: stop: %v", goos, kills)
			}
			re := regexp.MustCompile(kills[0][4])
			prog := filepath.ToSlash(LayoutFor(s.env).Program)
			for line, want := range map[string]bool{
				prog + " open":              true,
				prog + " open --plan 100":   true,
				prog + " uninstall":         false,
				prog + " install":           false,
				prog + " watch":             false,
				"/tmp/other/netdoctor open": false,
				"/x" + prog + " open":       false,
			} {
				if re.MatchString(line) != want {
					t.Errorf("%s: pattern %q on %q: want %v", goos, kills[0][4], line, want)
				}
			}
		}
	}
}

// With no open page, uninstall stops nothing.
func TestUninstallWithNoOpenPage(t *testing.T) {
	s := newScratch(t, "linux")
	Install(s.env, Choice{}, io.Discard)
	Uninstall(s.env, false, io.Discard)
	if len(s.p.ran("pkill")) != 0 {
		t.Error("stopped something with no open page")
	}
}
