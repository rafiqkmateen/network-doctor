package install

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	_ "embed"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

//go:embed page.html
var pageHTML []byte

// Mode says which page the server shows.
type Mode int

// The two pages.
const (
	ModeInstall Mode = iota
	ModeUninstall
)

func (m Mode) String() string {
	if m == ModeUninstall {
		return "uninstall"
	}
	return "install"
}

// The policy every answer carries: nothing from outside, no frames, no forms sent anywhere (ND-272).
const pagePolicy = "default-src 'none'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; " +
	"img-src 'self' data:; base-uri 'none'; form-action 'none'; frame-ancestors 'none'"

func guardHeaders(w http.ResponseWriter) {
	h := w.Header()
	h.Set("X-Frame-Options", "DENY")
	h.Set("Content-Security-Policy", pagePolicy)
	h.Set("Referrer-Policy", "no-referrer")
	h.Set("X-Content-Type-Options", "nosniff")
	h.Set("Cache-Control", "no-store")
}

// hostOK accepts only localhost, 127.0.0.1 or [::1] with this exact port, as the open page does.
func hostOK(host, port string) bool {
	h, p, err := net.SplitHostPort(host)
	if err != nil || p != port {
		return false
	}
	h = strings.ToLower(h)
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// pageServer holds one setup or removal page.
type pageServer struct {
	e     Env
	mode  Mode
	log   io.Writer
	port  string
	token string
	l     Layout

	mu       sync.Mutex
	lastSeen time.Time
	shownAt  time.Time // the finish screen was shown
	working  bool
	stopAsk  bool // OK was pressed on the no-browser box: stop once no work runs
	ran      bool // one install or removal per page
	guideOK  bool // the install on this page finished well, so the guide route answers
}

// guideLimit caps how much of the guide file the guide route reads.
const guideLimit = 1 << 20

func newToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// ServeHTTP checks every request before anything else (ND-272): this computer only, the Host
// rule, no other site, the token as the first part of the path, and the page's own header on
// anything that changes something.
func (s *pageServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	guardHeaders(w)
	refuse := func() { http.Error(w, "not allowed", http.StatusForbidden) }
	if h, _, err := net.SplitHostPort(r.RemoteAddr); err != nil || !net.ParseIP(h).IsLoopback() {
		refuse()
		return
	}
	if !hostOK(r.Host, s.port) {
		refuse()
		return
	}
	if site := r.Header.Get("Sec-Fetch-Site"); site != "" && site != "none" && site != "same-origin" {
		refuse()
		return
	}
	if o := r.Header.Get("Origin"); o != "" && o != "http://127.0.0.1:"+s.port && o != "http://localhost:"+s.port {
		refuse()
		return
	}
	prefix := "/" + s.token + "/"
	if len(r.URL.Path) < len(prefix) ||
		subtle.ConstantTimeCompare([]byte(r.URL.Path[:len(prefix)]), []byte(prefix)) != 1 {
		http.NotFound(w, r)
		return
	}
	s.mu.Lock()
	s.lastSeen = time.Now()
	s.mu.Unlock()
	rest := r.URL.Path[len(prefix):]
	if rest == "guide" && r.Method == http.MethodGet {
		s.guide(w, r)
		return
	}
	if rest == "" || rest == "api/texts" {
		if r.Method != http.MethodGet {
			http.Error(w, "not allowed", http.StatusMethodNotAllowed)
			return
		}
		if rest == "" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write(pageHTML)
			return
		}
		s.texts(w)
		return
	}
	if r.Method != http.MethodPost || r.Header.Get("X-NetDoctor") != "1" {
		http.Error(w, "not allowed", http.StatusForbidden)
		return
	}
	switch {
	case rest == "api/install" && s.mode == ModeInstall:
		s.install(w, r)
	case rest == "api/uninstall" && s.mode == ModeUninstall:
		s.uninstall(w, r)
	case rest == "api/shown":
		s.mu.Lock()
		if s.ran && s.shownAt.IsZero() {
			s.shownAt = time.Now()
		}
		s.mu.Unlock()
		w.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(w, r)
	}
}

// guide answers the GUIDE.md beside the installed program as plain text, only once the install on
// this page has finished well (ND-272, ND-279). The finish screen shows it, so a Mac user never has
// to look inside the app for it.
func (s *pageServer) guide(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	ok := s.guideOK
	s.mu.Unlock()
	if !ok || s.mode != ModeInstall {
		http.NotFound(w, r)
		return
	}
	f, err := os.Open(filepath.Join(s.l.ProgramDir, "GUIDE.md"))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	if fi, err := f.Stat(); err != nil || !fi.Mode().IsRegular() {
		http.NotFound(w, r)
		return
	}
	b, err := io.ReadAll(io.LimitReader(f, guideLimit))
	if err != nil {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Write(b)
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func (s *pageServer) texts(w http.ResponseWriter) {
	if s.mode == ModeUninstall {
		writeJSON(w, http.StatusOK, removalPageTexts(s.e.GOOS, s.l))
		return
	}
	writeJSON(w, http.StatusOK, pageTexts(s.e.GOOS, s.l))
}

// begin takes the page's one run; a second press, or one while the first works, is refused.
func (s *pageServer) begin() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ran || s.working {
		return false
	}
	s.working = true
	return true
}

func (s *pageServer) end() {
	s.mu.Lock()
	s.working, s.ran = false, true
	s.mu.Unlock()
}

type answer struct {
	OK    bool     `json:"ok"`
	Lines []string `json:"lines"`
}

func (s *pageServer) install(w http.ResponseWriter, r *http.Request) {
	var c Choice
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&c); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if c.OoklaOffer && !c.OoklaAgree {
		writeJSON(w, http.StatusBadRequest, answer{Lines: []string{ooklaNeedTick}})
		return
	}
	if !c.OoklaOffer {
		c.OoklaAgree = false
	}
	if !s.begin() {
		http.Error(w, "already done", http.StatusConflict)
		return
	}
	res := Install(s.e, c, s.log)
	s.end()
	if len(res.Problems) > 0 {
		writeJSON(w, http.StatusOK, answer{Lines: append([]string{txFailed}, res.Problems...)})
		return
	}
	s.mu.Lock()
	s.guideOK = true
	s.mu.Unlock()
	writeJSON(w, http.StatusOK, answer{OK: true, Lines: FinishLines(s.e.GOOS, s.l.ProgramDir, s.l.Results, c, res)})
}

func (s *pageServer) uninstall(w http.ResponseWriter, r *http.Request) {
	// The page asks nothing: its one button removes everything, the results too (ND-291), so
	// the answer is an empty object and any field in it is ignored.
	var q struct{}
	if err := json.NewDecoder(io.LimitReader(r.Body, 4096)).Decode(&q); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !s.begin() {
		http.Error(w, "already done", http.StatusConflict)
		return
	}
	rep, err := UninstallReport(s.e, false, s.log)
	s.end()
	settled := SettledLines(rep.Settled) // left as the person set it: said, never a fault (ND-316)
	var problems []string
	var nu *NotUndoneError
	if err != nil {
		for _, p := range strings.Split(err.Error(), "\n") {
			if p == probResultsName {
				problems = append(problems, p) // the one the person must know about
			} else if p != nuHead {
				problems = append(problems, probRemove)
			}
		}
	}
	if errors.As(err, &nu) && len(problems) == 0 {
		// Only a fix was not put back: the rest went, and the lines say which and what to do (ND-316).
		lines := append(append([]string{unDone}, settled...), NotUndoneLines(nu.Failures, nu.Notes, false)...)
		writeJSON(w, http.StatusOK, answer{OK: false, Lines: append(lines, txClosePage)})
		return
	}
	lines := removedLines(dedupe(problems))
	if len(settled) > 0 {
		last := lines[len(lines)-1]
		lines = append(append(lines[:len(lines)-1:len(lines)-1], settled...), last)
	}
	if nu != nil {
		lines = append(lines, NotUndoneLines(nu.Failures, nu.Notes, false)...)
	}
	writeJSON(w, http.StatusOK, answer{OK: err == nil, Lines: lines})
}

// dedupe keeps the first of each line, in order.
func dedupe(lines []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, l := range lines {
		if !seen[l] {
			seen[l] = true
			out = append(out, l)
		}
	}
	return out
}

// Serve shows the setup or removal page in the default browser, served on 127.0.0.1 at a port
// the system picks, and quits after the finish screen is shown or after Env.Quiet with no request.
func Serve(ctx context.Context, e Env, mode Mode, log io.Writer) error {
	e.fill()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return err
	}
	s := &pageServer{e: e, mode: mode, log: log, token: newToken(), l: LayoutFor(e),
		port: strconv.Itoa(ln.Addr().(*net.TCPAddr).Port), lastSeen: time.Now()}
	if mode == ModeUninstall && !installed(s.l) {
		s.ran = true // nothing to remove: the page says so, and showing that ends it
	}
	srv := &http.Server{Handler: s, ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := context.WithCancel(ctx)
	defer stop()
	go func() {
		t := time.NewTicker(max(10*time.Millisecond, min(e.Quiet, e.Grace)/4))
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				srv.Close()
				return
			case now := <-t.C:
				s.mu.Lock()
				quit := !s.working && (s.stopAsk || now.Sub(s.lastSeen) > e.Quiet ||
					(!s.shownAt.IsZero() && now.Sub(s.shownAt) > e.Grace))
				s.mu.Unlock()
				if quit {
					sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					srv.Shutdown(sctx)
					cancel()
					return
				}
			}
		}
	}()
	url := "http://127.0.0.1:" + s.port + "/" + s.token + "/"
	if e.OpenURL == nil || e.OpenURL(url) != nil {
		go func() {
			if e.NoBrowser != nil && e.NoBrowser(url, log) {
				s.mu.Lock()
				s.stopAsk = true // never in the middle of an install or removal
				s.mu.Unlock()
			} else if e.NoBrowser == nil {
				fmt.Fprintf(log, outNoBrowser+"\n", url)
			}
		}()
	}
	err = srv.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
