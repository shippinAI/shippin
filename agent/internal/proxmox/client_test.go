package proxmox

import (
	"context"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func trustedClient(t *testing.T, server *httptest.Server) *Client {
	t.Helper()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw})
	client, err := New(server.URL, "user@realm!token", "secret-value", ca)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func TestInspectVerifiedHTTPSAndInventory(t *testing.T) {
	var requests []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		if got := r.Header.Get("Authorization"); got != "PVEAPIToken=user@realm!token=secret-value" {
			t.Errorf("incorrect authorization header")
		}
		switch r.URL.RequestURI() {
		case "/api2/json/version":
			fmt.Fprint(w, `{"data":{"version":"8.4"}}`)
		case "/api2/json/cluster/resources?type=vm":
			fmt.Fprint(w, `{"data":[{"vmid":102,"node":"node-b","type":"qemu","status":"stopped"},{"vmid":101,"name":"app","node":"node-a","type":"lxc","status":"running"}]}`)
		default:
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()
	got, err := trustedClient(t, server).Inspect(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	want := Observation{Provider: "proxmox", ServerVersion: "8.4", Instances: []Instance{{ID: 101, Name: "app", Node: "node-a", Type: "lxc", Status: "running"}, {ID: 102, Node: "node-b", Type: "qemu", Status: "stopped"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	if !reflect.DeepEqual(requests, []string{"GET /api2/json/version", "GET /api2/json/cluster/resources?type=vm"}) {
		t.Fatalf("unexpected requests: %v", requests)
	}
}

func TestInspectFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"denied", "secret response", 403},
		{"redirect", "", 302},
		{"invalid", "{", 200},
		{"null", `{"data":null}`, 200},
		{"incomplete", `{"data":[{"vmid":101}]}`, 200},
		{"duplicate", `{"data":[{"vmid":101,"node":"n","type":"lxc","status":"running"},{"vmid":101,"node":"n","type":"qemu","status":"stopped"}]}`, 200},
		{"oversized", strings.Repeat("x", maxResponseBytes+1), 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api2/json/version" {
					fmt.Fprint(w, `{"data":{"version":"8.4"}}`)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			got, err := trustedClient(t, server).Inspect(context.Background())
			if err == nil || got.Provider != "" || strings.Contains(err.Error(), "secret") {
				t.Fatalf("expected sanitized failure, got %#v, %v", got, err)
			}
		})
	}
}

func TestRejectsInvalidConfigurationAndUntrustedTLS(t *testing.T) {
	for _, origin := range []string{"http://example.com", "https://user:pass@example.com", "https://example.com/path", "https://example.com?token=x", "https://example.com/#fragment"} {
		if _, err := New(origin, "id", "secret", nil); err == nil {
			t.Errorf("accepted invalid endpoint %q", origin)
		}
	}
	if _, err := New("https://example.com", "id", "secret\nvalue", nil); err == nil {
		t.Fatal("accepted invalid token")
	}
	if _, err := New("https://example.com", "id", "secret", []byte("invalid")); err == nil {
		t.Fatal("accepted invalid CA")
	}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { t.Error("untrusted server received request") }))
	defer server.Close()
	client, err := New(server.URL, "id", "secret", nil)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Inspect(context.Background()); err == nil {
		t.Fatal("accepted untrusted TLS")
	}
}
