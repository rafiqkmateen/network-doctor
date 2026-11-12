package app

import (
	"bytes"
	"context"
	"crypto/tls"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type tlsCall struct {
	mu    sync.Mutex
	addr  string
	allow []string
	cert  bool
}

func fakeServeTLS(call *tlsCall) func(context.Context, string, []string, func() ([]byte, error), func(string), tls.Certificate) error {
	return func(ctx context.Context, addr string, allow []string, run func() ([]byte, error), onRemote func(string), c tls.Certificate) error {
		call.mu.Lock()
		call.addr, call.allow, call.cert = addr, allow, len(c.Certificate) > 0
		call.mu.Unlock()
		<-ctx.Done()
		return nil
	}
}

func shareHTTPS(t *testing.T, dir string, plain bool) (string, *tlsCall) {
	t.Helper()
	call := &tlsCall{}
	var plainCall serveCall
	ctx, cancel := context.WithTimeout(context.Background(), 700*time.Millisecond)
	defer cancel()
	var out bytes.Buffer
	Share(ctx, healthy(), ShareOptions{Port: 8765, Wait: time.Second, CertDir: dir, Plain: plain,
		Serve: fakeServe(&plainCall, 0, ""), ServeTLS: fakeServeTLS(call)}, Options{PlanMbps: 300}, &out)
	return out.String(), call
}

// ND-190: share gives an https address, draws a code for it, and serves with a certificate.
func TestShareUsesHTTPSByDefault(t *testing.T) {
	dir := t.TempDir()
	out, call := shareHTTPS(t, dir, false)
	if !strings.Contains(out, "https://192.168.1.20:8765/") || strings.Contains(out, "http://192.168.1.20") {
		t.Errorf("the address must be https:\n%s", out)
	}
	call.mu.Lock()
	defer call.mu.Unlock()
	if call.addr != "192.168.1.20:8765" || !call.cert {
		t.Errorf("must serve on the adapter address with a certificate: %+v", call)
	}
	if len(call.allow) != 1 || call.allow[0] != "192.168.1.0/24" {
		t.Errorf("still only its own network: %v", call.allow)
	}
	if _, err := os.Stat(filepath.Join(dir, "https-cert.pem")); err != nil {
		t.Errorf("the certificate is kept in the results folder: %v", err)
	}
}

// ND-190: --http turns it off, for a person who wants the old plain address.
func TestShareCanStayPlain(t *testing.T) {
	out, call := shareHTTPS(t, t.TempDir(), true)
	if !strings.Contains(out, "http://192.168.1.20:8765/") || strings.Contains(out, "https://") {
		t.Errorf("plain means http:\n%s", out)
	}
	if call.addr != "" {
		t.Error("plain must not start the secure server")
	}
}

// ND-192: the tool says in plain words that the phone warns once, and what to press.
func TestShareExplainsThePhoneWarning(t *testing.T) {
	out, _ := shareHTTPS(t, t.TempDir(), false)
	if !strings.Contains(out, shareWarn) || !strings.Contains(out, shareWarnPress) {
		t.Errorf("the warning must be explained before the phone sees it:\n%s", out)
	}
	plain, _ := shareHTTPS(t, t.TempDir(), true)
	if strings.Contains(plain, shareWarn) {
		t.Error("no warning text when the address is plain")
	}
}

// If the certificate cannot be made, share says so and falls back to plain, never to silence.
func TestShareFallsBackWhenNoCertificate(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "file-not-folder")
	os.WriteFile(bad, []byte("x"), 0o644)
	out, _ := shareHTTPS(t, bad, false)
	if !strings.Contains(out, shareNoCert) || !strings.Contains(out, "http://192.168.1.20:8765/") {
		t.Errorf("must say it could not make the certificate and use the plain address:\n%s", out)
	}
}
