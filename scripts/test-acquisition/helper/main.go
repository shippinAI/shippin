// Disposable acquisition fixture only. Never deploy as a service component.
package main

import (
	"bytes"
	"crypto"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "fixture helper failed; sensitive detail suppressed")
		os.Exit(1)
	}
}
func run() error {
	if os.Getenv("SHIPPIN_ACQUISITION_DISPOSABLE") != "1" || len(os.Args) != 2 {
		return errors.New("fixture opt in required")
	}
	switch os.Args[1] {
	case "probe":
		return probeAuthorities()
	case "exporter-files":
		return exporterFiles("/secrets")
	case "renew-certs":
		for _, name := range []string{"admin", "proxy"} {
			if err := renewCertificate("/secrets", name, time.Now()); err != nil {
				return err
			}
		}
		return nil
	case "proxy-cert":
		return proxyCertificate()
	case "proxy":
		target, _ := url.Parse("http://127.0.0.1:18080")
		proxy := httputil.NewSingleHostReverseProxy(target)
		director := proxy.Director
		proxy.Director = func(r *http.Request) { host := r.Host; director(r); r.Host = host }
		proxy.ErrorHandler = func(w http.ResponseWriter, r *http.Request, e error) {
			http.Error(w, "fixture upstream unavailable", 502)
		}
		server := &http.Server{Addr: "0.0.0.0:18443", Handler: proxy, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 20 * time.Second, IdleTimeout: 30 * time.Second, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
		return server.ListenAndServeTLS("/secrets/proxy-cert", "/secrets/proxy-key")
	case "delete":
		return deleteLead()
	default:
		return errors.New("unknown phase")
	}
}

const exportDSN = "postgres://shippin_export_fixture@127.0.0.1:5432/shippin_signup_disposable?sslmode=disable"

func loadSigner(path string) (ed25519.PrivateKey, error) {
	raw, e := os.ReadFile(path)
	if e != nil {
		return nil, e
	}
	defer clear(raw)
	block, rest := pem.Decode(raw)
	if block == nil || block.Type != "PRIVATE KEY" || len(bytes.TrimSpace(rest)) != 0 {
		return nil, errors.New("invalid signer")
	}
	defer clear(block.Bytes)
	parsed, e := x509.ParsePKCS8PrivateKey(block.Bytes)
	if e != nil {
		return nil, e
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return nil, errors.New("not Ed25519")
	}
	return key, nil
}
func fixtureToken(key ed25519.PrivateKey, management bool) string {
	issuer, audience, scope := "fixture", "source-admin", "marketing:read"
	if management {
		issuer, audience, scope = "fixture-management", "source-management", "marketing:write"
	}
	now := time.Now().Unix()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"iss": issuer, "aud": audience, "sub": "fixture-acquisition-delete", "tenant": "container", "projects": []string{"signup"}, "scopes": []string{scope}, "exp": now + 120, "nbf": now - 1})
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	return unsigned + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(unsigned)))
}

// Probe checks the fixed private endpoint without mutating source state.
func probeAuthorities() error {
	exporter, e := loadSigner("/secrets/export-signer")
	if e != nil {
		return e
	}
	defer clear(exporter)
	admin, e := loadSigner("/secrets/admin-signer")
	if e != nil {
		return e
	}
	defer clear(admin)
	if bytes.Equal(exporter.Public().(ed25519.PublicKey), admin.Public().(ed25519.PublicKey)) {
		return errors.New("authorities must differ")
	}
	ca, e := os.ReadFile("/secrets/admin-cert")
	if e != nil {
		return e
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return errors.New("invalid trust")
	}
	transport := &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}
	defer transport.CloseIdleConnections()
	client := &http.Client{Timeout: 5 * time.Second, Transport: transport, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	for _, probe := range []struct {
		token  string
		status int
	}{{fixtureToken(exporter, false), http.StatusOK}, {fixtureToken(admin, false), http.StatusUnauthorized}, {fixtureToken(admin, true), http.StatusUnauthorized}} {
		r, e := http.NewRequest(http.MethodGet, "https://127.0.0.1:18082/v1/projects/signup/marketing/leads?limit=1", nil)
		if e != nil {
			return e
		}
		r.Header.Set("Authorization", "Bearer "+probe.token)
		response, e := client.Do(r)
		if e != nil {
			return e
		}
		response.Body.Close()
		if response.StatusCode != probe.status {
			return errors.New("private authority probe failed")
		}
	}
	fmt.Println("PASS verified TLS exporter GET200; original signer old binding and management binding GET401")
	return nil
}

func deleteLead() error {
	id := os.Getenv("ACQUISITION_TEST_LEAD_ID")
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(id) {
		return errors.New("invalid lead id")
	}
	exporter, e := loadSigner("/secrets/export-signer")
	if e != nil {
		return e
	}
	defer clear(exporter)
	admin, e := loadSigner("/secrets/admin-signer")
	if e != nil {
		return e
	}
	defer clear(admin)
	if bytes.Equal(exporter.Public().(ed25519.PublicKey), admin.Public().(ed25519.PublicKey)) {
		return errors.New("authorities must differ")
	}
	ca, e := os.ReadFile("/secrets/admin-cert")
	if e != nil {
		return e
	}
	roots := x509.NewCertPool()
	if !roots.AppendCertsFromPEM(ca) {
		return errors.New("invalid trust")
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}, CheckRedirect: func(*http.Request, []*http.Request) error { return errors.New("redirect refused") }}
	readToken, writeToken, forgedToken := fixtureToken(exporter, false), fixtureToken(admin, true), fixtureToken(exporter, true)
	request := func(method, path, token string) (*http.Response, error) {
		var body io.Reader
		if method == "POST" {
			body = strings.NewReader(`{"status":"contacted"}`)
		}
		r, e := http.NewRequest(method, "https://127.0.0.1:18082"+path, body)
		if e != nil {
			return nil, e
		}
		r.Header.Set("Authorization", "Bearer "+token)
		if method == "POST" {
			r.Header.Set("Content-Type", "application/json")
			r.Header.Set("Idempotency-Key", "dddddddddddddddddddddddddddddddd")
		}
		return client.Do(r)
	}
	route := "/v1/projects/signup/marketing/leads"
	readTarget := func() (json.RawMessage, error) {
		after := "0"
		for page := 0; page < 100; page++ {
			r, e := request("GET", route+"?limit=100&after="+after, readToken)
			if e != nil {
				return nil, e
			}
			if r.StatusCode != 200 {
				r.Body.Close()
				return nil, errors.New("source read denied")
			}
			var result struct {
				Items      []json.RawMessage `json:"items"`
				NextCursor uint64            `json:"next_cursor"`
			}
			e = json.NewDecoder(io.LimitReader(r.Body, 2<<20)).Decode(&result)
			r.Body.Close()
			if e != nil {
				return nil, e
			}
			for _, raw := range result.Items {
				var lead struct {
					ID      string `json:"id"`
					Email   string `json:"email"`
					Deleted bool   `json:"deleted"`
				}
				if e = json.Unmarshal(raw, &lead); e != nil {
					return nil, e
				}
				if lead.ID == id {
					if lead.Deleted || !strings.HasSuffix(lead.Email, "@example.test") {
						return nil, errors.New("not an active synthetic lead")
					}
					return raw, nil
				}
			}
			if len(result.Items) < 100 {
				break
			}
			next := fmt.Sprint(result.NextCursor)
			if next == after {
				return nil, errors.New("cursor did not advance")
			}
			after = next
		}
		return nil, errors.New("target not found")
	}
	before, e := readTarget()
	if e != nil {
		return e
	}
	for _, probe := range []struct{ method, path, token string }{{"GET", route, writeToken}, {"POST", route + "/" + id + "/status", forgedToken}, {"DELETE", route + "/" + id, forgedToken}} {
		r, e := request(probe.method, probe.path, probe.token)
		if e != nil {
			return e
		}
		r.Body.Close()
		if r.StatusCode != http.StatusUnauthorized {
			return errors.New("authority boundary allowed request")
		}
	}
	after, e := readTarget()
	if e != nil {
		return e
	}
	if !bytes.Equal(before, after) {
		return errors.New("denied requests changed target")
	}
	r, e := request("DELETE", route+"/"+id, writeToken)
	if e != nil {
		return e
	}
	defer r.Body.Close()
	if r.StatusCode != 204 {
		return errors.New("delete failed")
	}
	fmt.Println("PASS independent exporter read, forged management denied unchanged, and management delete count=1; panel tombstone qualification still required")
	return nil
}
func exclusiveFile(path string, data []byte) error {
	f, e := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(data)
	closeErr := f.Close()
	if e != nil {
		return e
	}
	return closeErr
}
func exporterFiles(dir string) error {
	names := []string{"export-signer", "export-verifier", "export-dsn"}
	count := 0
	for _, n := range names {
		st, e := os.Lstat(filepath.Join(dir, n))
		if e == nil {
			if !st.Mode().IsRegular() {
				return errors.New("not regular fixture file")
			}
			count++
		} else if !os.IsNotExist(e) {
			return e
		}
	}
	if count != 0 && count != len(names) {
		return errors.New("partial exporter files; manual inspection required")
	}
	if count == len(names) {
		key, e := loadSigner(filepath.Join(dir, names[0]))
		if e != nil {
			return e
		}
		defer clear(key)
		raw, e := os.ReadFile(filepath.Join(dir, names[1]))
		if e != nil {
			return e
		}
		block, rest := pem.Decode(raw)
		if block == nil || block.Type != "PUBLIC KEY" || len(bytes.TrimSpace(rest)) != 0 {
			return errors.New("invalid verifier")
		}
		pub, e := x509.ParsePKIXPublicKey(block.Bytes)
		if e != nil {
			return e
		}
		ed, ok := pub.(ed25519.PublicKey)
		if !ok || !bytes.Equal(ed, key.Public().(ed25519.PublicKey)) {
			return errors.New("export key mismatch")
		}
		raw, e = os.ReadFile(filepath.Join(dir, names[2]))
		if e != nil {
			return e
		}
		if string(raw) != exportDSN {
			return errors.New("unexpected export DSN")
		}
		admin, e := loadSigner(filepath.Join(dir, "admin-signer"))
		if e != nil {
			return e
		}
		defer clear(admin)
		if bytes.Equal(admin.Public().(ed25519.PublicKey), ed) {
			return errors.New("export signer equals management")
		}
		fmt.Println("PASS existing exporter files validated without replacement")
		return nil
	}
	// Require an existing original management key before extending the fixture.
	admin, e := loadSigner(filepath.Join(dir, "admin-signer"))
	if e != nil {
		return e
	}
	defer clear(admin)
	pub, key, e := ed25519.GenerateKey(rand.Reader)
	if e != nil {
		return e
	}
	defer clear(key)
	privateDER, e := x509.MarshalPKCS8PrivateKey(key)
	if e != nil {
		return e
	}
	defer clear(privateDER)
	publicDER, e := x509.MarshalPKIXPublicKey(pub)
	if e != nil {
		return e
	}
	files := [][]byte{pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: privateDER}), pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: publicDER}), []byte(exportDSN)}
	for i, data := range files {
		e = exclusiveFile(filepath.Join(dir, names[i]), data)
		clear(data)
		if e != nil {
			return e
		}
	}
	fmt.Println("PASS distinct synthetic exporter files created; original secrets unchanged")
	return nil
}
func renewCertificate(dir, name string, now time.Time) error {
	if name != "admin" && name != "proxy" {
		return errors.New("unsupported fixture certificate")
	}
	path, keyPath := filepath.Join(dir, name+"-cert"), filepath.Join(dir, name+"-key")
	for _, p := range []string{path, keyPath} {
		st, e := os.Lstat(p)
		if e != nil {
			return e
		}
		if !st.Mode().IsRegular() {
			return errors.New("certificate path not regular")
		}
	}
	raw, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	keyRaw, e := os.ReadFile(keyPath)
	if e != nil {
		return e
	}
	defer clear(keyRaw)
	pair, e := tls.X509KeyPair(raw, keyRaw)
	if e != nil {
		return e
	}
	cert, e := x509.ParseCertificate(pair.Certificate[0])
	if e != nil {
		return e
	}
	if cert.VerifyHostname("127.0.0.1") != nil {
		return errors.New("fixture loopback SAN missing")
	}
	if cert.NotAfter.After(now.Add(time.Hour)) {
		fmt.Println("PASS existing fixture certificate remains valid")
		return nil
	}
	signer, ok := pair.PrivateKey.(crypto.Signer)
	if !ok {
		return errors.New("unsupported TLS key")
	}
	serial, e := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if e != nil {
		return e
	}
	template := &x509.Certificate{SerialNumber: serial, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(8 * time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true}
	der, e := x509.CreateCertificate(rand.Reader, template, template, signer.Public(), signer)
	if e != nil {
		return e
	}
	replacement := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})
	sum := sha256.Sum256(raw)
	backup := fmt.Sprintf("%s.before-renew-%x", path, sum[:8])
	if e = exclusiveFile(backup, raw); e != nil {
		return e
	}
	temp := path + ".renew-pending"
	if e = exclusiveFile(temp, replacement); e != nil {
		return e
	}
	if e = os.Rename(temp, path); e != nil {
		return e
	}
	fmt.Println("PASS fixture certificate renewed with same TLS private key and preserved old certificate")
	return nil
}

func proxyCertificate() error {
	for _, path := range []string{"/secrets/proxy-key", "/secrets/proxy-cert"} {
		if _, err := os.Stat(path); err == nil {
			return errors.New("proxy certificate already exists")
		} else if !os.IsNotExist(err) {
			return err
		}
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	if err != nil {
		return err
	}
	now := time.Now()
	template := &x509.Certificate{SerialNumber: serial, NotBefore: now.Add(-time.Minute), NotAfter: now.Add(4 * time.Hour), IPAddresses: []net.IP{net.ParseIP("127.0.0.1")}, DNSNames: []string{"localhost"}, KeyUsage: x509.KeyUsageDigitalSignature | x509.KeyUsageCertSign, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}, BasicConstraintsValid: true, IsCA: true}
	cert, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		return err
	}
	der, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	defer clear(der)
	for path, data := range map[string][]byte{"/secrets/proxy-cert": pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: cert}), "/secrets/proxy-key": pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: der})} {
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
		if err != nil {
			return err
		}
		_, err = file.Write(data)
		file.Close()
		clear(data)
		if err != nil {
			return err
		}
	}
	fmt.Println("PASS separate synthetic proxy certificate created; existing admin/source keys unchanged")
	return nil
}
