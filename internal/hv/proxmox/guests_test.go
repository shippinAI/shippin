package proxmox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"testing"

	"shippin.cloud/console/internal/hv"
)

// A fake with what a real node has: a next id, storage that says what it holds, a bridge, and an inventory
// that does not show a new guest straight away.
func guestServer(t *testing.T, sent *url.Values) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	asks := 0
	made := false
	return httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		data := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"data": v}) }
		switch {
		case r.URL.Path == "/api2/json/cluster/nextid":
			data("107")
		case strings.HasSuffix(r.URL.Path, "/storage"):
			data([]map[string]string{{"storage": "backups", "content": "backup"}, {"storage": "local", "content": "images,rootdir,vztmpl,iso"}})
		case strings.HasSuffix(r.URL.Path, "/network"):
			data([]map[string]string{{"iface": "eth0", "type": "eth"}, {"iface": "vmbr0", "type": "bridge"}, {"iface": "vmbr1", "type": "OVSBridge"}})
		case r.Method == http.MethodPost && (strings.HasSuffix(r.URL.Path, "/lxc") || strings.HasSuffix(r.URL.Path, "/qemu")):
			_ = r.ParseForm()
			*sent = r.PostForm
			made = true
			data("UPID:n1:00000001:00000002:00000003:vzcreate:107:token@pve:")
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/status"):
			data(map[string]string{"status": "stopped", "exitstatus": "OK"})
		case strings.Contains(r.URL.Path, "/tasks/"):
			data([]map[string]string{{"t": "TASK OK"}})
		case r.URL.Path == "/api2/json/cluster/resources":
			asks++
			if made && asks > 2 {
				data([]map[string]any{{"vmid": 107, "name": "web", "type": "lxc", "status": "stopped", "node": "n1"}})
				return
			}
			data([]map[string]any{})
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			data(map[string]string{"status": "stopped"})
		default:
			http.NotFound(w, r)
		}
	}))
}

func TestCreateContainerSendsWhatARealNodeNeeds(t *testing.T) {
	var sent url.Values
	srv := guestServer(t, &sent)
	defer srv.Close()
	c := clientFor(t, srv)
	g, err := c.CreateGuest(context.Background(), hv.GuestSpec{Kind: hv.KindCT, Name: "web", Image: "local:vztmpl/debian.tar.zst", Storage: "local", Network: "vmbr0", Memory: 512, Cores: 2, Disk: 4})
	if err != nil || g.ID != "107" {
		t.Fatalf("a guest the inventory is slow to show is still a guest: %+v %v", g, err)
	}
	for k, want := range map[string]string{"vmid": "107", "hostname": "web", "ostemplate": "local:vztmpl/debian.tar.zst", "rootfs": "local:4", "net0": "name=eth0,bridge=vmbr0,ip=dhcp", "memory": "512", "cores": "2"} {
		if sent.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, sent.Get(k), want)
		}
	}
}

func TestCreateVirtualMachineNeedsNoImage(t *testing.T) {
	var sent url.Values
	srv := guestServer(t, &sent)
	defer srv.Close()
	c := clientFor(t, srv)
	if _, err := c.CreateGuest(context.Background(), hv.GuestSpec{Kind: hv.KindVM, Name: "web", Storage: "local", Network: "vmbr0", Disk: 12}); err != nil {
		t.Fatal(err)
	}
	for k, want := range map[string]string{"name": "web", "scsi0": "local:12", "net0": "virtio,bridge=vmbr0"} {
		if sent.Get(k) != want {
			t.Errorf("%s = %q, want %q", k, sent.Get(k), want)
		}
	}
	if sent.Get("ide2") != "" {
		t.Errorf("no installer was asked for: %q", sent.Get("ide2"))
	}
}

func TestCreateRefusesBeforeSendingAnything(t *testing.T) {
	var sent url.Values
	srv := guestServer(t, &sent)
	defer srv.Close()
	c := clientFor(t, srv)
	for name, spec := range map[string]hv.GuestSpec{
		"a bad name":            {Kind: hv.KindVM, Name: "1 bad"},
		"a container, no image": {Kind: hv.KindCT, Name: "web"},
		"no explicit storage":   {Kind: hv.KindVM, Name: "web", Network: "vmbr0", Disk: 8},
		"no explicit network":   {Kind: hv.KindVM, Name: "web", Storage: "local", Disk: 8},
		"no disk size":          {Kind: hv.KindVM, Name: "web", Storage: "local", Network: "vmbr0"},
		"storage lacks images":  {Kind: hv.KindVM, Name: "web", Storage: "backups", Network: "vmbr0", Disk: 8},
		"network is not bridge": {Kind: hv.KindVM, Name: "web", Storage: "local", Network: "eth0", Disk: 8},
	} {
		if _, err := c.CreateGuest(context.Background(), spec); err == nil {
			t.Errorf("%s must be refused", name)
		}
	}
	if sent != nil {
		t.Fatalf("nothing should have been sent: %v", sent)
	}
}

func TestCreateAcceptsProviderReportedOVSBridge(t *testing.T) {
	var sent url.Values
	srv := guestServer(t, &sent)
	defer srv.Close()
	c := clientFor(t, srv)
	if _, err := c.CreateGuest(context.Background(), hv.GuestSpec{Kind: hv.KindVM, Name: "web", Storage: "local", Network: "vmbr1", Disk: 8}); err != nil {
		t.Fatal(err)
	}
	if sent.Get("net0") != "virtio,bridge=vmbr1" {
		t.Fatalf("selected OVS bridge was not used: %s", sent.Get("net0"))
	}
}

// The first of these is what Proxmox VE 9.2 sends to a token without the role it needs.
func TestARefusalSaysWhy(t *testing.T) {
	for body, want := range map[string]string{
		`{"data":null,"message":"Permission check failed (/storage/local, Datastore.AllocateSpace)\n"}`:   "Permission check failed (/storage/local, Datastore.AllocateSpace)",
		`{"data":null,"message":"Parameter verification failed.\n","errors":{"name":"invalid format\n"}}`: "Parameter verification failed. name: invalid format",
		`Bad Gateway`: "Bad Gateway",
	} {
		if got := refusal([]byte(body)); got != want {
			t.Errorf("%q, want %q", got, want)
		}
	}
}
