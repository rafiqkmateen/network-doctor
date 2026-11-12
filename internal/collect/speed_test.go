package collect

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync/atomic"
	"testing"
	"time"
)

// fakeSpeedServer stands in for the public test server. It records how many
// downloads and uploads ran at the same moment.
func fakeSpeedServer(t *testing.T) (url string, peakDown, peakUp *int32) {
	t.Helper()
	var curDown, curUp, pDown, pUp int32
	track := func(cur, peak *int32) func() {
		n := atomic.AddInt32(cur, 1)
		for {
			p := atomic.LoadInt32(peak)
			if n <= p || atomic.CompareAndSwapInt32(peak, p, n) {
				break
			}
		}
		return func() { atomic.AddInt32(cur, -1) }
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/__down", func(w http.ResponseWriter, r *http.Request) {
		defer track(&curDown, &pDown)()
		total, _ := strconv.Atoi(r.URL.Query().Get("bytes"))
		chunk := make([]byte, 64*1024)
		for sent := 0; sent < total; sent += len(chunk) {
			if _, err := w.Write(chunk); err != nil {
				return
			}
		}
	})
	mux.HandleFunc("/__up", func(w http.ResponseWriter, r *http.Request) {
		defer track(&curUp, &pUp)()
		io.Copy(io.Discard, r.Body)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv.URL, &pDown, &pUp
}

// ND-11: the speed test opens several connections at once, so a fast line is filled.
func TestSpeedUsesSeveralConnections(t *testing.T) {
	url, peakDown, peakUp := fakeSpeedServer(t)
	start := time.Now()
	down, up, err := runSpeed(context.Background(), url, speedPlan{Streams: 6, Warmup: 100 * time.Millisecond, Measure: 600 * time.Millisecond})
	if err != nil {
		t.Fatal(err)
	}
	if down <= 0 || up <= 0 {
		t.Fatalf("want both speeds above zero, got down %.1f up %.1f", down, up)
	}
	if got := atomic.LoadInt32(peakDown); got < 4 {
		t.Errorf("downloads at the same time: %d, want at least 4", got)
	}
	if got := atomic.LoadInt32(peakUp); got < 4 {
		t.Errorf("uploads at the same time: %d, want at least 4", got)
	}
	if took := time.Since(start); took > 4*time.Second {
		t.Errorf("test ran %v, want a few seconds at most", took)
	}
}

// ND-11: the default plan is long enough to leave the slow start behind and short
// enough to stay light on the line.
func TestDefaultSpeedPlan(t *testing.T) {
	p := defaultSpeedPlan
	if p.Streams < 4 || p.Streams > 8 {
		t.Errorf("streams %d, want 4 to 8", p.Streams)
	}
	if p.Measure < 3*time.Second || p.Measure > 8*time.Second {
		t.Errorf("measure time %v, want 3 to 8 seconds", p.Measure)
	}
	if p.Warmup <= 0 {
		t.Error("want a warm-up before counting")
	}
}

// ND-180: when the server answers "too many requests", the error says so and carries the wait.
func TestSpeedServerRefusesWithAWait(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/__down", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Retry-After", "120")
		w.WriteHeader(http.StatusTooManyRequests)
	})
	mux.HandleFunc("/__up", func(w http.ResponseWriter, r *http.Request) {})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	_, _, err := runSpeed(context.Background(), srv.URL, speedPlan{Streams: 4, Warmup: 50 * time.Millisecond, Measure: 200 * time.Millisecond})
	var refused *SpeedRefusedError
	if !errors.As(err, &refused) {
		t.Fatalf("want a refusal error, got %v", err)
	}
	if refused.RetryAfter != 120*time.Second {
		t.Errorf("wait %v, want 2m0s", refused.RetryAfter)
	}
}

// ND-183: the default test is light on the public server: few connections, a few seconds.
func TestDefaultSpeedPlanIsLight(t *testing.T) {
	p := defaultSpeedPlan
	if p.Streams > 4 {
		t.Errorf("streams %d, want 4 or fewer", p.Streams)
	}
	if p.Measure > 3*time.Second {
		t.Errorf("measure time %v, want 3 seconds or less", p.Measure)
	}
}

// ND-03: with no server, the test says so rather than reporting zeros as a result.
func TestSpeedServerDown(t *testing.T) {
	srv := httptest.NewServer(http.NotFoundHandler())
	url := srv.URL
	srv.Close()
	_, _, err := runSpeed(context.Background(), url, speedPlan{Streams: 4, Warmup: 50 * time.Millisecond, Measure: 200 * time.Millisecond})
	if err == nil {
		t.Fatal("want an error when the server cannot be reached")
	}
}
