package proxmox

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestStartReadsBackAndSkipsWhenRunning(t *testing.T) {
	var mu sync.Mutex
	status := map[string]string{"100": "stopped", "101": "running"}
	posts := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "PVEAPIToken=id!tok=secret" {
			http.Error(w, "auth", http.StatusUnauthorized)
			return
		}
		switch {
		case r.URL.Path == "/api2/json/version":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"version": "8.2.2"}})
		case r.URL.Path == "/api2/json/cluster/resources":
			mu.Lock()
			defer mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"vmid": 100, "name": "ct-app", "type": "lxc", "status": status["100"], "node": "n1"},
				{"vmid": 101, "name": "vm-db", "type": "qemu", "status": status["101"], "node": "n1"},
			}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/status/start"):
			mu.Lock()
			posts++
			status["100"] = "running"
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID:n1"})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	h, err := c.Health(context.Background())
	if err != nil || !h.Online || h.Version != "8.2.2" {
		t.Fatalf("health %+v %v", h, err)
	}
	got, err := c.Start(context.Background(), "100")
	if err != nil || got.Status != "running" || got.Kind != "ct" {
		t.Fatalf("start %+v %v", got, err)
	}
	if _, err := c.Start(context.Background(), "101"); err != nil {
		t.Fatal(err)
	}
	if posts != 1 {
		t.Fatalf("posts=%d, second start must not POST", posts)
	}
}

func TestStartFailsWhenReadBackDisagrees(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api2/json/cluster/resources" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"vmid": 100, "name": "a", "type": "qemu", "status": "stopped", "node": "n1"},
			}})
			return
		}
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID"})
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	got, err := c.Start(context.Background(), "100")
	if err == nil || got.Status != "stopped" || !strings.Contains(err.Error(), "stopped") {
		t.Fatalf("got %+v err %v", got, err)
	}
}

func TestTLSRejectsUnknownCA(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	c, err := New(Endpoint{BaseURL: srv.URL, Node: "n1", TokenID: "id!tok"}, func() (string, error) { return "secret", nil })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.Health(context.Background()); err == nil {
		t.Fatal("system roots must not trust the test certificate")
	}
}

func TestSnapshotAllowsLXC(t *testing.T) {
	var path string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			path = r.URL.Path
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID:n1:00000001:00000002:00000003:snapshot:100:token@pve:"})
			return
		}
		if strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/status") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "stopped", "exitstatus": "OK", "type": "snapshot", "upid": "UPID:n1:00000001:00000002:00000003:snapshot:100:token@pve:"}})
			return
		}
		if strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/log") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"t": "TASK OK"}}})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
			{"vmid": 100, "name": "ct", "type": "lxc", "status": "running", "node": "n1"},
		}})
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	got, err := c.Snapshot(context.Background(), "100", "nightly")
	if err != nil || got.Kind != "ct" || !strings.Contains(path, "/lxc/100/snapshot") {
		t.Fatalf("%+v %v path %s", got, err, path)
	}
}

func clientFor(t *testing.T, srv *httptest.Server) *Client {
	t.Helper()
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	c, err := New(Endpoint{BaseURL: srv.URL, Node: "n1", TokenID: "id!tok", CAPEM: pemBytes}, func() (string, error) { return "secret", nil })
	if err != nil {
		t.Fatal(err)
	}
	return c
}
