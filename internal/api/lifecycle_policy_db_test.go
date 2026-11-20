package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/ext"
	"shippin.cloud/console/internal/hostclient"
)

type fakeLifecycle struct {
	prepared, removed []string
	asked             ext.OrgState
}

func (f *fakeLifecycle) Prepare(_ context.Context, rt ext.Runtime, org ext.Org) (string, error) {
	if rt.Pool == nil {
		panic("no runtime")
	}
	f.prepared = append(f.prepared, org.ID)
	return "10.9.9.9", nil
}

func (f *fakeLifecycle) Ready(s ext.OrgState) (bool, string) {
	f.asked = s
	return false, "The probe module is still preparing this organization."
}

func (f *fakeLifecycle) Remove(_ context.Context, _ ext.Runtime, org ext.Org) (string, error) {
	f.removed = append(f.removed, org.ID)
	return "probe/kept", nil
}

func lifecycleDB(t *testing.T) *db.DB {
	t.Helper()
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	d, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	return d
}

// deleteOrg runs the operator's delete of tn and returns the response.
func deleteOrg(t *testing.T, a *API, tn *db.Tenant) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(http.MethodDelete, "/", strings.NewReader(`{"confirm":"`+tn.Name+`"}`))
	r.SetPathValue("tenantId", tn.ID)
	w := httptest.NewRecorder()
	a.opDeleteTenant(w, r, &caller{id: &auth.Identity{Sub: "staff-1"}})
	return w
}

// A module's lifecycle prepares a new organization, decides when work may start in it, and removes
// it; the panel records the status and the address it returned.
func TestAModuleLifecyclePreparesAndRemoves(t *testing.T) {
	d := lifecycleDB(t)
	f := &fakeLifecycle{}
	defer ext.Swap(ext.Module{Name: "probe", Lifecycle: f})()
	a := &API{cfg: testCfg(), db: d}

	org, _ := uuid.NewV7()
	tn, err := a.ensureTenant(context.Background(), org.String(), "Lifecycle Co", "p")
	if err != nil {
		t.Fatal(err)
	}
	if len(f.prepared) != 1 || f.prepared[0] != tn.ID {
		t.Fatalf("the module was not asked to prepare the organization: %v", f.prepared)
	}
	saved, err := d.TenantByID(context.Background(), tn.ID)
	if err != nil || saved.Status != "active" || saved.SSHIP != "10.9.9.9" {
		t.Fatalf("recorded %+v, %v", saved, err)
	}

	if ok, why := a.orgReady(*saved); ok || !strings.Contains(why, "probe module") {
		t.Errorf("readiness must be the module's: %v %q", ok, why)
	}
	if f.asked.ID != tn.ID || f.asked.Status != "active" || f.asked.Address != "10.9.9.9" {
		t.Errorf("the module saw %+v", f.asked)
	}

	w := deleteOrg(t, a, saved)
	var out map[string]string
	_ = json.Unmarshal(w.Body.Bytes(), &out)
	if w.Code != http.StatusOK || out["dataset_kept"] != "probe/kept" || len(f.removed) != 1 || f.removed[0] != tn.ID {
		t.Fatalf("delete: %d %s, removed %v", w.Code, w.Body, f.removed)
	}
	if gone, _ := d.TenantByID(context.Background(), tn.ID); gone == nil || gone.Status != "deleted" {
		t.Errorf("the organization is not marked deleted: %+v", gone)
	}
}

// Without a module the panel prepares and removes an organization through shippin-hostd, and work
// waits until it is active with an address, as before.
func TestNoModuleKeepsTheCurrentLifecycle(t *testing.T) {
	defer ext.Swap()()
	if _, ok := (&API{}).lifecycle().(currentLifecycle); !ok {
		t.Fatal("the panel's own lifecycle changed")
	}
	d := lifecycleDB(t)
	var calls []string
	hostd := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.Path)
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/provision"):
			_, _ = w.Write([]byte(`{"ssh_ip":"10.1.2.3"}`))
		case r.Method == http.MethodDelete:
			_, _ = w.Write([]byte(`{"dataset_kept":"pool/t-kept"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer hostd.Close()
	a := &API{cfg: testCfg(), db: d, host: hostclient.New(hostd.URL)}

	org, _ := uuid.NewV7()
	tn, err := a.ensureTenant(context.Background(), org.String(), "Hostd Co", "p")
	if err != nil {
		t.Fatal(err)
	}
	if tn.Status != "active" || tn.SSHIP != "10.1.2.3" || len(calls) != 1 || calls[0] != "POST /tenants/"+tn.ID+"/provision" {
		t.Fatalf("prepare: %+v, hostd saw %v", tn, calls)
	}

	for _, s := range []ext.OrgState{{Status: "active"}, {Status: "provisioning", Address: "10.1.2.3"}} {
		if ok, why := a.lifecycle().Ready(s); ok || why != "This tenant isn't provisioned yet." {
			t.Errorf("%+v must wait: %v %q", s, ok, why)
		}
	}
	if ok, _ := a.orgReady(*tn); !ok {
		t.Error("an active organization with an address must be ready")
	}

	if w := deleteOrg(t, a, tn); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "pool/t-kept") {
		t.Fatalf("delete: %d %s", w.Code, w.Body)
	}
	if calls[len(calls)-1] != "DELETE /tenants/"+tn.ID {
		t.Errorf("hostd saw %v", calls)
	}
}
