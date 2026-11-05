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
	"time"

	"shippin.cloud/console/internal/hv"
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
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			mu.Lock()
			defer mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": status[strings.Split(r.URL.Path, "/")[6]]}})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/status/start"):
			mu.Lock()
			posts++
			status["100"] = "running"
			mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID:n1:00000001:00000002:00000003:vzstart:100:token@pve:"})
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/status"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "stopped", "exitstatus": "OK", "type": "vzstart"}})
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/log"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"t": "TASK OK"}}})
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

func TestHealthAcceptsOnlySupportedProxmoxMajors(t *testing.T) {
	for _, test := range []struct {
		version string
		valid   bool
	}{
		{version: "8.2.2", valid: true},
		{version: "9.2.11", valid: true},
		{version: "7.4.17"},
		{version: "10.0.1"},
		{version: "8.x"},
		{version: ""},
	} {
		t.Run(test.version, func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path != "/api2/json/version" {
					http.NotFound(w, r)
					return
				}
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"version": test.version}})
			}))
			defer srv.Close()
			health, err := clientFor(t, srv).Health(context.Background())
			if test.valid && (err != nil || !health.Online || health.Version != test.version) {
				t.Fatalf("supported version health=%+v err=%v", health, err)
			}
			if !test.valid && (err == nil || health.Online || !strings.Contains(health.Error, "supported major versions are 8 and 9")) {
				t.Fatalf("unsupported version health=%+v err=%v", health, err)
			}
		})
	}
}

func TestStartFailsWhenReadBackDisagrees(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/status/current") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "stopped"}})
			return
		}
		if r.URL.Path == "/api2/json/cluster/resources" {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"vmid": 100, "name": "a", "type": "qemu", "status": "stopped", "node": "n1"},
			}})
			return
		}
		if r.Method == http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID:n1:00000001:00000002:00000003:vzstart:100:token@pve:"})
			return
		}
		if strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/status") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "stopped", "exitstatus": "OK", "type": "vzstart"}})
			return
		}
		if strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/log") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"t": "TASK OK"}}})
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

func TestGuestNetworkUsesQEMUAgentAndLXCInterfaceAPIs(t *testing.T) {
	for _, tc := range []struct {
		name, kind, endpoint, body string
	}{
		{"qemu guest agent", "qemu", "/api2/json/nodes/n1/qemu/101/agent/network-get-interfaces", `{"data":{"result":[{"name":"ens18","ip-addresses":[{"ip-address-type":"ipv4","ip-address":"192.0.2.20","scope":"global"}]}]}}`},
		{"lxc interface inventory", "lxc", "/api2/json/nodes/n1/lxc/102/interfaces", `{"data":[{"name":"eth0","inet":"192.0.2.21/24","inet6":"2001:db8::21/64"}]}`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var seen string
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch r.URL.Path {
				case "/api2/json/cluster/resources":
					id := 101
					if tc.kind == "lxc" {
						id = 102
					}
					_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{{"vmid": id, "name": "guest", "type": tc.kind, "status": "running", "node": "n1"}}})
				case "/api2/json/nodes/n1/" + tc.kind + "/" + map[string]string{"qemu": "101", "lxc": "102"}[tc.kind] + "/status/current":
					_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "running"}})
				default:
					seen = r.URL.Path
					if seen != tc.endpoint {
						http.NotFound(w, r)
						return
					}
					_, _ = w.Write([]byte(tc.body))
				}
			}))
			defer srv.Close()
			guestID := map[string]string{"qemu": "101", "lxc": "102"}[tc.kind]
			guestKind := hv.KindVM
			if tc.kind == "lxc" {
				guestKind = hv.KindCT
			}
			got, err := clientFor(t, srv).GuestNetwork(context.Background(), hv.Guest{ID: guestID, Node: "n1", Kind: guestKind})
			if err != nil || len(got) == 0 || seen != tc.endpoint {
				t.Fatalf("addresses=%+v path=%s err=%v", got, seen, err)
			}
		})
	}
}

func TestSnapshotAllowsLXC(t *testing.T) {
	var path string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/status/current") {
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "running"}})
			return
		}
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

// A start takes seconds on a real host. The state must be read after the task, not after the POST.
func TestStartWaitsForTheTaskBeforeReadingBack(t *testing.T) {
	var mu sync.Mutex
	status, polls := "stopped", 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		switch {
		case r.URL.Path == "/api2/json/cluster/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"vmid": 100, "name": "a", "type": "lxc", "status": status, "node": "n1"},
			}})
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": status}})
		case r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID:n1:00000001:00000002:00000003:vzstart:100:token@pve:"})
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/log"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"t": "TASK OK"}}})
		case strings.Contains(r.URL.Path, "/tasks/"):
			polls++
			if polls < 3 {
				_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "running", "type": "vzstart"}})
				return
			}
			status = "running"
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "stopped", "exitstatus": "OK", "type": "vzstart"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	c.PollBase, c.PollMax = time.Millisecond, 2*time.Millisecond
	got, err := c.Start(context.Background(), "100")
	if err != nil || got.Status != "running" {
		t.Fatalf("got %+v err %v", got, err)
	}
	if polls < 3 {
		t.Fatalf("read back before the task ended: %d polls", polls)
	}
}

func TestStartReportsAFailedTask(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/api2/json/cluster/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"vmid": 100, "name": "a", "type": "lxc", "status": "stopped", "node": "n1"},
			}})
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "stopped"}})
		case r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": "UPID:n1:00000001:00000002:00000003:vzstart:100:token@pve:"})
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/log"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"t": "no space"}}})
		case strings.Contains(r.URL.Path, "/tasks/"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "stopped", "exitstatus": "no space left", "type": "vzstart"}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	if _, err := clientFor(t, srv).Start(context.Background(), "100"); err == nil || !strings.Contains(err.Error(), "no space left") {
		t.Fatalf("got %v", err)
	}
}

// pvestatd refreshes /cluster/resources about every ten seconds, so on a real server the inventory still
// shows the old status after a task has finished. Only status/current is up to date.
func TestChangesBelieveStatusCurrentOverALaggingInventory(t *testing.T) {
	var mu sync.Mutex
	current, posts := "stopped", 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		upid := map[string]any{"data": "UPID:n1:00000001:00000002:00000003:qmstart:100:token@pve:"}
		switch {
		case r.URL.Path == "/api2/json/cluster/resources":
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]any{
				{"vmid": 100, "name": "a", "type": "qemu", "status": "stopped", "node": "n1"},
			}})
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": current}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/status/start"):
			posts++
			current = "running"
			_ = json.NewEncoder(w).Encode(upid)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/status/stop"):
			posts++
			current = "stopped"
			_ = json.NewEncoder(w).Encode(upid)
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/status"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]string{"status": "stopped", "exitstatus": "OK", "type": "qmstart"}})
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/log"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": []map[string]string{{"t": "TASK OK"}}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := clientFor(t, srv)
	ctx := context.Background()
	got, err := c.Start(ctx, "100")
	if err != nil || got.Status != "running" {
		t.Fatalf("a start that worked must not read as failed: %+v %v", got, err)
	}
	if _, err := c.Start(ctx, "100"); err != nil || posts != 1 {
		t.Fatalf("already running, nothing to send: posts=%d err=%v", posts, err)
	}
	// The inventory still says stopped. Believing it would skip this stop and report success.
	got, err = c.Stop(ctx, "100")
	if err != nil || got.Status != "stopped" || posts != 2 {
		t.Fatalf("the stop must really be sent: %+v posts=%d err=%v", got, posts, err)
	}
}
