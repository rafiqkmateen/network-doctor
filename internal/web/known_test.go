package web

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type markCall struct {
	got  [][]Mark
	sets []Settings
}

func (m *markCall) saver() *Saver {
	return &Saver{
		Devices:  func(list []Mark) error { m.got = append(m.got, list); return nil },
		Settings: func(s Settings) error { m.sets = append(m.sets, s); return nil },
	}
}

// knownReq builds a POST to /api/known that passes every rule; each test breaks one.
func knownReq(body string) *http.Request {
	r := httptest.NewRequest(http.MethodPost, "/api/known", strings.NewReader(body))
	r.RemoteAddr, r.Host = "127.0.0.1:4000", "127.0.0.1:8765"
	r.Header.Set("Content-Type", "application/json")
	r.Header.Set("X-NetDoctor", "1")
	return r
}

const goodBody = `{"devices":[{"mac":"02-00-00-00-00-20","name":"phone","state":"mine"},{"mac":"02:00:00:00:00:31","name":"","state":"not-mine"}]}`

func serveKnown(h http.Handler, r *http.Request) int {
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec.Code
}

// ND-264: a good request from this computer saves exactly the devices sent, with their names.
func TestKnownAcceptsAGoodRequest(t *testing.T) {
	var m markCall
	h := servingHandler(okRun, nil, nil, m.saver())
	if code := serveKnown(h, knownReq(goodBody)); code != 200 {
		t.Fatalf("a good request must be accepted, got %d", code)
	}
	if len(m.got) != 1 || len(m.got[0]) != 2 {
		t.Fatalf("want one save of two devices, got %v", m.got)
	}
	if m.got[0][0] != (Mark{MAC: "02:00:00:00:00:20", Name: "phone", State: "mine"}) || m.got[0][1] != (Mark{MAC: "02:00:00:00:00:31", State: "not-mine"}) {
		t.Errorf("saved %v", m.got[0])
	}
}

// ND-264: each rule refuses on its own, and nothing is saved.
func TestKnownRefusesEverythingElse(t *testing.T) {
	var m markCall
	h := servingHandler(okRun, []string{"192.168.1.0/24"}, nil, m.saver())
	cases := map[string]func() *http.Request{
		"another device, even an allowed one": func() *http.Request { r := knownReq(goodBody); r.RemoteAddr = "192.168.1.9:4000"; return r },
		"no X-NetDoctor header":               func() *http.Request { r := knownReq(goodBody); r.Header.Del("X-NetDoctor"); return r },
		"a plain form post": func() *http.Request {
			r := knownReq("devices=02:00:00:00:00:20")
			r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
			r.Header.Del("X-NetDoctor")
			return r
		},
		"text content type":   func() *http.Request { r := knownReq(goodBody); r.Header.Set("Content-Type", "text/plain"); return r },
		"a foreign Host":      func() *http.Request { r := knownReq(goodBody); r.Host = "evil.example:8765"; return r },
		"a home-network Host": func() *http.Request { r := knownReq(goodBody); r.Host = "192.168.1.20:8765"; return r },
		"a bad address": func() *http.Request {
			return knownReq(`{"devices":[{"mac":"02:00:00:00:00:2g","name":"","state":"mine"}]}`)
		},
		"a short address": func() *http.Request {
			return knownReq(`{"devices":[{"mac":"02:00:00:00:00","name":"","state":"mine"}]}`)
		},
		"an unknown field": func() *http.Request {
			return knownReq(`{"devices":[{"mac":"02:00:00:00:00:20","state":"mine","path":"x"}]}`)
		},
		"no devices":       func() *http.Request { return knownReq(`{"devices":[]}`) },
		"not JSON":         func() *http.Request { return knownReq(`devices`) },
		"a GET":            func() *http.Request { r := knownReq(""); r.Method = http.MethodGet; return r },
		"too many devices": func() *http.Request { return knownReq(manyDevices(300)) },
		"a name that is not text": func() *http.Request {
			return knownReq(`{"devices":[{"mac":"02:00:00:00:00:20","name":5,"state":"mine"}]}`)
		},
		"an unknown state": func() *http.Request {
			return knownReq(`{"devices":[{"mac":"02:00:00:00:00:20","name":"","state":"owner"}]}`)
		},
		"no state": func() *http.Request { return knownReq(`{"devices":[{"mac":"02:00:00:00:00:20","name":""}]}`) },
	}
	for name, mk := range cases {
		if code := serveKnown(h, mk()); code == 200 {
			t.Errorf("%s: must be refused, got 200", name)
		}
	}
	if len(m.got) != 0 {
		t.Errorf("nothing may be saved by a refused request: %v", m.got)
	}
}

func manyDevices(n int) string {
	var b strings.Builder
	b.WriteString(`{"devices":[`)
	for i := 0; i < n; i++ {
		if i > 0 {
			b.WriteString(",")
		}
		fmt.Fprintf(&b, `{"mac":"02:00:00:00:%02x:%02x","name":"","state":"mine"}`, i/256, i%256)
	}
	b.WriteString("]}")
	return b.String()
}

// ND-264: names reach the list as plain text: cut to 40 characters, control characters removed,
// markup kept as the characters typed.
func TestKnownCleansNames(t *testing.T) {
	var m markCall
	h := servingHandler(okRun, nil, nil, m.saver())
	body := `{"devices":[{"mac":"02:00:00:00:00:20","name":"` + strings.Repeat("a", 70) + `","state":"mine"},` +
		`{"mac":"02:00:00:00:00:21","name":"tv\n\u0007room","state":"mine"},{"mac":"02:00:00:00:00:22","name":"<img src=x onerror=alert(1)>","state":"mine"}]}`
	if code := serveKnown(h, knownReq(body)); code != 200 {
		t.Fatalf("names that need cleaning are cleaned, not refused: got %d", code)
	}
	got := m.got[0]
	if len([]rune(got[0].Name)) != 40 || got[1].Name != "tvroom" || got[2].Name != "<img src=x onerror=alert(1)>" {
		t.Errorf("names saved: %q", []string{got[0].Name, got[1].Name, got[2].Name})
	}
}

// ND-264: with no way to save (the share page), the address does not exist.
func TestKnownIsOffWithoutASaver(t *testing.T) {
	h := servingHandler(okRun, nil, nil, nil)
	if code := serveKnown(h, knownReq(goodBody)); code == 200 {
		t.Error("with no saver the request must be refused")
	}
}

// ND-264: the open page takes the same request, with its own strict Host rule.
func TestKnownOnTheOpenPage(t *testing.T) {
	var m markCall
	h := openHandler(okRun, "50000", func(string) {}, func(bool) {}, m.saver())
	r := knownReq(goodBody)
	r.Host = "127.0.0.1:50000"
	if code := serveKnown(h, r); code != 200 {
		t.Errorf("the open page must accept a good request, got %d", code)
	}
	r = knownReq(goodBody)
	r.Host = "evil.example:50000"
	if code := serveKnown(h, r); code != http.StatusForbidden {
		t.Errorf("a foreign Host must be refused, got %d", code)
	}
}

// ND-264: undecided puts a device back (used to bring back an ignored one).
func TestKnownAcceptsUndecided(t *testing.T) {
	var m markCall
	h := servingHandler(okRun, nil, nil, m.saver())
	body := `{"devices":[{"mac":"02:00:00:00:00:20","name":"","state":"undecided"},{"mac":"02:00:00:00:00:21","name":"","state":"ignored"}]}`
	if code := serveKnown(h, knownReq(body)); code != 200 {
		t.Fatalf("undecided and ignored are real choices, got %d", code)
	}
	if m.got[0][0].State != "undecided" || m.got[0][1].State != "ignored" {
		t.Errorf("saved %v", m.got[0])
	}
}

func settingsReq(body string) *http.Request {
	r := knownReq(body)
	r.URL.Path = "/api/settings"
	return r
}

// ND-265: the new-device switch is saved through the same protections as the device list.
func TestSettingsSwitch(t *testing.T) {
	var m markCall
	h := servingHandler(okRun, []string{"192.168.1.0/24"}, nil, m.saver())
	if code := serveKnown(h, settingsReq(`{"warn_new_devices":false}`)); code != 200 {
		t.Fatalf("a good request must be accepted, got %d", code)
	}
	if len(m.sets) != 1 || m.sets[0].WarnNewDevices {
		t.Fatalf("want one save with warnings off, got %v", m.sets)
	}
	bad := map[string]func() *http.Request{
		"another device": func() *http.Request {
			r := settingsReq(`{"warn_new_devices":true}`)
			r.RemoteAddr = "192.168.1.9:1"
			return r
		},
		"no header": func() *http.Request {
			r := settingsReq(`{"warn_new_devices":true}`)
			r.Header.Del("X-NetDoctor")
			return r
		},
		"form post": func() *http.Request {
			r := settingsReq(`{"warn_new_devices":true}`)
			r.Header.Set("Content-Type", "text/plain")
			return r
		},
		"foreign Host": func() *http.Request {
			r := settingsReq(`{"warn_new_devices":true}`)
			r.Host = "evil.example:8765"
			return r
		},
		"missing value":    func() *http.Request { return settingsReq(`{}`) },
		"not a true/false": func() *http.Request { return settingsReq(`{"warn_new_devices":"no"}`) },
		"unknown field":    func() *http.Request { return settingsReq(`{"warn_new_devices":true,"sinks":["file"]}`) },
	}
	for name, mk := range bad {
		if code := serveKnown(h, mk()); code == 200 {
			t.Errorf("%s: must be refused, got 200", name)
		}
	}
	if len(m.sets) != 1 {
		t.Errorf("refused requests must save nothing: %v", m.sets)
	}
}

// ND-263: starting a check or keeping the open page alive needs the page's own header, or a browser
// that says the request comes from the same page, so another website can do neither.
func TestCheckAndAliveNeedThePage(t *testing.T) {
	h := servingHandler(okRun, nil, nil, nil)
	for _, path := range []string{"/api/check", "/api/alive"} {
		do := func(set func(*http.Request)) int {
			r := httptest.NewRequest(http.MethodGet, path, nil)
			r.RemoteAddr, r.Host = "127.0.0.1:4000", "127.0.0.1:8765"
			set(r)
			return serveKnown(h, r)
		}
		if c := do(func(*http.Request) {}); c != http.StatusForbidden {
			t.Errorf("%s with no header: %d, want 403", path, c)
		}
		if c := do(func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "cross-site") }); c != http.StatusForbidden {
			t.Errorf("%s from another site: %d, want 403", path, c)
		}
		if c := do(func(r *http.Request) { r.Header.Set("X-NetDoctor", "1") }); c != 200 {
			t.Errorf("%s with the page's header: %d, want 200", path, c)
		}
		if c := do(func(r *http.Request) { r.Header.Set("Sec-Fetch-Site", "same-origin") }); c != 200 {
			t.Errorf("%s from the same page: %d, want 200", path, c)
		}
	}
	r := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	r.RemoteAddr, r.Host = "127.0.0.1:4000", "127.0.0.1:8765"
	if c := serveKnown(h, r); c != 200 {
		t.Errorf("/api/health stays open: %d", c)
	}
}

// ND-264: the check's answer tells the page whether it can save, so it never shows buttons that
// would be refused: only with a saver, from this computer, at an address naming this computer.
func TestCheckSaysWhetherThePageCanSave(t *testing.T) {
	var m markCall
	can := func(sv *Saver, host string) string {
		h := servingHandler(okRun, []string{"192.168.1.0/24"}, nil, sv)
		r := httptest.NewRequest(http.MethodGet, "/api/check", nil)
		r.RemoteAddr, r.Host = "127.0.0.1:4000", host
		r.Header.Set("X-NetDoctor", "1")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		return rec.Header().Get("X-NetDoctor-Save")
	}
	if can(m.saver(), "127.0.0.1:8765") != "1" || can(m.saver(), "localhost:8765") != "1" {
		t.Error("the page on this computer with a saver can save")
	}
	if can(nil, "127.0.0.1:8765") != "" {
		t.Error("with no saver (share) the page must not offer to save")
	}
	if can(m.saver(), "192.168.1.20:8765") != "" {
		t.Error("at the home-network address the save would be refused, so it must not be offered")
	}
}

// ND-263: no other website may show the page inside a frame: every response of every page server
// says so, errors included.
func TestEveryResponseRefusesFrames(t *testing.T) {
	var m markCall
	handlers := map[string]http.Handler{
		"serve": servingHandler(okRun, nil, nil, m.saver()),
		"share": servingHandler(okRun, []string{"192.168.1.0/24"}, nil, nil),
		"open":  openHandler(okRun, "8765", func(string) {}, func(bool) {}, m.saver()),
		"plain": HandlerWith(okRun, nil, nil),
	}
	reqs := map[string]func() *http.Request{
		"page": func() *http.Request { return httptest.NewRequest(http.MethodGet, "/", nil) },
		"api": func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/api/check", nil)
			r.Header.Set("X-NetDoctor", "1")
			return r
		},
		"404": func() *http.Request { return httptest.NewRequest(http.MethodGet, "/nothing", nil) },
		"403 host": func() *http.Request {
			r := httptest.NewRequest(http.MethodGet, "/", nil)
			r.Host = "evil.example:8765"
			return r
		},
		"403 header": func() *http.Request { return httptest.NewRequest(http.MethodGet, "/api/alive", nil) },
	}
	for hn, h := range handlers {
		for rn, mk := range reqs {
			r := mk()
			r.RemoteAddr = "127.0.0.1:4000"
			if r.Host == "example.com" {
				r.Host = "127.0.0.1:8765"
			}
			rec := httptest.NewRecorder()
			h.ServeHTTP(rec, r)
			if rec.Header().Get("X-Frame-Options") != "DENY" || rec.Header().Get("Content-Security-Policy") != "frame-ancestors 'none'" {
				t.Errorf("%s, %s (status %d): frame headers missing: %v", hn, rn, rec.Code, rec.Header())
			}
		}
	}
}
