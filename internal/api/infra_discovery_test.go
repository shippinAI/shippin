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
	"shippin.cloud/console/internal/hv/incus"
	"shippin.cloud/console/internal/infrastructure/servers"
)

// Existing provider resources need no Shippin creation or import record. Exercise the real
// Incus driver through the authenticated panel API, including changes made outside the panel.
func TestIncusDiscoveryManagesExistingGuestsAndTracksExternalChanges(t *testing.T) {
	t.Setenv("SHIPPIN_SELF_GUEST", "panel-on-another-host")
	var mu sync.Mutex
	guests := []map[string]string{
		{"name": "existing-vm", "type": "virtual-machine", "status": "Running"},
		{"name": "existing-container", "type": "container", "status": "Stopped"},
	}
	unavailable, writes, creates := false, 0, 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer fixture-token" || r.URL.Query().Get("project") != "operations" {
			http.Error(w, "credential or project mismatch", http.StatusForbidden)
			return
		}
		if unavailable {
			http.Error(w, "inventory unavailable", http.StatusServiceUnavailable)
			return
		}
		respond := func(v any) { _ = json.NewEncoder(w).Encode(map[string]any{"metadata": v}) }
		if r.URL.Path == "/1.0/instances" {
			if r.Method != http.MethodGet {
				creates++
				http.Error(w, "discovery must not create a guest", http.StatusBadRequest)
				return
			}
			if r.URL.Query().Get("recursion") != "1" {
				t.Errorf("inventory request must read provider records: %s", r.URL)
			}
			respond(guests)
			return
		}
		for _, guest := range guests {
			base := "/1.0/instances/" + guest["name"]
			switch {
			case r.Method == http.MethodGet && r.URL.Path == base:
				respond(guest)
				return
			case r.Method == http.MethodPut && r.URL.Path == base+"/state":
				var in struct{ Action string }
				if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Action != "stop" {
					t.Errorf("unexpected power request: %+v %v", in, err)
				}
				writes++
				guest["status"] = "Stopped"
				respond(map[string]string{})
				return
			case r.Method == http.MethodPost && r.URL.Path == base+"/snapshots":
				var in struct{ Name string }
				if err := json.NewDecoder(r.Body).Decode(&in); err != nil || in.Name != "before-change" {
					t.Errorf("unexpected snapshot request: %+v %v", in, err)
				}
				writes++
				respond(map[string]string{})
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(srv.Close)
	cert := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	driver, err := incus.New(incus.Endpoint{BaseURL: srv.URL, Project: "operations", CAPEM: cert}, func() (string, error) { return "fixture-token", nil })
	if err != nil {
		t.Fatal(err)
	}
	dir := &memHV{items: []servers.Endpoint{{ID: guestServerID, Slug: "test-host", Kind: "incus", BaseURL: srv.URL, Node: "operations"}}}
	a := infraAPI(t, "op_infra", dir)
	a.openHV = func(servers.Endpoint) (hv.Driver, error) { return driver, nil }
	mux := http.NewServeMux()
	a.infraRoutes(mux)
	base := "/v1/infra/servers/" + guestServerID
	list := func(want int) []hv.Guest {
		t.Helper()
		rec := infraDo(mux, http.MethodGet, base+"/guests", "")
		var out struct{ Items []hv.Guest }
		if rec.Code != http.StatusOK || json.Unmarshal(rec.Body.Bytes(), &out) != nil || len(out.Items) != want {
			t.Fatalf("want %d discovered guests: %d %s", want, rec.Code, rec.Body)
		}
		return out.Items
	}
	got := list(2)
	if got[0].ID != "existing-vm" || got[0].Kind != "vm" || got[1].Kind != "ct" {
		t.Fatalf("provider identities or kinds lost: %+v", got)
	}
	mu.Lock()
	guests = append(guests, map[string]string{"name": "external-vm", "type": "virtual-machine", "status": "Running"})
	mu.Unlock()
	if got := list(3); got[2].ID != "external-vm" {
		t.Fatalf("external creation missing: %+v", got)
	}
	for _, action := range []struct{ path, body string }{
		{"/guests/external-vm/status/stop", ""},
		{"/guests/external-vm/snapshots", `{"snapname":"before-change"}`},
	} {
		rec := infraDo(mux, http.MethodPost, base+action.path, action.body)
		if rec.Code != http.StatusOK {
			t.Fatalf("existing guest action failed: %d %s", rec.Code, rec.Body)
		}
	}
	if got := list(3); got[2].Status != "Stopped" {
		t.Fatalf("power action not read back: %+v", got)
	}
	denied := infraAPI(t, "org_owner", dir)
	denied.openHV = a.openHV
	deniedMux := http.NewServeMux()
	denied.infraRoutes(deniedMux)
	for _, request := range []struct{ method, path string }{
		{http.MethodGet, "/guests"}, {http.MethodPost, "/guests/external-vm/status/stop"},
	} {
		if rec := infraDo(deniedMux, request.method, base+request.path, ""); rec.Code != http.StatusForbidden {
			t.Fatalf("discovery must not grant organization owners staff authority: %d %s", rec.Code, rec.Body)
		}
	}
	mu.Lock()
	guests = guests[1:]
	mu.Unlock()
	if got := list(2); got[0].ID != "existing-container" {
		t.Fatalf("external removal still listed: %+v", got)
	}
	mu.Lock()
	unavailable = true
	mu.Unlock()
	if rec := infraDo(mux, http.MethodGet, base+"/guests", ""); rec.Code != http.StatusBadGateway || strings.Contains(rec.Body.String(), `"items"`) {
		t.Fatalf("an outage must not look like empty inventory: %d %s", rec.Code, rec.Body)
	}
	mu.Lock()
	unavailable = false
	mu.Unlock()
	list(2)
	if rec := infraDo(mux, http.MethodDelete, base+"/attachment", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("detach: %d %s", rec.Code, rec.Body)
	}
	mu.Lock()
	defer mu.Unlock()
	if writes != 2 || creates != 0 || len(guests) != 2 {
		t.Fatalf("unexpected provider writes or destructive detach: writes=%d creates=%d guests=%d", writes, creates, len(guests))
	}
	ops, _ := dir.Operations(t.Context(), guestServerID, 10)
	if len(ops) != 2 || ops[0].Status != servers.OpSucceeded || ops[1].Status != servers.OpSucceeded {
		t.Fatalf("discovered guest management must be recorded: %+v", ops)
	}
}
