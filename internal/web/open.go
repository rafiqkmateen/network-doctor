package web

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// hostAllowed says whether the Host header a browser sent names this computer. With a port (the
// open command) only localhost, 127.0.0.1 or [::1] with that exact port pass. With no port (serve
// and share) localhost or any address written as numbers pass, so phones on the home network still
// work. Either way a made-up name fails, which is what a DNS trick from a website needs (ND-263).
func hostAllowed(host, port string) bool {
	h, p, err := net.SplitHostPort(host)
	if port != "" {
		if err != nil || p != port {
			return false
		}
		h = strings.ToLower(h)
		return h == "localhost" || h == "127.0.0.1" || h == "::1"
	}
	if err != nil {
		h = strings.TrimSuffix(strings.TrimPrefix(host, "["), "]")
	}
	if h == "" {
		return false
	}
	return strings.EqualFold(h, "localhost") || net.ParseIP(h) != nil
}

// hostGuard refuses any request whose Host header hostAllowed rejects.
func hostGuard(next http.Handler, port string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		noFrames(w)
		if !hostAllowed(r.Host, port) {
			http.Error(w, "not allowed", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// servingHandler is the handler the serve and share pages run: HandlerWith behind the Host check.
func servingHandler(run func() ([]byte, error), allow []string, onRemote func(ip string), sv *Saver) http.Handler {
	return hostGuard(HandlerSave(run, allow, onRemote, sv), "")
}

// OpenOptions control ServeOpen. Zero durations take the defaults.
type OpenOptions struct {
	// Idle: quit when the page has loaded but has not checked in for this long (default 3 minutes;
	// a tab in the background checks in less often).
	Idle time.Duration
	// NoPage: quit when the page never loads within this long (default 5 minutes).
	NoPage time.Duration
	// Ready is called once the port is open, with the page's address.
	Ready func(url string)
	// Save takes the device choices and the new-device switch; nil refuses them.
	Save *Saver
	// StateFile, when set, is where this run notes its port and a random token, so the next press
	// can retire it: one page at a time. Written whole at once and removed on quit.
	StateFile string
	// Grace: how long a retired server waits for its page to hear before quitting (default
	// defaultGrace). It quits at once when its page has been told.
	Grace time.Duration
}

// retireHeader carries the token a new open run uses to retire the old one. A website cannot send
// a custom header to this computer without the browser asking first, and the token is secret.
const retireHeader = "X-NetDoctor-Retire"

// retireReasonHeader, set to "removed" by the uninstall, makes the old page say Network Doctor was
// removed instead of pointing to a newer page.
const retireReasonHeader = "X-NetDoctor-Retire-Reason"

// defaultGrace outlasts a hidden tab's heartbeat (browsers slow it to about once a minute), so the
// old page hears "retired" instead of finding Network Doctor gone with old numbers on screen.
const defaultGrace = 75 * time.Second

// lockState takes the lock file next to the state file, so two presses at once take turns: the
// second finds the first one's server and retires it. It waits up to about 10 seconds, takes over
// a lock older than 30 seconds, and never blocks the run: the returned func removes the lock.
func lockState(path string) func() {
	lock := path + ".lock"
	os.MkdirAll(filepath.Dir(path), 0o700)
	end := time.Now().Add(10 * time.Second)
	for {
		f, err := os.OpenFile(lock, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			f.Close()
			return func() { os.Remove(lock) }
		}
		if fi, serr := os.Stat(lock); serr == nil && time.Since(fi.ModTime()) > 30*time.Second {
			os.Remove(lock) // left by a stopped program
			continue
		}
		if time.Now().After(end) {
			return func() {}
		}
		time.Sleep(25 * time.Millisecond)
	}
}

// openState is what the state file holds.
type openState struct {
	Port  string `json:"port"`
	Token string `json:"token"`
}

// openCtl lets the open handler take a retire request and tell the old page it is retired.
type openCtl struct {
	token   string
	retire  func()
	retired func() bool
	told    func()
	// markRemoved and isRemoved, when set, note that the retire came from the uninstall.
	markRemoved func()
	isRemoved   func() bool
}

// retireOld asks the open server named in the state file, if any, to retire. Anything wrong with
// the file or no answer is ignored: a stale or foreign file never blocks the new run.
func retireOld(path string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var st openState
	if json.Unmarshal(b, &st) != nil || st.Token == "" {
		return
	}
	if n, err := strconv.Atoi(st.Port); err != nil || n < 1 || n > 65535 {
		return
	}
	req, err := http.NewRequest(http.MethodPost, "http://127.0.0.1:"+st.Port+"/api/retire", nil)
	if err != nil {
		return
	}
	req.Header.Set(retireHeader, st.Token)
	c := &http.Client{Timeout: 2 * time.Second}
	if res, err := c.Do(req); err == nil {
		res.Body.Close()
	}
}

// writeState writes the state file whole at once: a temporary file in the same folder, then a rename.
func writeState(path string, st openState) error {
	b, err := json.Marshal(st)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	f, err := os.CreateTemp(filepath.Dir(path), ".open-*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(b)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		os.Remove(tmp)
		return errors.Join(werr, cerr)
	}
	if err := os.Rename(tmp, path); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// removeState removes the state file only while it still names this run.
func removeState(path, token string) {
	b, err := os.ReadFile(path)
	if err != nil {
		return
	}
	var st openState
	if json.Unmarshal(b, &st) == nil && st.Token == token {
		os.Remove(path)
	}
}

func newToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// openHandler serves the page for the open command: only this computer, only with a Host header that
// names this computer and port. touch hears each allowed request's path; busy hears a check start and end.
func openHandler(run func() ([]byte, error), port string, touch func(path string), busy func(bool), sv *Saver) http.Handler {
	return openHandlerCtl(run, port, touch, busy, sv, nil)
}

// openHandlerCtl is openHandler with the retire controls; ctl nil turns retiring off.
func openHandlerCtl(run func() ([]byte, error), port string, touch func(path string), busy func(bool),
	sv *Saver, ctl *openCtl) http.Handler {
	counted := func() ([]byte, error) {
		busy(true)
		defer busy(false)
		return run()
	}
	inner := HandlerSave(counted, nil, nil, sv)
	return hostGuard(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !AllowedRemote(r.RemoteAddr, nil) {
			http.Error(w, "not allowed", http.StatusForbidden)
			return
		}
		if r.URL.Path == "/api/retire" {
			sfs := r.Header.Get("Sec-Fetch-Site")
			tok := r.Header.Get(retireHeader)
			if ctl == nil || ctl.token == "" || r.Method != http.MethodPost || r.Header.Get("Origin") != "" ||
				(sfs != "" && sfs != "none") || subtle.ConstantTimeCompare([]byte(tok), []byte(ctl.token)) != 1 {
				http.Error(w, "not allowed", http.StatusForbidden)
				return
			}
			if r.Header.Get(retireReasonHeader) == "removed" && ctl.markRemoved != nil {
				ctl.markRemoved()
			}
			ctl.retire()
			w.WriteHeader(http.StatusNoContent)
			return
		}
		if ctl != nil && ctl.retired() && fromPage(r) &&
			(r.URL.Path == "/api/alive" || r.URL.Path == "/api/check") {
			why := "retired" // the page shows the newer-page notice
			if ctl.isRemoved != nil && ctl.isRemoved() {
				why = "removed" // the page says Network Doctor was removed
			}
			http.Error(w, why, http.StatusGone)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			ctl.told()
			return
		}
		if fromPage(r) {
			touch(r.URL.Path) // only the page's own requests keep the server open, never a plain load
		}
		inner.ServeHTTP(w, r)
	}), port)
}

// ServeOpen runs the page on 127.0.0.1 at a port the system picks, and quits by itself when the page
// is gone: no heartbeat for Idle after it loaded, or no page at all within NoPage. It never quits
// while a check runs. A normal quit returns nil (ND-263).
func ServeOpen(ctx context.Context, run func() ([]byte, error), o OpenOptions) error {
	if o.Idle <= 0 {
		o.Idle = 3 * time.Minute
	}
	if o.NoPage <= 0 {
		o.NoPage = 5 * time.Minute
	}
	if o.Grace <= 0 {
		o.Grace = defaultGrace
	}
	unlock := func() {}
	if o.StateFile != "" {
		unlock = lockState(o.StateFile) // held over read old, retire old, write new
		retireOld(o.StateFile)          // one page at a time: the older one gives way
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		unlock()
		return err
	}
	port := strconv.Itoa(ln.Addr().(*net.TCPAddr).Port)

	var (
		mu        sync.Mutex
		started   = time.Now()
		lastSeen  time.Time // zero until the page loads or checks in
		running   int
		retiredAt time.Time // zero until a newer run retires this one
		removed   bool      // the retire came from the uninstall
		told      bool      // the page heard it is retired
	)
	token := ""
	if o.StateFile != "" {
		token = newToken()
	}
	ctl := &openCtl{token: token,
		retire:      func() { mu.Lock(); retiredAt = time.Now(); mu.Unlock() },
		retired:     func() bool { mu.Lock(); defer mu.Unlock(); return !retiredAt.IsZero() },
		told:        func() { mu.Lock(); told = true; mu.Unlock() },
		markRemoved: func() { mu.Lock(); removed = true; mu.Unlock() },
		isRemoved:   func() bool { mu.Lock(); defer mu.Unlock(); return removed },
	}
	touch := func(string) { // a request from the page itself shows it is still open
		mu.Lock()
		lastSeen = time.Now()
		mu.Unlock()
	}
	busy := func(on bool) {
		mu.Lock()
		if on {
			running++
		} else {
			running--
			lastSeen = time.Now()
		}
		mu.Unlock()
	}
	srv := &http.Server{Handler: openHandlerCtl(run, port, touch, busy, o.Save, ctl),
		ReadHeaderTimeout: 5 * time.Second}

	step := min(o.Idle, o.NoPage) / 4
	step = max(10*time.Millisecond, min(step, time.Second))
	stop := make(chan struct{})
	loopDone := make(chan struct{})
	go func() {
		defer close(loopDone)
		t := time.NewTicker(step)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				srv.Close()
				return
			case <-stop:
				return
			case now := <-t.C:
				mu.Lock()
				// A retired server finishes any running check, then quits once its page has heard,
				// or after Grace if the page never asks; answers in flight are let out first.
				retire := running == 0 && !retiredAt.IsZero() && (told || now.Sub(retiredAt) > o.Grace)
				gone := running == 0 && ((lastSeen.IsZero() && now.Sub(started) > o.NoPage) ||
					(!lastSeen.IsZero() && now.Sub(lastSeen) > o.Idle))
				mu.Unlock()
				if retire {
					sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
					srv.Shutdown(sctx)
					cancel()
					return
				}
				if gone {
					srv.Close()
					return
				}
			}
		}
	}()
	if o.StateFile != "" {
		writeState(o.StateFile, openState{Port: port, Token: token}) // a failed write only loses the retire
		defer removeState(o.StateFile, token)
	}
	unlock()
	if o.Ready != nil {
		o.Ready("http://127.0.0.1:" + port + "/")
	}
	err = srv.Serve(ln)
	close(stop)
	<-loopDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
