package hv

import (
	"context"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUnknownCertificateIsRefusedThenAcceptedOnlyWhenTrusted(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	get := func(cfg *TLSConfigResult) error {
		req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, srv.URL, nil)
		res, err := (&http.Client{Transport: &http.Transport{TLSClientConfig: cfg.Config}}).Do(req)
		if err == nil {
			res.Body.Close()
		}
		return err
	}
	strict, err := TLSConfig(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	refused := get(&TLSConfigResult{strict})
	if refused == nil {
		t.Fatal("an unknown certificate must be refused")
	}
	seen := PresentedCertificate(refused)
	if seen == nil {
		t.Fatalf("the failure should show the certificate: %v", refused)
	}
	info := DescribeCertificate(seen)
	if !strings.Contains(info.Fingerprint, ":") || info.PEM == "" || len(info.Names) == 0 {
		t.Fatalf("%+v", info)
	}
	trusted, err := TLSConfig(nil, []byte(info.PEM), nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := get(&TLSConfigResult{trusted}); err != nil {
		t.Fatalf("a trusted certificate must connect even when the address is not one it names: %v", err)
	}
}

func TestATrustedCertificateTrustsNothingElse(t *testing.T) {
	first := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer first.Close()
	other := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	other.StartTLS()
	defer other.Close()
	// A different key under the same names must not pass. Forge one by trusting an unrelated CA.
	cfg, err := TLSConfig(nil, unrelatedCertPEM(t), nil)
	if err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(http.MethodGet, first.URL, nil)
	if res, err := (&http.Client{Transport: &http.Transport{TLSClientConfig: cfg}}).Do(req); err == nil {
		res.Body.Close()
		t.Fatal("a certificate other than the trusted one was accepted")
	}
}

func TestBadTrustInputIsRefused(t *testing.T) {
	if _, err := TLSConfig(nil, []byte("not pem"), nil); err == nil {
		t.Error("garbage accepted as a trusted certificate")
	}
	if _, err := TLSConfig([]byte("not pem"), nil, nil); err == nil {
		t.Error("garbage accepted as a CA")
	}
	key := pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte{1}})
	if _, err := TLSConfig(nil, key, nil); err == nil {
		t.Error("a private key accepted as a certificate")
	}
}
