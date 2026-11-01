package incus

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func localServer(t *testing.T, handler http.Handler) string {
	t.Helper()
	socket := filepath.Join(t.TempDir(), "incus.sock")
	listener, err := net.Listen("unix", socket)
	if err != nil {
		t.Fatalf("create test socket: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener.Close()
	server.Listener = listener
	server.Start()
	t.Cleanup(server.Close)
	return socket
}

func TestInspectUsesOnlyLocalReadRequestsForSelectedProject(t *testing.T) {
	var requests []string
	var requestMu sync.Mutex
	socket := localServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestMu.Lock()
		requests = append(requests, r.Method+" "+r.URL.RequestURI())
		requestMu.Unlock()
		switch r.URL.Path {
		case "/1.0":
			fmt.Fprint(w, `{"type":"sync","status_code":200,"metadata":{"environment":{"server_version":"6.0"}}}`)
		case "/1.0/instances":
			fmt.Fprint(w, `{"type":"sync","status_code":200,"metadata":[{"name":"z","type":"virtual-machine","status":"Stopped"},{"name":"a","type":"container","status":"Running"}]}`)
		default:
			t.Errorf("unexpected endpoint %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	client, err := NewLocal(socket)
	if err != nil {
		t.Fatal(err)
	}
	got, err := client.Inspect(context.Background(), "team & lab")
	if err != nil {
		t.Fatal(err)
	}
	want := Observation{Provider: "incus", Project: "team & lab", ServerVersion: "6.0", Instances: []Instance{{Name: "a", Type: "container", Status: "Running"}, {Name: "z", Type: "virtual-machine", Status: "Stopped"}}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v, want %#v", got, want)
	}
	requestMu.Lock()
	defer requestMu.Unlock()
	if !reflect.DeepEqual(requests, []string{"GET /1.0", "GET /1.0/instances?project=team+%26+lab&recursion=1"}) {
		t.Fatalf("unexpected requests: %v", requests)
	}
}

func TestInspectFailsClosed(t *testing.T) {
	for _, tc := range []struct {
		name, body string
		status     int
	}{
		{"denied", `sensitive response text`, http.StatusForbidden},
		{"redirect", ``, http.StatusFound},
		{"invalid-json", `{`, http.StatusOK},
		{"provider-error", `{"type":"error","status_code":400,"metadata":{}}`, http.StatusOK},
		{"null-list", `{"type":"sync","status_code":200,"metadata":null}`, http.StatusOK},
		{"incomplete-instance", `{"type":"sync","status_code":200,"metadata":[{"name":"vm"}]}`, http.StatusOK},
		{"oversized", strings.Repeat("x", maxResponseBytes+1), http.StatusOK},
	} {
		t.Run(tc.name, func(t *testing.T) {
			socket := localServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/1.0" {
					fmt.Fprint(w, `{"type":"sync","status_code":200,"metadata":{"environment":{"server_version":"6.0"}}}`)
					return
				}
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}))
			client, err := NewLocal(socket)
			if err != nil {
				t.Fatal(err)
			}
			got, err := client.Inspect(context.Background(), "default")
			if err == nil || got.Provider != "" || strings.Contains(err.Error(), "sensitive response text") {
				t.Fatalf("expected no partial result and sanitized failure, got %#v, %v", got, err)
			}
		})
	}
}

func TestConfigurationAndUnavailableSocket(t *testing.T) {
	if _, err := NewLocal("relative.sock"); err == nil {
		t.Fatal("relative socket accepted")
	}
	client, err := NewLocal(filepath.Join(t.TempDir(), "missing.sock"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Inspect(context.Background(), ""); err == nil {
		t.Fatal("missing project accepted")
	}
	if _, err := client.Inspect(context.Background(), "default"); err == nil {
		t.Fatal("missing socket accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := client.Inspect(ctx, "default"); err == nil {
		t.Fatal("cancellation ignored")
	}
}
