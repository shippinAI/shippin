// Disposable acquisition fixture only. Never deploy as a service component.
package main

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/ed25519"
	"crypto/elliptic"
	"crypto/rand"
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
func deleteLead() error {
	id := os.Getenv("ACQUISITION_TEST_LEAD_ID")
	if !regexp.MustCompile(`^[a-f0-9]{32}$`).MatchString(id) {
		return errors.New("invalid lead id")
	}
	raw, err := os.ReadFile("/secrets/admin-signer")
	if err != nil {
		return err
	}
	block, rest := pem.Decode(raw)
	if block == nil || len(bytes.TrimSpace(rest)) != 0 {
		return errors.New("invalid signer")
	}
	parsed, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return err
	}
	key, ok := parsed.(ed25519.PrivateKey)
	if !ok {
		return errors.New("not Ed25519")
	}
	defer clear(key)
	defer clear(raw)
	ca, err := os.ReadFile("/secrets/admin-cert")
	if err != nil {
		return err
	}
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca) {
		return errors.New("invalid trust")
	}
	client := &http.Client{Timeout: 5 * time.Second, Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}}, CheckRedirect: func(r *http.Request, via []*http.Request) error { return errors.New("redirect refused") }}
	now := time.Now().Unix()
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"EdDSA","typ":"JWT"}`))
	claims, _ := json.Marshal(map[string]any{"iss": "fixture", "aud": "source-admin", "sub": "fixture-acquisition-delete", "tenant": "container", "projects": []string{"signup"}, "scopes": []string{"marketing:read", "marketing:write"}, "exp": now + 120, "nbf": now - 1})
	unsigned := header + "." + base64.RawURLEncoding.EncodeToString(claims)
	token := unsigned + "." + base64.RawURLEncoding.EncodeToString(ed25519.Sign(key, []byte(unsigned)))
	request := func(method, path string) (*http.Response, error) {
		r, e := http.NewRequest(method, "https://127.0.0.1:18082"+path, nil)
		if e != nil {
			return nil, e
		}
		r.Header.Set("Authorization", "Bearer "+token)
		return client.Do(r)
	}
	// Walk bounded pages, proving the exact target exists before a destructive fixture request.
	found := false
	after := "0"
	for page := 0; page < 100 && !found; page++ {
		response, e := request("GET", "/v1/projects/signup/marketing/leads?limit=100&after="+after)
		if e != nil {
			return e
		}
		if response.StatusCode != 200 {
			response.Body.Close()
			return errors.New("source read denied")
		}
		var result struct {
			Items []struct {
				ID      string `json:"id"`
				Email   string `json:"email"`
				Deleted bool   `json:"deleted"`
			}
			NextCursor uint64 `json:"next_cursor"`
		}
		e = json.NewDecoder(io.LimitReader(response.Body, 2<<20)).Decode(&result)
		response.Body.Close()
		if e != nil {
			return e
		}
		for _, lead := range result.Items {
			if lead.ID == id {
				if lead.Deleted || !strings.HasSuffix(lead.Email, "@example.test") {
					return errors.New("not an active synthetic lead")
				}
				found = true
			}
		}
		if len(result.Items) < 100 {
			break
		}
		next := fmt.Sprint(result.NextCursor)
		if next == after {
			return errors.New("cursor did not advance")
		}
		after = next
	}
	if !found {
		return errors.New("target not found")
	}
	response, err := request("DELETE", "/v1/projects/signup/marketing/leads/"+id)
	if err != nil {
		return err
	}
	defer response.Body.Close()
	if response.StatusCode != 200 && response.StatusCode != 204 {
		return errors.New("delete failed")
	}
	fmt.Println("PASS source fixture delete count=1; panel tombstone qualification still required")
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
