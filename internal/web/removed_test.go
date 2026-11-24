package web

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// When the program is removed, the open page is retired with the reason "removed", and the page
// then says Network Doctor was removed instead of pointing to a newer page.
func TestRetiredBecauseRemoved(t *testing.T) {
	var removed, retired bool
	ctl := &openCtl{token: "tok",
		retire:      func() { retired = true },
		retired:     func() bool { return retired },
		told:        func() {},
		markRemoved: func() { removed = true },
		isRemoved:   func() bool { return removed },
	}
	h := openHandlerCtl(func() ([]byte, error) { return []byte("{}"), nil }, "9", func(string) {}, func(bool) {}, nil, ctl)
	req := httptest.NewRequest(http.MethodPost, "http://127.0.0.1:9/api/retire", nil)
	req.Host = "127.0.0.1:9"
	req.RemoteAddr = "127.0.0.1:5000"
	req.Header.Set(retireHeader, "tok")
	req.Header.Set(retireReasonHeader, "removed")
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	if rec.Code != http.StatusNoContent || !retired || !removed {
		t.Fatalf("retire: %d %v %v", rec.Code, retired, removed)
	}
	alive := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:9/api/alive", nil)
	alive.Host = "127.0.0.1:9"
	alive.RemoteAddr = "127.0.0.1:5000"
	alive.Header.Set("X-NetDoctor", "1")
	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, alive)
	body, _ := io.ReadAll(rec.Body)
	if rec.Code != http.StatusGone || strings.TrimSpace(string(body)) != "removed" {
		t.Errorf("heartbeat after removal: %d %q", rec.Code, body)
	}
	if !strings.Contains(string(page), "Network Doctor was removed from this computer.") ||
		!strings.Contains(string(page), "=== 'removed'") {
		t.Error("the page does not say Network Doctor was removed")
	}
}
