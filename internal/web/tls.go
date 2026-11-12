package web

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

const (
	certFile    = "https-cert.pem"
	keyFile     = "https-key.pem"
	certLife    = 365 * 24 * time.Hour
	certRenewIn = 30 * 24 * time.Hour
)

// EnsureCert returns this computer's own certificate for the given addresses.
// It is made here, kept in dir, and never leaves the machine. A good one is
// reused so a phone warns only once; a missing, damaged, short-lived or
// wrong-address one is replaced.
func EnsureCert(dir string, addrs []net.IP, now time.Time) (tls.Certificate, error) {
	cp, kp := filepath.Join(dir, certFile), filepath.Join(dir, keyFile)
	if c, err := tls.LoadX509KeyPair(cp, kp); err == nil && certIsGood(c, addrs, now) {
		return c, nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return tls.Certificate{}, err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 100))
	if err != nil {
		return tls.Certificate{}, err
	}
	tmpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: "Network Doctor (made on this computer)"},
		NotBefore:             now.Add(-time.Hour),
		NotAfter:              now.Add(certLife),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
		IPAddresses:           addrs,
	}
	der, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		return tls.Certificate{}, err
	}
	kb, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return tls.Certificate{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(cp, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o644); err != nil {
		return tls.Certificate{}, err
	}
	if err := os.WriteFile(kp, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb}), 0o600); err != nil {
		return tls.Certificate{}, err
	}
	return tls.LoadX509KeyPair(cp, kp)
}

func certIsGood(c tls.Certificate, addrs []net.IP, now time.Time) bool {
	if len(c.Certificate) == 0 {
		return false
	}
	leaf, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil || now.Add(certRenewIn).After(leaf.NotAfter) || now.Before(leaf.NotBefore) {
		return false
	}
	for _, a := range addrs {
		if leaf.VerifyHostname(a.String()) != nil {
			return false
		}
	}
	return true
}

// ServeTLSWith is ServeWith over HTTPS (TLS 1.2 or newer). The listen guard and
// the allow list are the same.
func ServeTLSWith(ctx context.Context, addr string, allow []string, run func() ([]byte, error), onRemote func(ip string), cert tls.Certificate) error {
	if err := CheckListen(addr, allow); err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              addr,
		Handler:           HandlerWith(run, allow, onRemote),
		ReadHeaderTimeout: 5 * time.Second,
		TLSConfig:         &tls.Config{Certificates: []tls.Certificate{cert}, MinVersion: tls.VersionTLS12},
	}
	go func() { <-ctx.Done(); srv.Close() }()
	err := srv.ListenAndServeTLS("", "")
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
