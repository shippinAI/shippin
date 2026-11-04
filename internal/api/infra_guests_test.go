package api

import (
	"encoding/json"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/hv/proxmox"
	"shippin.cloud/console/internal/servers"
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

func TestABadNewGuestIsRefusedBeforeAnythingHappens(t *testing.T) {
	h := newGuestsHarness(t)
	for name, body := range map[string]string{
		"no name":              `{"kind":"vm"}`,
		"no kind":              `{"name":"web"}`,
		"another kind":         `{"kind":"pod","name":"web"}`,
		"negative memory":      `{"kind":"vm","name":"web","memory":-1}`,
		"negative cores":       `{"kind":"vm","name":"web","cores":-1}`,
		"negative disk":        `{"kind":"vm","name":"web","disk":-1}`,
		"missing placement":    `{"kind":"vm","name":"web","disk":8}`,
		"storage only":         `{"kind":"vm","name":"web","disk":8,"storage":"local"}`,
		"network only":         `{"kind":"vm","name":"web","disk":8,"network":"vmbr0"}`,
		"a field nobody knows": `{"kind":"vm","name":"web","root_password":"x"}`,
	} {
		rec := infraDo(h.mux, http.MethodPost, "/v1/infra/servers/"+guestServerID+"/guests", body)
		if rec.Code != http.StatusBadRequest {
			t.Errorf("%s: got %d %s", name, rec.Code, rec.Body)
		}
	}
	ops, _ := h.dir.Operations(t.Context(), "", 0)
	if h.posted() != 0 || len(ops) != 0 {
		t.Fatalf("something ran: %d posts, %+v", h.posted(), ops)
	}
}

func TestANewGuestWaitsForAgentHubBeforeProviderCall(t *testing.T) {
	h := newGuestsHarness(t)
	rec := infraDo(h.mux, http.MethodPost, "/v1/infra/servers/"+guestServerID+"/guests", `{"kind":"vm","name":"web","memory":512,"disk":8,"storage":"local","network":"vmbr0"}`)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), `"code":"agenthub_review_unavailable"`) {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	ops, _ := h.dir.Operations(t.Context(), "", 0)
	if h.posted() != 0 || len(ops) != 0 {
		t.Fatalf("%+v", ops)
	}
}

func TestProviderIsNotCalledBeforeAgentHubReview(t *testing.T) {
	h := newGuestsHarness(t)
	h.refuse = `{"data":null,"message":"Permission check failed (/storage/local, Datastore.AllocateSpace)\n"}`
	rec := infraDo(h.mux, http.MethodPost, "/v1/infra/servers/"+guestServerID+"/guests", `{"kind":"vm","name":"web","disk":8,"storage":"local","network":"vmbr0"}`)
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "agenthub_review_unavailable") || strings.Contains(rec.Body.String(), "s3cret") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
	ops, _ := h.dir.Operations(t.Context(), "", 0)
	if h.posted() != 0 || len(ops) != 0 {
		t.Fatalf("%+v", ops)
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

func TestDeletingThePanelsOwnGuestStillWaitsForAgentHub(t *testing.T) {
	t.Setenv("SHIPPIN_SELF_GUEST", "web")
	h := newGuestsHarness(t)
	rec := infraDo(h.mux, http.MethodDelete, "/v1/infra/servers/"+guestServerID+"/guests/web", "")
	if rec.Code != http.StatusServiceUnavailable || !strings.Contains(rec.Body.String(), "agenthub_review_unavailable") {
		t.Fatalf("%d %s", rec.Code, rec.Body)
	}
}
