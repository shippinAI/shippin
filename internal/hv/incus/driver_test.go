package incus

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestStartReadsBack(t *testing.T) {
	var mu sync.Mutex
	status := "Stopped"
	var saw string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/1.0":
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]any{"environment": map[string]string{"server_version": "6.0"}}})
		case r.URL.Path == "/1.0/instances/app":
			mu.Lock()
			st := status
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]string{"name": "app", "status": st, "type": "container"}})
		case r.Method == http.MethodPut && r.URL.Path == "/1.0/instances/app/state":
			b, _ := io.ReadAll(r.Body)
			saw = string(b)
			mu.Lock()
			status = "Running"
			mu.Unlock()
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	h, err := c.Health(context.Background())
	if err != nil || h.Version != "6.0" {
		t.Fatalf("%+v %v", h, err)
	}
	got, err := c.Start(context.Background(), "app")
	if err != nil || got.Status != "Running" || got.Kind != "ct" {
		t.Fatalf("%+v %v", got, err)
	}
	if !strings.Contains(saw, `"action":"start"`) {
		t.Fatalf("body %s", saw)
	}
	if _, err := c.Start(context.Background(), "app"); err != nil {
		t.Fatal(err)
	}
}

func TestTLSRejectsUnknownCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	defer srv.Close()
	c, err := New(Endpoint{BaseURL: srv.URL}, func() (string, error) { return "secret", nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Health(context.Background()); err == nil {
		t.Fatal("expected TLS failure")
	}
}

func clientFor(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	c, err := New(Endpoint{BaseURL: srv.URL, CAPEM: pemBytes}, func() (string, error) { return "secret", nil })
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func TestMutationWaitsForTheOperationAndReportsItsFailure(t *testing.T) {
	for _, tc := range []struct {
		name    string
		wait    string
		wantErr string
	}{
		{"success", `{"metadata":{"status_code":200}}`, ""},
		{"failure", `{"metadata":{"status_code":400,"err":"disk full"}}`, "disk full"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var mu sync.Mutex
			status, waited := "Stopped", ""
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				switch {
				case r.Method == http.MethodPut:
					if got := r.URL.Query().Get("project"); got != "lab" {
						t.Errorf("project %q", got)
					}
					w.WriteHeader(http.StatusAccepted)
					_ = json.NewEncoder(w).Encode(map[string]any{"type": "async", "metadata": map[string]string{"id": "op-1"}})
				case r.URL.Path == "/1.0/operations/op-1/wait":
					waited = r.URL.Query().Get("project")
					if tc.wantErr == "" {
						status = "Running"
					}
					_, _ = io.WriteString(w, tc.wait)
				case r.URL.Path == "/1.0/instances/app":
					_ = json.NewEncoder(w).Encode(map[string]any{"metadata": map[string]string{"name": "app", "status": status, "type": "container"}})
				default:
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()
			c := clientFor(t, srv)
			c.Endpoint.Project = "lab"
			got, err := c.Start(context.Background(), "app")
			if waited != "lab" {
				t.Fatalf("did not wait on the operation in its project: %q", waited)
			}
			if tc.wantErr == "" {
				if err != nil || got.Status != "Running" {
					t.Fatalf("%+v %v", got, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestGuestIDIsEscapedSoItCannotReachAnotherEndpoint(t *testing.T) {
	var saw string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		saw = r.URL.EscapedPath()
		http.NotFound(w, r)
	}))
	defer srv.Close()
	if _, err := clientFor(t, srv).Guest(context.Background(), "../certificates"); err == nil {
		t.Fatal("expected not found")
	}
	if saw != "/1.0/instances/..%2Fcertificates" {
		t.Fatalf("path %q", saw)
	}
}
