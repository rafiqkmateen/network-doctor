package install

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"encoding/hex"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

// tgz makes a small gzip tar file with the given files.
func tgz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var b bytes.Buffer
	gz := gzip.NewWriter(&b)
	tw := tar.NewWriter(gz)
	for name, body := range files {
		tw.WriteHeader(&tar.Header{Name: name, Mode: 0o755, Size: int64(len(body)), Typeflag: tar.TypeReg})
		tw.Write([]byte(body))
	}
	tw.Close()
	gz.Close()
	return b.Bytes()
}

func sum(b []byte) string { h := sha256.Sum256(b); return hex.EncodeToString(h[:]) }

// ookla is a pretend Ookla server: every https request, whatever the name, reaches it, so the
// address check in the code is the real one.
type ooklaServer struct {
	srv  *httptest.Server
	mu   sync.Mutex
	seen []string
	body []byte
}

func newOokla(t *testing.T, body []byte) *ooklaServer {
	o := &ooklaServer{body: body}
	o.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		o.mu.Lock()
		o.seen = append(o.seen, "https://"+r.Host+r.URL.Path)
		o.mu.Unlock()
		w.Write(o.body)
	}))
	t.Cleanup(o.srv.Close)
	return o
}

func (o *ooklaServer) client() *http.Client {
	c := o.srv.Client()
	tr := c.Transport.(*http.Transport).Clone()
	addr := o.srv.Listener.Addr().String()
	tr.DialContext = func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}
	tr.DialTLSContext = nil
	tr.TLSClientConfig = &tls.Config{RootCAs: tr.TLSClientConfig.RootCAs, ServerName: "example.com"}
	c.Transport = tr
	c.CheckRedirect = ooklaRedirect
	return c
}

func failingClient(t *testing.T) *http.Client {
	return &http.Client{Transport: roundTrip(func(r *http.Request) (*http.Response, error) {
		t.Errorf("fetched %s", r.URL)
		return nil, io.EOF
	})}
}

type roundTrip func(*http.Request) (*http.Response, error)

func (f roundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// ND-302: each system has its own https address on install.speedtest.net and its own fixed SHA-256.
func TestOoklaFilePerSystem(t *testing.T) {
	want := map[string]string{
		"darwin/arm64": "macosx-universal.tgz", "darwin/amd64": "macosx-universal.tgz",
		"linux/amd64": "linux-x86_64.tgz", "linux/arm64": "linux-aarch64.tgz", "linux/arm": "linux-armhf.tgz",
	}
	for sys, end := range want {
		goos, goarch, _ := strings.Cut(sys, "/")
		f, ok := ooklaFileFor(defaultOoklaFiles, goos, goarch)
		if !ok {
			t.Fatalf("%s has no Ookla file", sys)
		}
		u, err := url.Parse(f.URL)
		if err != nil || u.Scheme != "https" || u.Host != "install.speedtest.net" || !strings.HasSuffix(f.URL, end) {
			t.Errorf("%s: %s", sys, f.URL)
		}
		if f.SHA256 != "" && len(f.SHA256) != 64 {
			t.Errorf("%s: bad SHA-256 %q", sys, f.SHA256)
		}
	}
	if _, ok := ooklaFileFor(defaultOoklaFiles, "linux", "386"); ok {
		t.Error("a chip Ookla does not serve was given a file")
	}

	good := tgz(t, map[string]string{"speedtest": "tool", "speedtest.md": "readme", "../escape": "bad"})
	for _, tc := range []struct {
		name  string
		hash  string
		added bool
	}{{"match", sum(good), true}, {"mismatch", strings.Repeat("0", 64), false}, {"no fixed hash yet", "", false}} {
		s := newScratch(t, "linux")
		o := newOokla(t, good)
		s.env.HTTP = o.client()
		s.env.OoklaFiles = map[string]OoklaFile{"linux/amd64": {
			URL: "https://install.speedtest.net/app/cli/ookla-speedtest-1.2.0-linux-x86_64.tgz", SHA256: tc.hash}}
		c := Choice{Weekly: true, OoklaOffer: true, OoklaAgree: true}
		r := Install(s.env, c, io.Discard)
		l := LayoutFor(s.env)
		tool := filepath.Join(l.ProgramDir, "ookla", "speedtest")
		if r.OoklaAdded != tc.added || exists(tool) != tc.added || choicesIn(t, l.ProgramDir)["ookla"] != tc.added {
			t.Errorf("%s: added %v, tool there %v", tc.name, r.OoklaAdded, exists(tool))
		}
		if exists(filepath.Join(s.home, ".local", "share", "escape")) || exists(filepath.Join(l.ProgramDir, "escape")) {
			t.Errorf("%s: a file escaped the ookla folder", tc.name)
		}
		runs := s.p.ran("speedtest")
		if tc.added {
			if len(runs) != 1 || strings.Join(runs[0][1:], " ") != "--accept-license --accept-gdpr -f json --progress=no --servers" {
				t.Errorf("license switches given %v", runs)
			}
			if !runnable(t, tool) {
				t.Error("the tool is not runnable")
			}
		} else {
			if len(runs) != 0 {
				t.Errorf("%s: the tool ran", tc.name)
			}
			if !strings.Contains(strings.Join(FinishLines("linux", "/p", "/r", c, r), " "), "The speed tool was not added.") {
				t.Errorf("%s: finish screen does not say so", tc.name)
			}
			left, _ := filepath.Glob(filepath.Join(l.ProgramDir, "*.tgz*"))
			left2, _ := filepath.Glob(filepath.Join(l.ProgramDir, ".ookla*"))
			if len(left)+len(left2) > 0 {
				t.Errorf("%s: the fetched file was not deleted: %v %v", tc.name, left, left2)
			}
		}
		if tc.hash == "" && len(o.seen) != 0 {
			t.Error("fetched with no fixed SHA-256")
		}
		if len(r.Problems) > 0 {
			t.Errorf("%s: the install did not go on: %v", tc.name, r.Problems)
		}
		if p := s.p.plans; len(p) != 1 || !p[0].Weekly {
			t.Errorf("%s: the rest of the install did not happen", tc.name)
		}
	}
}

// ND-302: the fetch refuses any address that is not https on install.speedtest.net, and redirects too.
func TestOoklaFetchRefusesOtherPlaces(t *testing.T) {
	good := tgz(t, map[string]string{"speedtest": "tool"})
	for _, u := range []string{"http://install.speedtest.net/a.tgz", "https://example.com/a.tgz",
		"https://install.speedtest.net.example.com/a.tgz"} {
		s := newScratch(t, "linux")
		o := newOokla(t, good)
		s.env.HTTP = o.client()
		s.env.OoklaFiles = map[string]OoklaFile{"linux/amd64": {URL: u, SHA256: sum(good)}}
		r := Install(s.env, Choice{OoklaOffer: true, OoklaAgree: true}, io.Discard)
		if r.OoklaAdded || len(o.seen) != 0 {
			t.Errorf("%s was fetched", u)
		}
	}
	req, _ := http.NewRequest("GET", "https://example.com/x", nil)
	if ooklaRedirect(req, nil) == nil {
		t.Error("a redirect away from Ookla was followed")
	}
	req2, _ := http.NewRequest("GET", "https://install.speedtest.net/x", nil)
	if ooklaRedirect(req2, nil) != nil {
		t.Error("a redirect within Ookla was refused")
	}
}

// No fetch without both ticks.
func TestOoklaNeedsBothTicks(t *testing.T) {
	for _, c := range []Choice{{OoklaOffer: true}, {OoklaAgree: true}} {
		s := newScratch(t, "darwin")
		s.env.HTTP = failingClient(t)
		Install(s.env, c, io.Discard)
		if exists(filepath.Join(LayoutFor(s.env).ProgramDir, "ookla")) {
			t.Error("Ookla without both ticks")
		}
	}
	_ = os.Remove
}
