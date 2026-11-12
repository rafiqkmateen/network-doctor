package web

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"io"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

func ips(s ...string) []net.IP {
	var out []net.IP
	for _, a := range s {
		out = append(out, net.ParseIP(a))
	}
	return out
}

// ND-191: the certificate is made on this computer, covers the address given, and is kept in the results folder.
func TestCertIsMadeHereAndCoversTheAddress(t *testing.T) {
	dir := t.TempDir()
	cert, err := EnsureCert(dir, ips("192.168.1.20", "127.0.0.1"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	leaf, err := x509.ParseCertificate(cert.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	if err := leaf.VerifyHostname("192.168.1.20"); err != nil {
		t.Errorf("must cover the share address: %v", err)
	}
	if leaf.NotAfter.Before(time.Now().Add(300 * 24 * time.Hour)) {
		t.Errorf("should last about a year, ends %v", leaf.NotAfter)
	}
	for _, f := range []string{"https-cert.pem", "https-key.pem"} {
		if _, err := os.Stat(filepath.Join(dir, f)); err != nil {
			t.Errorf("missing %s: %v", f, err)
		}
	}
	if runtime.GOOS != "windows" {
		st, _ := os.Stat(filepath.Join(dir, "https-key.pem"))
		if st.Mode().Perm()&0o077 != 0 {
			t.Errorf("the private key must be readable only by its owner: %v", st.Mode())
		}
	}
}

// ND-191: the same certificate is reused while it is good; a new address or an old certificate makes a new one.
func TestCertIsReusedUntilItNeedsReplacing(t *testing.T) {
	dir := t.TempDir()
	now := time.Now()
	a, _ := EnsureCert(dir, ips("192.168.1.20"), now)
	b, _ := EnsureCert(dir, ips("192.168.1.20"), now.Add(24*time.Hour))
	if string(a.Certificate[0]) != string(b.Certificate[0]) {
		t.Error("a good certificate must be reused, so the phone warns only once")
	}
	c, _ := EnsureCert(dir, ips("192.168.1.77"), now)
	if string(a.Certificate[0]) == string(c.Certificate[0]) {
		t.Error("a new address needs a certificate that covers it")
	}
	d, _ := EnsureCert(dir, ips("192.168.1.77"), now.Add(350*24*time.Hour))
	if string(c.Certificate[0]) == string(d.Certificate[0]) {
		t.Error("a certificate within 30 days of ending must be replaced")
	}
	os.WriteFile(filepath.Join(dir, "https-cert.pem"), []byte("broken"), 0o644)
	if _, err := EnsureCert(dir, ips("192.168.1.77"), now); err != nil {
		t.Errorf("a damaged file must be replaced, not fatal: %v", err)
	}
}

// ND-190: the page really answers over HTTPS, with the same allow rules as before.
func TestServeTLSAnswersOverHTTPS(t *testing.T) {
	dir := t.TempDir()
	cert, err := EnsureCert(dir, ips("127.0.0.1"), time.Now())
	if err != nil {
		t.Fatal(err)
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Skip("no local port")
	}
	addr := ln.Addr().String()
	ln.Close()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go ServeTLSWith(ctx, addr, nil, func() ([]byte, error) { return []byte(`{}`), nil }, nil, cert)

	pool := x509.NewCertPool()
	leaf, _ := x509.ParseCertificate(cert.Certificate[0])
	pool.AddCert(leaf)
	client := &http.Client{Timeout: 2 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool}}}
	var resp *http.Response
	for i := 0; i < 40; i++ {
		resp, err = client.Get("https://" + addr + "/api/health")
		if err == nil {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 || string(b) != `{"ok":true}` {
		t.Errorf("status %d body %s", resp.StatusCode, b)
	}
	if resp.TLS == nil || resp.TLS.Version < tls.VersionTLS12 {
		t.Error("must be TLS 1.2 or newer")
	}
}

// ND-190: the listen guard still refuses to open beyond this machine without an allow list.
func TestServeTLSKeepsTheListenGuard(t *testing.T) {
	cert, _ := EnsureCert(t.TempDir(), ips("192.168.1.20"), time.Now())
	err := ServeTLSWith(context.Background(), "0.0.0.0:8765", nil, nil, nil, cert)
	if err == nil {
		t.Error("listening beyond this machine needs an allow list")
	}
}
