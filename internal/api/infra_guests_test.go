package api

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"shippin.cloud/console/internal/hv"
	"shippin.cloud/console/internal/hv/proxmox"
	guests "shippin.cloud/console/internal/infrastructure/server/guest"
	"shippin.cloud/console/internal/infrastructure/servers"
	"shippin.cloud/console/internal/platform/config"
)

const guestServerID = "019a0000-0000-7000-8000-0000000000aa"

// guestsHarness is a panel in front of a Proxmox that answers like a real node. refuse, when set, is what
// creating a guest gets back instead.
type guestsHarness struct {
	a      *API
	dir    *memHV
	mux    *http.ServeMux
	mu     sync.Mutex
	posts  int
	refuse string
}

func (h *guestsHarness) posted() int {
	h.mu.Lock()
	defer h.mu.Unlock()
	return h.posts
}

func newGuestsHarness(t *testing.T) *guestsHarness {
	t.Helper()
	h := &guestsHarness{}
	made := false
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h.mu.Lock()
		defer h.mu.Unlock()
		data := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"data": v}) }
		switch {
		case r.URL.Path == "/api2/json/cluster/nextid":
			data("107")
		case strings.HasSuffix(r.URL.Path, "/content"):
			data([]map[string]string{{"volid": "local:vztmpl/debian.tar.zst"}})
		case strings.HasSuffix(r.URL.Path, "/storage"):
			data([]map[string]string{{"storage": "local", "content": "images,rootdir,vztmpl"}})
		case strings.HasSuffix(r.URL.Path, "/network"):
			data([]map[string]string{{"iface": "vmbr0", "type": "bridge"}})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/qemu"):
			h.posts++
			if h.refuse != "" {
				w.WriteHeader(http.StatusForbidden)
				_, _ = w.Write([]byte(h.refuse))
				return
			}
			made = true
			data("UPID:n1:00000001:00000002:00000003:qmcreate:107:token@pve:")
		case strings.Contains(r.URL.Path, "/tasks/") && strings.HasSuffix(r.URL.Path, "/status"):
			data(map[string]string{"status": "stopped", "exitstatus": "OK"})
		case strings.Contains(r.URL.Path, "/tasks/"):
			data([]map[string]any{{"n": 1, "t": "TASK OK"}})
		case r.URL.Path == "/api2/json/cluster/resources":
			if made {
				data([]map[string]any{{"vmid": 107, "name": "web", "type": "qemu", "status": "stopped", "node": "n1"}})
				return
			}
			data([]map[string]any{})
		case strings.HasSuffix(r.URL.Path, "/status/current"):
			data(map[string]string{"status": "stopped"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	trusted := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	cl, err := proxmox.New(proxmox.Endpoint{BaseURL: srv.URL, Node: "n1", TokenID: "id!tok", CAPEM: trusted}, func() (string, error) { return "s3cret-token-value", nil })
	if err != nil {
		t.Fatal(err)
	}
	h.dir = &memHV{items: []servers.Endpoint{{ID: guestServerID, Slug: "lab", Label: "Lab", Kind: "proxmox", BaseURL: "https://pve.example:8006", Node: "n1", TokenID: "id!tok", TokenEnv: "X"}}}
	h.a = infraAPI(t, "op_infra", h.dir)
	h.a.openPVE = func(servers.Endpoint) (*proxmox.Client, error) { return cl, nil }
	h.a.cfg.UI.Providers = []config.UIProvider{
		{Kind: "proxmox", Images: []config.UIImage{{ID: "local:iso/named.iso", Label: "Named here", Kind: "vm"}}},
		{Kind: "incus", Images: []config.UIImage{{ID: "images:debian/13", Label: "Not this provider"}}},
	}
	h.mux = http.NewServeMux()
	h.a.infraRoutes(h.mux)
	return h
}

// create makes a guest through the action, as the panel's create form does.
func (h *guestsHarness) create(in guests.CreateInput) (hv.Guest, error) {
	in.Server = guestServerID
	return guests.Create.Run(asStaff(h.a), in)
}

func (h *guestsHarness) remove(id, confirm string) error {
	_, err := guests.Delete.Run(asStaff(h.a), guests.DeleteInput{ByID: guests.ByID{Server: guestServerID, ID: id}, Confirm: confirm})
	return err
}

func TestABadNewGuestIsRefusedBeforeAnythingHappens(t *testing.T) {
	h := newGuestsHarness(t)
	for name, in := range map[string]guests.CreateInput{
		"no name":           {Kind: "vm"},
		"no kind":           {Name: "web"},
		"another kind":      {Kind: "pod", Name: "web"},
		"negative memory":   {Kind: "vm", Name: "web", Memory: -1},
		"negative cores":    {Kind: "vm", Name: "web", Cores: -1},
		"negative disk":     {Kind: "vm", Name: "web", Disk: -1},
		"missing placement": {Kind: "vm", Name: "web", Disk: 8},
		"storage only":      {Kind: "vm", Name: "web", Disk: 8, Storage: "local"},
		"network only":      {Kind: "vm", Name: "web", Disk: 8, Network: "vmbr0"},
	} {
		if _, err := h.create(in); refusal(err) != http.StatusBadRequest {
			t.Errorf("%s: got %v", name, err)
		}
	}
	ops, _ := h.dir.Operations(t.Context(), "", 0)
	if h.posted() != 0 || len(ops) != 0 {
		t.Fatalf("something ran: %d posts, %+v", h.posted(), ops)
	}
}

func TestANewGuestIsMadeOnTheProviderAndRecorded(t *testing.T) {
	h := newGuestsHarness(t)
	g, err := h.create(guests.CreateInput{Kind: "vm", Name: "web", Memory: 512, Disk: 8, Storage: "local", Network: "vmbr0"})
	if err != nil || g.ID != "107" {
		t.Fatalf("%v %+v", err, g)
	}
	ops, _ := h.dir.Operations(t.Context(), "", 0)
	if h.posted() == 0 || len(ops) != 1 {
		t.Fatalf("%d posts, %+v", h.posted(), ops)
	}
}

func TestAProviderRefusalReachesTheOperatorWithItsReason(t *testing.T) {
	h := newGuestsHarness(t)
	h.refuse = `{"data":null,"message":"Permission check failed (/storage/local, Datastore.AllocateSpace)\n"}`
	_, err := h.create(guests.CreateInput{Kind: "vm", Name: "web", Disk: 8, Storage: "local", Network: "vmbr0"})
	if refusal(err) != http.StatusBadGateway || !strings.Contains(err.Error(), "Datastore.AllocateSpace") || strings.Contains(err.Error(), "s3cret") {
		t.Fatalf("%v", err)
	}
	ops, _ := h.dir.Operations(t.Context(), "", 0)
	if len(ops) != 1 {
		t.Fatalf("a refused change is still on the record: %+v", ops)
	}
}

func TestImagesAreWhatTheHostHasThenWhatThisInstallationNames(t *testing.T) {
	h := newGuestsHarness(t)
	rec := infraDo(h.mux, http.MethodGet, "/v1/infra/servers/"+guestServerID+"/images", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	var out struct {
		Items []struct{ ID, Label, Kind string } `json:"items"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	if len(out.Items) != 2 || out.Items[0].ID != "local:vztmpl/debian.tar.zst" || out.Items[0].Kind != "ct" || out.Items[1].Label != "Named here" {
		t.Fatalf("%+v", out.Items)
	}
}

func TestDeletingThePanelsOwnGuestNeedsItsOwnConfirmation(t *testing.T) {
	t.Setenv("SHIPPIN_SELF_GUEST", "web")
	h := newGuestsHarness(t)
	if _, err := h.create(guests.CreateInput{Kind: "vm", Name: "web", Memory: 512, Disk: 8, Storage: "local", Network: "vmbr0"}); err != nil {
		t.Fatal(err)
	}
	if err := h.remove("107", "107"); refusal(err) != http.StatusConflict || !strings.Contains(err.Error(), "This panel runs on that guest") {
		t.Fatalf("%v", err)
	}
}

func TestDeletingAGuestMustNameIt(t *testing.T) {
	t.Setenv("SHIPPIN_SELF_GUEST", "someone-else")
	h := newGuestsHarness(t)
	for _, confirm := range []string{"", "108"} {
		if err := h.remove("107", confirm); refusal(err) != http.StatusBadRequest || !strings.Contains(err.Error(), "Confirm what you are removing") {
			t.Fatalf("%q: %v", confirm, err)
		}
	}
	if ops, _ := h.dir.Operations(t.Context(), "", 0); len(ops) != 0 {
		t.Fatalf("an unconfirmed delete ran: %+v", ops)
	}
}
