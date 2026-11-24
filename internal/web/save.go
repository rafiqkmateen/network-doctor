package web

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"mime"
	"net"
	"net/http"
	"strings"

	"github.com/rafiqkmateen/network-doctor/internal/core"
)

// Mark is one device choice sent by the page: the address, the choice ("mine", "not-mine",
// "ignored" or "undecided") and an optional name, already cleaned to plain text (ND-264).
type Mark struct {
	MAC   string `json:"mac"`
	Name  string `json:"name"`
	State string `json:"state"`
}

// Settings is what the page's Settings area can change: only the new-device switch (ND-265).
type Settings struct {
	WarnNewDevices bool `json:"warn_new_devices"`
}

// Saver writes what the page sends. The web package checks every request; the caller only stores.
type Saver struct {
	Devices  func([]Mark) error
	Settings func(Settings) error
}

// MaxMarks is the most devices one request may carry.
const MaxMarks = 256

var markStates = map[string]bool{"mine": true, "not-mine": true, "ignored": true, "undecided": true}

// fromThisPage lets a save through only when it surely comes from the page on this computer:
// a POST, from this computer, to an address that names this computer, with the page's own
// header and a JSON body. A form on another website can send none of the last two, and a
// browser refuses to add them for another site without asking this server first, which says no.
func fromThisPage(w http.ResponseWriter, r *http.Request) bool {
	refuse := func(code int, msg string) bool { http.Error(w, msg, code); return false }
	if r.Method != http.MethodPost {
		return refuse(http.StatusMethodNotAllowed, "use POST")
	}
	if !AllowedRemote(r.RemoteAddr, nil) {
		return refuse(http.StatusForbidden, "only the page on this computer can save")
	}
	if !namesThisComputer(r.Host) {
		return refuse(http.StatusForbidden, "not allowed")
	}
	if r.Header.Get("X-NetDoctor") != "1" {
		return refuse(http.StatusForbidden, "not allowed")
	}
	if mt, _, err := mime.ParseMediaType(r.Header.Get("Content-Type")); err != nil || mt != "application/json" {
		return refuse(http.StatusUnsupportedMediaType, "send JSON")
	}
	return true
}

// namesThisComputer is true when a Host header names this computer: localhost, 127.0.0.1 or [::1].
func namesThisComputer(host string) bool {
	h, _, err := net.SplitHostPort(host)
	if err != nil {
		h = host
	}
	h = strings.ToLower(h)
	return h == "localhost" || h == "127.0.0.1" || h == "::1"
}

// decodeStrict reads one JSON value with no unknown fields and nothing after it.
func decodeStrict(w http.ResponseWriter, r *http.Request, v any) error {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 64<<10))
	if err != nil {
		return err
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		return err
	}
	if dec.More() {
		return errors.New("more than one value")
	}
	return nil
}

func knownHandler(sv *Saver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sv == nil || sv.Devices == nil {
			http.NotFound(w, r)
			return
		}
		if !fromThisPage(w, r) {
			return
		}
		var req struct {
			Devices []Mark `json:"devices"`
		}
		if err := decodeStrict(w, r, &req); err != nil {
			http.Error(w, "not a list of devices", http.StatusBadRequest)
			return
		}
		if len(req.Devices) == 0 || len(req.Devices) > MaxMarks {
			http.Error(w, "send between 1 and 256 devices", http.StatusBadRequest)
			return
		}
		list := make([]Mark, 0, len(req.Devices))
		for _, m := range req.Devices {
			if !core.ValidMAC(m.MAC) || !markStates[m.State] {
				http.Error(w, "not a device address or not a choice", http.StatusBadRequest)
				return
			}
			list = append(list, Mark{MAC: core.NormalizeMAC(m.MAC), Name: core.CleanName(m.Name), State: m.State})
		}
		if err := sv.Devices(list); err != nil {
			http.Error(w, `{"error":"could not save"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}

func settingsHandler(sv *Saver) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if sv == nil || sv.Settings == nil {
			http.NotFound(w, r)
			return
		}
		if !fromThisPage(w, r) {
			return
		}
		var req struct {
			Warn *bool `json:"warn_new_devices"`
		}
		if err := decodeStrict(w, r, &req); err != nil || req.Warn == nil {
			http.Error(w, "send warn_new_devices as true or false", http.StatusBadRequest)
			return
		}
		if err := sv.Settings(Settings{WarnNewDevices: *req.Warn}); err != nil {
			http.Error(w, `{"error":"could not save"}`, http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"ok":true}`))
	}
}
