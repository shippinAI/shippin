package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/ext"
	"shippin.cloud/console/internal/infrastructure/hostclient"
	"shippin.cloud/console/internal/services/jobs"
	"shippin.cloud/console/internal/tenancy/projects"
)

// readyRule is a module lifecycle whose Ready is a fixed answer, not "status active".
type readyRule struct {
	allow bool
	why   string
	saw   []ext.OrgState
}

func (r *readyRule) Prepare(context.Context, ext.Runtime, ext.Org) (string, error) { return "", nil }
func (r *readyRule) Ready(s ext.OrgState) (bool, string) {
	r.saw = append(r.saw, s)
	return r.allow, r.why
}
func (r *readyRule) Remove(context.Context, ext.Runtime, ext.Org) (string, error) { return "", nil }

// readyFixture is an API, one organization, and the hostd calls made for it.
func readyFixture(t *testing.T, status, addr string, rule *readyRule) (*API, *db.DB, db.Tenant, *[]string) {
	t.Helper()
	a, d, tn := notifyAPI(t)
	ctx := context.Background()
	if status != "" {
		if err := d.SetTenantStatus(ctx, tn.ID, status, addr, ""); err != nil {
			t.Fatal(err)
		}
		tn.Status, tn.SSHIP = status, addr
	}
	if rule != nil {
		t.Cleanup(ext.Swap(ext.Module{Name: "probe-ready", Lifecycle: rule}))
	} else {
		t.Cleanup(ext.Swap())
	}
	hits := &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*hits = append(*hits, r.Method+" "+r.URL.Path)
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/instances") {
			_, _ = w.Write([]byte(`{"metadata":[],"partial":false}`))
			return
		}
		_ = json.NewEncoder(w).Encode(hostclient.TenantVM{Mode: "vm", Name: "tvm-test", CPU: "8",
			MemLimit: "16GiB", RootSize: "50GiB", DataSize: "200GiB"})
	}))
	t.Cleanup(srv.Close)
	a.host = hostclient.New(srv.URL)
	return a, d, tn, hits
}

func saw(hits []string, part string) bool {
	for _, h := range hits {
		if strings.Contains(h, part) {
			return true
		}
	}
	return false
}

func problemMessage(t *testing.T, w *httptest.ResponseRecorder) string {
	t.Helper()
	var out struct{ Message string }
	if err := json.Unmarshal(w.Body.Bytes(), &out); err != nil {
		t.Fatal(err)
	}
	return out.Message
}

func opCall(a *API, method, tenant, action string, h func(http.ResponseWriter, *http.Request, *caller)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, "/", nil)
	r.SetPathValue("tenantId", tenant)
	if action != "" {
		r.SetPathValue("action", action)
	}
	w := httptest.NewRecorder()
	h(w, r, &caller{id: &auth.Identity{Sub: "staff-1", Name: "Staff"}})
	return w
}

func TestBoundaryJobHonoursReadiness(t *testing.T) {
	plan := func(t *testing.T, d *db.DB, id string) string {
		t.Helper()
		b, err := (&projects.Store{Pool: d.Pool}).PlanBoundary(context.Background(), id, 8, 16384, 50, 200)
		if err != nil {
			t.Fatal(err)
		}
		return b
	}
	run := func(a *API, tn db.Tenant, b string) error {
		_, err := a.runBoundaryJob(context.Background(), &jobs.Job{TenantID: tn.ID, TargetID: b, Attempts: 1, MaxAttempts: 3}, func(string) {})
		return err
	}

	t.Run("a module can refuse an active organization", func(t *testing.T) {
		rule := &readyRule{why: "The probe module is still preparing this organization."}
		a, d, tn, hits := readyFixture(t, "active", "203.0.113.9", rule)
		err := run(a, tn, plan(t, d, tn.ID))
		if err == nil || !strings.Contains(err.Error(), "probe module") {
			t.Fatalf("got %v", err)
		}
		if saw(*hits, "/vm") {
			t.Fatalf("hostd was called: %v", *hits)
		}
		if len(rule.saw) != 1 || rule.saw[0].ID != tn.ID || rule.saw[0].Status != "active" || rule.saw[0].Address != "203.0.113.9" {
			t.Fatalf("module saw %+v", rule.saw)
		}
	})

	t.Run("a module can allow an organization that is not active", func(t *testing.T) {
		a, d, tn, hits := readyFixture(t, "", "", &readyRule{allow: true})
		if err := run(a, tn, plan(t, d, tn.ID)); err != nil {
			t.Fatal(err)
		}
		if !saw(*hits, "/tenants/"+tn.ID+"/vm") {
			t.Fatalf("hostd was not asked: %v", *hits)
		}
	})

	t.Run("with no module an active organization needs no address", func(t *testing.T) {
		a, d, tn, hits := readyFixture(t, "active", "", nil)
		if err := run(a, tn, plan(t, d, tn.ID)); err != nil {
			t.Fatal(err)
		}
		if !saw(*hits, "/vm") {
			t.Fatalf("hostd was not asked: %v", *hits)
		}
	})

	t.Run("with no module a tenant that is not active still waits", func(t *testing.T) {
		a, d, tn, hits := readyFixture(t, "", "", nil)
		err := run(a, tn, plan(t, d, tn.ID))
		if err == nil || err.Error() != "tenant is provisioning; waiting for it to be active" {
			t.Fatalf("got %v", err)
		}
		if saw(*hits, "/vm") {
			t.Fatalf("hostd was called: %v", *hits)
		}
	})
}

func TestTenantVMHonoursReadiness(t *testing.T) {
	t.Run("enable", func(t *testing.T) {
		t.Run("a module can refuse an active organization", func(t *testing.T) {
			a, _, tn, hits := readyFixture(t, "active", "203.0.113.9", &readyRule{why: "The probe module is still preparing this organization."})
			w := opCall(a, http.MethodPost, tn.ID, "", a.opEnableTenantVM)
			if w.Code != http.StatusConflict || !strings.Contains(problemMessage(t, w), "probe module") {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if saw(*hits, "/vm") {
				t.Fatalf("hostd was called: %v", *hits)
			}
		})
		t.Run("a module can refuse a failed organization", func(t *testing.T) {
			a, _, tn, hits := readyFixture(t, "failed", "", &readyRule{why: "The probe module is still preparing this organization."})
			w := opCall(a, http.MethodPost, tn.ID, "", a.opEnableTenantVM)
			if w.Code != http.StatusConflict || !strings.Contains(problemMessage(t, w), "probe module") {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if saw(*hits, "/vm") {
				t.Fatalf("hostd was called: %v", *hits)
			}
		})
		t.Run("a module can allow an organization that is not active", func(t *testing.T) {
			a, _, tn, hits := readyFixture(t, "suspended", "", &readyRule{allow: true})
			w := opCall(a, http.MethodPost, tn.ID, "", a.opEnableTenantVM)
			if w.Code != http.StatusOK {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if !saw(*hits, "/tenants/"+tn.ID+"/vm") {
				t.Fatalf("hostd was not asked: %v", *hits)
			}
		})
		t.Run("with no module a failed organization can still be made VM-backed", func(t *testing.T) {
			a, _, tn, hits := readyFixture(t, "failed", "", nil)
			w := opCall(a, http.MethodPost, tn.ID, "", a.opEnableTenantVM)
			if w.Code != http.StatusOK {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if !saw(*hits, "/vm") {
				t.Fatalf("hostd was not asked: %v", *hits)
			}
		})
		t.Run("with no module a suspended organization is still refused", func(t *testing.T) {
			a, _, tn, hits := readyFixture(t, "suspended", "", nil)
			w := opCall(a, http.MethodPost, tn.ID, "", a.opEnableTenantVM)
			if w.Code != http.StatusConflict || problemMessage(t, w) != "Tenant is suspended." {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if saw(*hits, "/vm") {
				t.Fatalf("hostd was called: %v", *hits)
			}
		})
	})

	t.Run("start and stop", func(t *testing.T) {
		t.Run("a module can refuse a start", func(t *testing.T) {
			a, _, tn, hits := readyFixture(t, "active", "203.0.113.9", &readyRule{why: "The probe module is still preparing this organization."})
			w := opCall(a, http.MethodPost, tn.ID, "start", a.opTenantVMAction)
			if w.Code != http.StatusConflict || !strings.Contains(problemMessage(t, w), "probe module") {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if saw(*hits, "/vm") {
				t.Fatalf("hostd was called: %v", *hits)
			}
		})
		t.Run("a module can allow a start that is not on an active organization", func(t *testing.T) {
			a, _, tn, hits := readyFixture(t, "suspended", "", &readyRule{allow: true})
			w := opCall(a, http.MethodPost, tn.ID, "start", a.opTenantVMAction)
			if w.Code != http.StatusOK {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if !saw(*hits, "/tenants/"+tn.ID+"/vm/start") {
				t.Fatalf("hostd was not asked: %v", *hits)
			}
		})
		t.Run("a stop is still allowed when the module says not ready", func(t *testing.T) {
			a, _, tn, hits := readyFixture(t, "active", "203.0.113.9", &readyRule{why: "The probe module is still preparing this organization."})
			w := opCall(a, http.MethodPost, tn.ID, "stop", a.opTenantVMAction)
			if w.Code != http.StatusOK {
				t.Fatalf("%d %s", w.Code, w.Body)
			}
			if !saw(*hits, "/vm/stop") {
				t.Fatalf("hostd was not asked: %v", *hits)
			}
		})
		t.Run("with no module a start still requires status active, and a stop does not", func(t *testing.T) {
			a, _, tn, hits := readyFixture(t, "provisioning", "", nil)
			start := opCall(a, http.MethodPost, tn.ID, "start", a.opTenantVMAction)
			if start.Code != http.StatusConflict || problemMessage(t, start) != "Tenant is provisioning." {
				t.Fatalf("start %d %s", start.Code, start.Body)
			}
			stop := opCall(a, http.MethodPost, tn.ID, "stop", a.opTenantVMAction)
			if stop.Code != http.StatusOK || !saw(*hits, "/vm/stop") {
				t.Fatalf("stop %d %s hits %v", stop.Code, stop.Body, *hits)
			}
		})
	})
}

func TestDriftHonoursReadiness(t *testing.T) {
	t.Run("a module can skip an active organization", func(t *testing.T) {
		a, _, tn, hits := readyFixture(t, "active", "203.0.113.9", &readyRule{why: "The probe module is still preparing this organization."})
		a.checkDrift(context.Background())
		if saw(*hits, "/tenants/"+tn.ID+"/instances") {
			t.Fatalf("drift read inventory: %v", *hits)
		}
	})
	t.Run("a module can include an organization that is not active", func(t *testing.T) {
		a, _, tn, hits := readyFixture(t, "suspended", "", &readyRule{allow: true})
		a.checkDrift(context.Background())
		if !saw(*hits, "/tenants/"+tn.ID+"/instances") {
			t.Fatalf("drift skipped the organization: %v", *hits)
		}
	})
	t.Run("with no module an active organization is checked without an address", func(t *testing.T) {
		a, _, tn, hits := readyFixture(t, "active", "", nil)
		a.checkDrift(context.Background())
		if !saw(*hits, "/tenants/"+tn.ID+"/instances") {
			t.Fatalf("drift skipped the organization: %v", *hits)
		}
	})
	t.Run("with no module an organization that is not active is skipped", func(t *testing.T) {
		a, _, tn, hits := readyFixture(t, "", "", nil)
		a.checkDrift(context.Background())
		if saw(*hits, "/tenants/"+tn.ID+"/instances") {
			t.Fatalf("drift read inventory: %v", *hits)
		}
	})
}
