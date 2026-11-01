package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/jobs"
)

// jobAPI is notifyAPI with a fake hostd answering workspace creates with codes[i] in turn.
func jobAPI(t *testing.T, codes ...int) (*API, *db.DB, db.Tenant, *atomic.Int32) {
	a, d, tn := notifyAPI(t)
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			_ = json.NewEncoder(w).Encode(map[string]string{"Progress": "Starting"})
			return
		}
		code := codes[min(int(calls.Add(1))-1, len(codes)-1)]
		w.WriteHeader(code)
		if code >= 300 {
			_ = json.NewEncoder(w).Encode(map[string]string{"Error": "hostd says no"})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]string{"IPv4": "203.0.113.7"})
	}))
	t.Cleanup(srv.Close)
	a.host = hostclient.New(srv.URL)
	a.jobs = jobs.NewStore(d.Pool)
	// Jobs left queued by other tests in this shared database would eat the fake's answers.
	if _, err := d.Pool.Exec(context.Background(), `DELETE FROM jobs WHERE status IN ('queued','running')`); err != nil {
		t.Fatal(err)
	}
	a.cfg.Jobs.MaxAttempts = 3
	a.runner = jobs.NewRunner(a.jobs, jobs.Settings{Workers: 1, Lease: 5 * time.Second, Heartbeat: time.Second,
		Poll: 20 * time.Millisecond, BackoffBase: 10 * time.Millisecond, BackoffMax: 10 * time.Millisecond})
	a.registerJobs(a.runner)
	ctx, cancel := context.WithCancel(context.Background())
	t.Cleanup(cancel)
	go a.runner.Run(ctx)
	return a, d, tn, &calls
}

func provisioningWS(t *testing.T, a *API, d *db.DB, tn db.Tenant) *db.Workspace {
	id := newUUID()
	ws := &db.Workspace{ID: id, TenantID: tn.ID, Name: "w" + id[len(id)-8:], State: "provisioning",
		CreatedBy: "u", Host: id + ".ws.test"}
	if err := d.CreateWorkspace(context.Background(), ws, a.cfg.Workspaces.PortMin, a.cfg.Workspaces.PortMax); err != nil {
		t.Fatal(err)
	}
	return ws
}

func finished(t *testing.T, a *API, id string) *jobs.Job {
	t.Helper()
	for range 300 {
		if j, _ := a.jobs.Get(context.Background(), id); j != nil && j.Done() {
			return j
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("job did not finish")
	return nil
}

func TestWorkspaceJobRetriesHostdThenRuns(t *testing.T) {
	a, d, tn, calls := jobAPI(t, 503, 200)
	ws := provisioningWS(t, a, d, tn)
	j, _, err := a.jobs.Enqueue(context.Background(), a.workspaceJob(*ws, "u"))
	if err != nil {
		t.Fatal(err)
	}
	a.runner.Wake()
	if got := finished(t, a, j.ID); got.Status != jobs.Succeeded || got.Attempts != 2 || calls.Load() != 2 {
		t.Fatalf("got %+v after %d calls", got, calls.Load())
	}
	after, _ := d.WorkspaceByID(context.Background(), ws.ID)
	if after.State != "running" || after.IPv4 != "203.0.113.7" {
		t.Fatalf("workspace %s %s", after.State, after.IPv4)
	}
}

func TestWorkspaceJobRejectedFailsWorkspace(t *testing.T) {
	a, d, tn, calls := jobAPI(t, 400)
	ws := provisioningWS(t, a, d, tn)
	j, _, _ := a.jobs.Enqueue(context.Background(), a.workspaceJob(*ws, "u"))
	if got := finished(t, a, j.ID); got.Status != jobs.Failed || calls.Load() != 1 {
		t.Fatalf("a hostd 400 must fail at once: %+v after %d calls", got, calls.Load())
	}
	if after, _ := d.WorkspaceByID(context.Background(), ws.ID); after.State != "failed" {
		t.Fatalf("workspace state %s", after.State)
	}
}

func TestGetJobVisibility(t *testing.T) {
	a, d, tn, _ := jobAPI(t, 400)
	ws := provisioningWS(t, a, d, tn)
	j, _, _ := a.jobs.Enqueue(context.Background(), a.workspaceJob(*ws, "u"))
	finished(t, a, j.ID)
	get := func(c *caller) *httptest.ResponseRecorder {
		a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) { return c, true }
		r := httptest.NewRequest("GET", "/v1/jobs/"+j.ID, nil)
		r.SetPathValue("jobId", j.ID)
		w := httptest.NewRecorder()
		a.getJob(w, r)
		return w
	}
	id := &auth.Identity{Sub: "s"}
	member := &caller{id: id, tenants: []tenantView{{Tenant: tn, Roles: []string{"org_member"}}}}
	if w := get(member); w.Code != 200 || !json.Valid(w.Body.Bytes()) {
		t.Fatalf("member: %d", w.Code)
	} else if b := w.Body.String(); !strings.Contains(b, `"status":"failed"`) || strings.Contains(b, "hostd says no") {
		t.Fatalf("member body leaks or misses status: %s", b)
	}
	grantee := &caller{id: id, tenants: []tenantView{{Tenant: tn, Roles: []string{}, grants: []db.Grant{{}}}}}
	if w := get(grantee); w.Code != 404 {
		t.Fatalf("grantee: %d", w.Code)
	}
	if w := get(&caller{id: id}); w.Code != 404 {
		t.Fatalf("stranger: %d", w.Code)
	}
	if w := get(&caller{id: id, op: "op_support"}); w.Code != 200 || !strings.Contains(w.Body.String(), "hostd says no") {
		t.Fatalf("operator: %d %s", w.Code, w.Body)
	}
}
