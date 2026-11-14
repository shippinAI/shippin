package main

import (
	"bytes"
	"crypto/ed25519"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestFixtureOptInRequired(t *testing.T) {
	t.Setenv("SHIPPIN_ACQUISITION_DISPOSABLE", "")
	if run() == nil {
		t.Fatal("must reject without explicit disposable opt in")
	}
}
func TestFixtureRejectsUnknownPhase(t *testing.T) {
	t.Setenv("SHIPPIN_ACQUISITION_DISPOSABLE", "1")
	old := os.Args
	defer func() { os.Args = old }()
	os.Args = []string{"helper", "not-supported"}
	if run() == nil {
		t.Fatal("must reject unsupported phase")
	}
}
func TestDeleteRejectsInvalidTargetBeforeReadingSecrets(t *testing.T) {
	for _, id := range []string{"", "../x", "https://real.example/lead", "abcdef", "0123456789abcdef0123456789abcdef/"} {
		t.Setenv("ACQUISITION_TEST_LEAD_ID", id)
		if err := deleteLead(); err == nil || err.Error() != "invalid lead id" {
			t.Fatal("invalid target must fail before secret or network access")
		}
	}
}

func testKey(t *testing.T, dir, name string) ed25519.PrivateKey {
	t.Helper()
	_, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		t.Fatal(e)
	}
	der, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		t.Fatal(e)
	}
	if e = exclusiveFile(filepath.Join(dir, name), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})); e != nil {
		t.Fatal(e)
	}
	return key
}
func TestExporterFilesPreserveAndValidate(t *testing.T) {
	dir := t.TempDir()
	testKey(t, dir, "admin-signer")
	if e := exporterFiles(dir); e != nil {
		t.Fatal(e)
	}
	before := map[string][]byte{}
	for _, n := range []string{"admin-signer", "export-signer", "export-verifier", "export-dsn"} {
		before[n], _ = os.ReadFile(filepath.Join(dir, n))
	}
	if e := exporterFiles(dir); e != nil {
		t.Fatal(e)
	}
	for n, b := range before {
		a, _ := os.ReadFile(filepath.Join(dir, n))
		if !bytes.Equal(a, b) {
			t.Fatal("existing file changed")
		}
	}
	if e := os.WriteFile(filepath.Join(dir, "export-dsn"), []byte("postgres://unexpected"), 0600); e != nil {
		t.Fatal(e)
	}
	if exporterFiles(dir) == nil {
		t.Fatal("unexpected DSN accepted")
	}
}
func TestExporterPartialFilesFailClosed(t *testing.T) {
	dir := t.TempDir()
	testKey(t, dir, "admin-signer")
	testKey(t, dir, "export-signer")
	if exporterFiles(dir) == nil {
		t.Fatal("partial set accepted")
	}
	if _, e := os.Stat(filepath.Join(dir, "export-verifier")); !os.IsNotExist(e) {
		t.Fatal("partial set silently completed")
	}
}
func TestExclusiveFileCannotReplace(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret")
	if e := exclusiveFile(path, []byte("original")); e != nil {
		t.Fatal(e)
	}
	if exclusiveFile(path, []byte("replacement")) == nil {
		t.Fatal("overwrote file")
	}
	got, _ := os.ReadFile(path)
	if string(got) != "original" {
		t.Fatal("old bytes lost")
	}
}
func TestSeparateTokenAuthorities(t *testing.T) {
	_, key, _ := ed25519.GenerateKey(rand.Reader)
	for _, management := range []bool{false, true} {
		token := fixtureToken(key, management)
		parts := strings.Split(token, ".")
		claimsRaw, e := base64.RawURLEncoding.DecodeString(parts[1])
		if e != nil {
			t.Fatal(e)
		}
		var c struct {
			Issuer   string   `json:"iss"`
			Audience string   `json:"aud"`
			Scopes   []string `json:"scopes"`
		}
		if e = json.Unmarshal(claimsRaw, &c); e != nil {
			t.Fatal(e)
		}
		issuer, audience, scope := "fixture", "source-admin", "marketing:read"
		if management {
			issuer, audience, scope = "fixture-management", "source-management", "marketing:write"
		}
		if c.Issuer != issuer || c.Audience != audience || len(c.Scopes) != 1 || c.Scopes[0] != scope {
			t.Fatal("authority mismatch")
		}
		sig, _ := base64.RawURLEncoding.DecodeString(parts[2])
		if !ed25519.Verify(key.Public().(ed25519.PublicKey), []byte(parts[0]+"."+parts[1]), sig) {
			t.Fatal("bad signature")
		}
	}
}
func fixtureCertificate(t *testing.T, dir string, key ed25519.PrivateKey, loopback bool) []byte {
	t.Helper()
	now := time.Now()
	cert := &x509.Certificate{SerialNumber: big.NewInt(1), NotBefore: now.Add(-2 * time.Hour), NotAfter: now.Add(-time.Hour), KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, IsCA: true, BasicConstraintsValid: true}
	if loopback {
		cert.IPAddresses = []net.IP{net.ParseIP("127.0.0.1")}
	}
	der, e := x509.CreateCertificate(rand.Reader, cert, cert, key.Public(), key)
	if e != nil {
		t.Fatal(e)
	}
	raw := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	if e = exclusiveFile(filepath.Join(dir, "admin-cert"), raw); e != nil {
		t.Fatal(e)
	}
	return raw
}
func TestRenewPreservesKeyAndBackup(t *testing.T) {
	dir := t.TempDir()
	key := testKey(t, dir, "admin-key")
	old := fixtureCertificate(t, dir, key, true)
	keyBefore, _ := os.ReadFile(filepath.Join(dir, "admin-key"))
	if e := renewCertificate(dir, "admin", time.Now()); e != nil {
		t.Fatal(e)
	}
	keyAfter, _ := os.ReadFile(filepath.Join(dir, "admin-key"))
	if !bytes.Equal(keyBefore, keyAfter) {
		t.Fatal("private key changed")
	}
	backups, _ := filepath.Glob(filepath.Join(dir, "admin-cert.before-renew-*"))
	if len(backups) != 1 {
		t.Fatal("backup missing")
	}
	backup, _ := os.ReadFile(backups[0])
	if !bytes.Equal(old, backup) {
		t.Fatal("backup differs")
	}
	certAfter, _ := os.ReadFile(filepath.Join(dir, "admin-cert"))
	if bytes.Equal(old, certAfter) {
		t.Fatal("expired cert not renewed")
	}
	if _, e := tls.X509KeyPair(certAfter, keyAfter); e != nil {
		t.Fatal(e)
	}
	if e := renewCertificate(dir, "admin", time.Now()); e != nil {
		t.Fatal(e)
	}
	again, _ := os.ReadFile(filepath.Join(dir, "admin-cert"))
	if !bytes.Equal(again, certAfter) {
		t.Fatal("fresh certificate replaced")
	}
}
func TestRenewRejectsMismatchedKeyOrSAN(t *testing.T) {
	for _, mismatch := range []bool{false, true} {
		dir := t.TempDir()
		key := testKey(t, dir, "admin-key")
		if mismatch {
			_, key, _ = ed25519.GenerateKey(rand.Reader)
		}
		old := fixtureCertificate(t, dir, key, mismatch)
		if renewCertificate(dir, "admin", time.Now()) == nil {
			t.Fatal("bad fixture accepted")
		}
		got, _ := os.ReadFile(filepath.Join(dir, "admin-cert"))
		if !bytes.Equal(old, got) {
			t.Fatal("invalid cert replaced")
		}
	}
}
