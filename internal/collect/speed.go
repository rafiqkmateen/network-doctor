package collect

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"
)

// speedBase is the public test server. Speed is one of the two things that go
// online; the other is the internet ping.
const speedBase = "https://speed.cloudflare.com"

// speedPlan says how hard and how long to test. One connection cannot fill a
// fast line, so the test runs several at once and counts bytes over a few
// seconds, after a short warm-up that leaves the slow start behind.
type speedPlan struct {
	Streams int
	Warmup  time.Duration
	Measure time.Duration
}

// Four connections for three seconds: enough to fill a fast line, and light on a
// public server that refuses a client that asks for too much (ND-183).
var defaultSpeedPlan = speedPlan{Streams: 4, Warmup: time.Second, Measure: 3 * time.Second}

// SpeedRefusedError means the test server answered "too many requests". It asks
// for a wait, and the number is how long. The speed is then not measured: a
// caller must not show it as 0.
type SpeedRefusedError struct {
	RetryAfter time.Duration // zero when the server did not say
}

func (e *SpeedRefusedError) Error() string { return "the speed server asked for a wait" }

// speedStatus turns a bad answer from the test server into an error.
func speedStatus(resp *http.Response) error {
	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	case http.StatusTooManyRequests:
		secs, _ := strconv.Atoi(resp.Header.Get("Retry-After"))
		return &SpeedRefusedError{RetryAfter: time.Duration(secs) * time.Second}
	}
	return fmt.Errorf("speed server answered %s", resp.Status)
}

// doSpeed measures download and upload in Mbps against the public test server.
func doSpeed(ctx context.Context) (down, up float64, err error) {
	return runSpeed(ctx, speedBase, defaultSpeedPlan)
}

func runSpeed(ctx context.Context, base string, p speedPlan) (down, up float64, err error) {
	client := &http.Client{} // the plan's clock ends each run, not a per-request timeout

	down, err = measureStreams(ctx, p, func(ctx context.Context, add func(int64)) error {
		return downloadOnce(ctx, client, base, add)
	})
	if err != nil {
		return 0, 0, err
	}
	up, err = measureStreams(ctx, p, func(ctx context.Context, add func(int64)) error {
		return uploadOnce(ctx, client, base, add)
	})
	if err != nil {
		return down, 0, err
	}
	return down, up, nil
}

// measureStreams runs the worker on p.Streams goroutines until the plan's time
// is up and returns the average Mbps counted after the warm-up.
func measureStreams(ctx context.Context, p speedPlan, worker func(context.Context, func(int64)) error) (float64, error) {
	start := time.Now()
	warmEnd := start.Add(p.Warmup)
	end := warmEnd.Add(p.Measure)
	cctx, cancel := context.WithDeadline(ctx, end)
	defer cancel()

	var total, counted int64
	add := func(n int64) {
		atomic.AddInt64(&total, n)
		if time.Now().After(warmEnd) {
			atomic.AddInt64(&counted, n)
		}
	}

	var wg sync.WaitGroup
	var errMu sync.Mutex
	var firstErr error
	for i := 0; i < p.Streams; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for cctx.Err() == nil {
				if err := worker(cctx, add); err != nil {
					if cctx.Err() == nil {
						errMu.Lock()
						if firstErr == nil {
							firstErr = err
						}
						errMu.Unlock()
					}
					return
				}
			}
		}()
	}
	wg.Wait()

	if atomic.LoadInt64(&total) == 0 {
		if firstErr != nil {
			return 0, firstErr
		}
		return 0, errors.New("no data moved during the speed test")
	}
	now := time.Now()
	if now.After(end) {
		now = end
	}
	secs := now.Sub(warmEnd).Seconds()
	if secs <= 0 {
		return 0, errors.New("speed test ended before the warm-up was over")
	}
	return float64(atomic.LoadInt64(&counted)) * 8 / secs / 1e6, nil
}

func downloadOnce(ctx context.Context, client *http.Client, base string, add func(int64)) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/__down?bytes=25000000", nil)
	if err != nil {
		return err
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if err := speedStatus(resp); err != nil {
		return err
	}
	buf := make([]byte, 64*1024)
	for {
		n, err := resp.Body.Read(buf)
		add(int64(n))
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
	}
}

// zeroReader gives an endless run of zero bytes and counts what is taken.
type zeroReader struct {
	left int64
	add  func(int64)
}

func (z *zeroReader) Read(p []byte) (int, error) {
	if z.left <= 0 {
		return 0, io.EOF
	}
	n := int64(len(p))
	if n > z.left {
		n = z.left
	}
	clear(p[:n])
	z.left -= n
	z.add(n)
	return int(n), nil
}

func uploadOnce(ctx context.Context, client *http.Client, base string, add func(int64)) error {
	const size = 10_000_000
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, base+"/__up", &zeroReader{left: size, add: add})
	if err != nil {
		return err
	}
	req.ContentLength = size
	req.Header.Set("Content-Type", "application/octet-stream")
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()
	return speedStatus(resp)
}
