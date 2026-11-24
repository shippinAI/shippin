package tlsfile

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func writePair(t *testing.T, dir, cn string, mod time.Time) {
	t.Helper()
	k, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: cn},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour), DNSNames: []string{cn}}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &k.PublicKey, k)
	if err != nil {
		t.Fatal(err)
	}
	kd, err := x509.MarshalECPrivateKey(k)
	if err != nil {
		t.Fatal(err)
	}
	cp, kp := filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem")
	if err := os.WriteFile(cp, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(kp, pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kd}), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, p := range []string{cp, kp} {
		if err := os.Chtimes(p, mod, mod); err != nil {
			t.Fatal(err)
		}
	}
}

func cn(t *testing.T, l *Loader) string {
	t.Helper()
	c, err := l.GetCertificate(nil)
	if err != nil {
		t.Fatal(err)
	}
	x, err := x509.ParseCertificate(c.Certificate[0])
	if err != nil {
		t.Fatal(err)
	}
	return x.Subject.CommonName
}

func TestReloadOnChange(t *testing.T) {
	dir := t.TempDir()
	writePair(t, dir, "one.example.invalid", time.Unix(1000, 0))
	l, err := New(filepath.Join(dir, "cert.pem"), filepath.Join(dir, "key.pem"))
	if err != nil {
		t.Fatal(err)
	}
	if got := cn(t, l); got != "one.example.invalid" {
		t.Fatalf("got %s", got)
	}
	writePair(t, dir, "two.example.invalid", time.Unix(2000, 0))
	if got := cn(t, l); got != "two.example.invalid" {
		t.Fatalf("after renewal got %s", got)
	}
	if err := os.WriteFile(filepath.Join(dir, "cert.pem"), []byte("garbage"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got := cn(t, l); got != "two.example.invalid" {
		t.Fatalf("bad renewal should keep the last pair, got %s", got)
	}
}

func TestMissingFilesFailAtStart(t *testing.T) {
	if _, err := New("/nonexistent/cert.pem", "/nonexistent/key.pem"); err == nil {
		t.Fatal("want error")
	}
}
