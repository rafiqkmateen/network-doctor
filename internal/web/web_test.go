package web

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// ND-81: local always, others only when the user allowed them.
func TestAllowedRemote(t *testing.T) {
	allow := []string{"192.168.1.0/24"}
	cases := map[string]bool{
		"127.0.0.1:5000":   true,
		"[::1]:5000":       true,
		"192.168.1.44:999": true,
		"192.168.2.44:999": false,
		"8.8.8.8:80":       false,
	}
	for addr, want := range cases {
		if got := AllowedRemote(addr, allow); got != want {
			t.Errorf("%s: got %v want %v", addr, got, want)
		}
	}
	if AllowedRemote("192.168.1.44:1", nil) {
		t.Error("with no allow list only this machine may connect")
	}
}

func TestHandlerRefusesBlockedAndServesAllowed(t *testing.T) {
	h := Handler(func() ([]byte, error) { return []byte(`{"ok":true}`), nil }, []string{"192.168.1.0/24"})
	do := func(remote string) int {
		req := httptest.NewRequest(http.MethodGet, "/api/check", nil)
		req.RemoteAddr = remote
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, req)
		return rec.Code
	}
	if do("127.0.0.1:1") != 200 || do("192.168.1.9:1") != 200 {
		t.Fatal("local and allowed must work")
	}
	if do("10.9.9.9:1") != http.StatusForbidden {
		t.Fatal("blocked address must be refused")
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:1"
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != 200 || rec.Body.Len() == 0 {
		t.Fatal("page should load")
	}
}

func TestListenGuard(t *testing.T) {
	if err := CheckListen("0.0.0.0:8765", nil); err == nil {
		t.Fatal("listening on all addresses needs an allow list")
	}
	if err := CheckListen("127.0.0.1:8765", nil); err != nil {
		t.Fatal("loopback needs none")
	}
	if err := CheckListen("0.0.0.0:8765", []string{"192.168.1.0/24"}); err != nil {
		t.Fatal(err)
	}
}
