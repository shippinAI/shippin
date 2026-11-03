package incus

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"shippin.cloud/console/internal/hv"
)

func tokenFor(t *testing.T, srv *httptest.Server, fingerprint string) string {
	t.Helper()
	addr := strings.TrimPrefix(srv.URL, "https://")
	raw, err := json.Marshal(map[string]any{"client_name": "panel", "fingerprint": fingerprint, "addresses": []string{addr}, "secret": "s3cret", "expires_at": "2099-01-01T00:00:00Z"})
	if err != nil {
		t.Fatal(err)
	}
	return base64.StdEncoding.EncodeToString(raw)
}

// fakeServer behaves like Incus for a remote client: public API until a certificate is trusted.
func fakeServer(t *testing.T) (srv *httptest.Server, trustedCount func() int, posted func() map[string]string) {
	t.Helper()
	var mu sync.Mutex
	trusted := map[string]bool{}
	var body map[string]string
	srv = httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		peer := ""
		if len(r.TLS.PeerCertificates) > 0 {
			peer = hv.Fingerprint(r.TLS.PeerCertificates[0])
		}
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/1.0/certificates":
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			if body["trust_token"] == "" || peer == "" {
				http.Error(w, "refused", http.StatusForbidden)
				return
			}
			trusted[peer] = true
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"type":"sync","status_code":200}`))
		case r.URL.Path == "/1.0":
			auth := "untrusted"
			if trusted[peer] {
				auth = "trusted"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"auth": auth, "environment": map[string]any{"server_version": "6.0.4", "server_name": "plat", "kernel_version": "6.12", "os_name": "Debian", "os_version": "13"}}})
		case r.URL.Path == "/1.0/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"cpu": map[string]any{"total": 112, "sockets": []map[string]string{{"name": "Xeon 8280"}}}, "memory": map[string]any{"total": 800, "used": 100}}})
		default:
			http.NotFound(w, r)
		}
	}))
	srv.TLS = &tls.Config{ClientAuth: tls.RequestClientCert}
	srv.StartTLS()
	t.Cleanup(srv.Close)
	return srv, func() int { mu.Lock(); defer mu.Unlock(); return len(trusted) }, func() map[string]string { mu.Lock(); defer mu.Unlock(); return body }
}

func TestEnrollSpendsTheTokenAndProvesTrust(t *testing.T) {
	srv, trusted, posted := fakeServer(t)
	token := tokenFor(t, srv, hv.Fingerprint(srv.Certificate()))
	got, err := Enroll(context.Background(), token, "default")
	if err != nil {
		t.Fatal(err)
	}
	if trusted() != 1 {
		t.Fatalf("trusted %d", trusted())
	}
	if posted()["trust_token"] != token || posted()["type"] != "client" {
		t.Fatalf("posted %v", posted())
	}
	if got.BaseURL != srv.URL || got.Version != "6.0.4" || len(got.TrustedPEM) == 0 || !strings.Contains(string(got.KeyPEM), "PRIVATE KEY") {
		t.Fatalf("%+v", got)
	}
	// What it kept really works, with verification on.
	cert, err := tls.X509KeyPair(got.CertPEM, got.KeyPEM)
	if err != nil {
		t.Fatal(err)
	}
	c, err := New(Endpoint{BaseURL: got.BaseURL, TrustedPEM: got.TrustedPEM, ClientCert: &cert}, func() (string, error) { return "", nil })
	if err != nil {
		t.Fatal(err)
	}
	if h, err := c.Health(context.Background()); err != nil || h.Version != "6.0.4" {
		t.Fatalf("%+v %v", h, err)
	}
	d, err := c.Details(context.Background())
	if err != nil || d.Hostname != "plat" || d.CPUs != 112 || d.CPUModel != "Xeon 8280" || d.MemTotal != 800 || d.OS != "Debian 13" {
		t.Fatalf("%+v %v", d, err)
	}
}

func TestEnrollRefusesACertificateThatDoesNotMatchAndSendsNothing(t *testing.T) {
	srv, trusted, posted := fakeServer(t)
	token := tokenFor(t, srv, strings.Repeat("ab", 32))
	_, err := Enroll(context.Background(), token, "")
	if err == nil || !strings.Contains(err.Error(), "does not match") {
		t.Fatalf("%v", err)
	}
	if trusted() != 0 || posted() != nil {
		t.Fatal("the token was sent to a server it did not come from")
	}
}

func TestParseTrustTokenRejectsGarbageWithoutQuotingIt(t *testing.T) {
	for _, s := range []string{"", "not-a-token", base64.StdEncoding.EncodeToString([]byte(`{"fingerprint":"x"}`))} {
		_, err := ParseTrustToken(s)
		if err == nil {
			t.Errorf("accepted %q", s)
		} else if s != "" && strings.Contains(err.Error(), s) {
			t.Errorf("error quotes the input: %v", err)
		}
	}
}

func TestLocateReportsWhenNoAddressAnswers(t *testing.T) {
	srv, _, _ := fakeServer(t)
	fp := hv.Fingerprint(srv.Certificate())
	addr := strings.TrimPrefix(srv.URL, "https://")
	srv.Close()
	_, err := Locate(context.Background(), TrustToken{Fingerprint: fp, Addresses: []string{addr}, Secret: "x"})
	if err == nil || !strings.Contains(err.Error(), "core.https_address") {
		t.Fatalf("%v", err)
	}
}

func TestSnapshotsListAndRestore(t *testing.T) {
	var restored string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/1.0/instances/web/snapshots":
			_, _ = w.Write([]byte(`{"metadata":["/1.0/instances/web/snapshots/before","/1.0/instances/web/snapshots/with%20space"]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/1.0/instances/web":
			var in map[string]string
			_ = json.NewDecoder(r.Body).Decode(&in)
			restored = in["restore"]
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"type":"async","metadata":{"id":"op1"}}`))
		case r.URL.Path == "/1.0/operations/op1/wait":
			_, _ = w.Write([]byte(`{"metadata":{"status_code":200}}`))
		case r.URL.Path == "/1.0/instances/web":
			_, _ = w.Write([]byte(`{"metadata":{"name":"web","type":"container","status":"Running"}}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	trusted := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	c, err := New(Endpoint{BaseURL: srv.URL, TrustedPEM: trusted}, func() (string, error) { return "token", nil })
	if err != nil {
		t.Fatal(err)
	}
	names, err := c.Snapshots(context.Background(), "web")
	if err != nil || len(names) != 2 || names[0] != "before" || names[1] != "with space" {
		t.Fatalf("%v %v", names, err)
	}
	if _, err := c.Rollback(context.Background(), "web", "before"); err != nil || restored != "before" {
		t.Fatalf("restored %q: %v", restored, err)
	}
	if _, err := c.Rollback(context.Background(), "web", "a/b"); err == nil {
		t.Fatal("a name with a slash must be refused")
	}
}

func TestHealthSaysWhenTheServerDoesNotTrustThePanel(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte(`{"metadata":{"auth":"untrusted"}}`))
	}))
	defer srv.Close()
	trusted := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	c, err := New(Endpoint{BaseURL: srv.URL, TrustedPEM: trusted}, func() (string, error) { return "tok", nil })
	if err != nil {
		t.Fatal(err)
	}
	h, err := c.Health(context.Background())
	if err == nil || h.Online {
		t.Fatalf("an untrusted panel must not read as online: %+v, %v", h, err)
	}
	if !strings.Contains(err.Error(), "no longer trusts") {
		t.Fatalf("the error should say what is wrong: %v", err)
	}
}
