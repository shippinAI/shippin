package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/managed"
)

// TestRRFlowPerVM (D36): two pre-D36 instances on one VM move (MigrateTenant) into one shared instance with
// a namespace each; the job moves their state, re-binds both devspaces; new attaches reuse it.
func TestRRFlowPerVM(t *testing.T) {
	a, d, tn := notifyAPI(t)
	ctx := context.Background()
	a.svcs = &managed.Store{Pool: d.Pool}
	a.cfg.Services.RRFlow = config.ServiceToggle{Enabled: true, ListedOnly: true, Tenants: []string{tn.ID}}
	var mu sync.Mutex
	var calls []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, r.Method+" "+r.URL.String())
		mu.Unlock()
		_, _ = w.Write([]byte(`{"ipv4":"10.128.9.5","port":7710}`))
	}))
	t.Cleanup(srv.Close)
	a.host = hostclient.New(srv.URL)
	b, p1, p2 := newUUID(), newUUID(), newUUID()
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.Pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	exec(`INSERT INTO boundaries(id,tenant_id,state,cpu,memory_mib,root_gib,data_gib) VALUES($1,$2,'ready',2,2048,20,20)`, b, tn.ID)
	for i, p := range []string{p1, p2} {
		exec(`INSERT INTO projects(id,tenant_id,boundary_id,name,created_by) VALUES($1,$2,$3,$4,'u')`, p, tn.ID, b, "p"+p[len(p)-6:]+string(rune('a'+i)))
	}
	ws := map[string]*db.Workspace{}
	old := map[string]string{}
	for _, p := range []string{p1, p2} {
		w := provisioningWS(t, a, d, tn)
		exec(`UPDATE workspaces SET project_id=$1 WHERE id=$2`, p, w.ID)
		ws[p] = w
		old[p] = newUUID()
		exec(`INSERT INTO service_instances(id,tenant_id,project_id,boundary_id,kind,state,created_by) VALUES($1,$2,$3,$4,'rrflow-state','ready','u')`,
			old[p], tn.ID, p, b)
		exec(`INSERT INTO service_bindings(id,tenant_id,instance_id,workspace_id,state) VALUES($1,$2,$3,$4,'bound')`, newUUID(), tn.ID, old[p], w.ID)
	}
	oldJob := newUUID()
	exec(`INSERT INTO jobs(id,tenant_id,kind,target_type,target_id,created_by) VALUES($1,$2,'service.provision','service_instance',$3,'u')`,
		oldJob, tn.ID, old[p1])
	_, err1 := a.svcs.MigrateTenant(ctx, tn.ID, "u") // the operator setup route's per-tenant move
	if _, err2 := a.svcs.MigrateTenant(ctx, tn.ID, "u"); err1 != nil || err2 != nil { // idempotent
		t.Fatal(err1, err2)
	}
	var st, why string
	_ = d.Pool.QueryRow(ctx, `SELECT status, coalesce(error,'') FROM jobs WHERE id=$1`, oldJob).Scan(&st, &why)
	if st != "canceled" || !strings.Contains(why, "retired") {
		t.Fatalf("old per-project job not retired: %s %q", st, why)
	}
	if _, err := a.runServiceJob(ctx, &jobs.Job{TenantID: tn.ID, TargetID: old[p1], Attempts: 1, MaxAttempts: 5}, func(string) {}); !jobs.IsPermanent(err) {
		t.Fatalf("a running old job must fail permanently (no retry loop): %v", err)
	}
	vms, _ := a.svcs.VMInstances(ctx, tn.ID)
	if len(vms) != 1 || len(vms[0].Bindings) != 2 {
		t.Fatalf("want one shared instance with both devspaces, got %+v", vms)
	}
	vm := vms[0]
	var njobs int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE target_id=$1 AND status='queued'`, vm.ID).Scan(&njobs)
	if njobs != 1 {
		t.Fatalf("migration queued %d provision jobs", njobs)
	}
	if ins, _ := a.svcs.ByProject(ctx, tn.ID, p1); rrflowCheck(setupProject{ID: p1, Name: "a", Workspaces: 1}, ins, true).Detail == "" ||
		!strings.Contains(rrflowCheck(setupProject{ID: p1, Name: "a", Workspaces: 1}, ins, true).Detail, "moving") {
		t.Fatal("checklist does not show the move in progress")
	}
	res, err := a.runServiceJob(ctx, &jobs.Job{TenantID: tn.ID, TargetID: vm.ID, Attempts: 1, MaxAttempts: 1}, func(string) {})
	if err != nil {
		t.Fatal(err)
	}
	if res.(map[string]any)["migrated"] != 2 || res.(map[string]any)["bound"] != 2 {
		t.Fatalf("job result %v", res)
	}
	for _, p := range []string{p1, p2} {
		var s string
		_ = d.Pool.QueryRow(ctx, `SELECT state FROM service_instances WHERE id=$1`, old[p]).Scan(&s)
		if s != "migrated" {
			t.Fatalf("old instance %s is %q, want migrated (kept until retire-into)", p, s)
		}
	}
	joined := strings.Join(calls, "\n")
	if strings.Contains(joined, "retire-into") || strings.Contains(joined, "DELETE") {
		t.Fatal("the provision run deleted or retired an old instance")
	}
	for _, p := range []string{p1, p2} {
		if !strings.Contains(joined, "/projects/"+p+"/services/"+old[p]+"/migrate-to/"+vm.ID) {
			t.Fatalf("state of project %s not migrated:\n%s", p, joined)
		}
		if !strings.Contains(joined, "/projects/"+p+"/services/"+vm.ID+"/bindings/") || !strings.Contains(joined, "scope=vm") {
			t.Fatalf("devspace of %s not bound in its namespace:\n%s", p, joined)
		}
		ins, _ := a.svcs.ByProject(ctx, tn.ID, p)
		if len(ins) != 1 || ins[0].ID != vm.ID || len(ins[0].Bindings) != 1 || ins[0].Bindings[0].WorkspaceID != ws[p].ID {
			t.Fatalf("project %s should see only the shared instance and its own binding: %+v", p, ins)
		}
		if ch := rrflowCheck(setupProject{ID: p, Name: "x", Workspaces: 1}, ins, true); ch.State != "ok" {
			t.Fatalf("checklist %s: %+v", p, ch)
		}
	}
	if strings.Index(joined, "migrate-to") > strings.Index(joined, "/bindings/") {
		t.Fatal("bound before the old state was moved")
	}
	// A third project on the same VM gets a namespace in the same instance, not a new one.
	p3 := newUUID()
	exec(`INSERT INTO projects(id,tenant_id,boundary_id,name,created_by) VALUES($1,$2,$3,$4,'u')`, p3, tn.ID, b, "p"+p3[len(p3)-6:]+"c")
	got, err := a.svcs.EnsureVM(ctx, tn.ID, p3, managed.KindRRFlow, "u")
	if err != nil || got.ID != vm.ID || !a.svcs.HasNamespace(ctx, vm.ID, p3) {
		t.Fatalf("third project: %v %v", got, err)
	}
	if ch := vmRRFlowCheck(mustVMs(t, a, tn.ID)); ch.State != "ok" {
		t.Fatalf("vm check %+v", ch)
	}
	// A project without a tenant VM cannot get rrflow.
	p4 := newUUID()
	exec(`INSERT INTO projects(id,tenant_id,name,created_by) VALUES($1,$2,$3,'u')`, p4, tn.ID, "p"+p4[len(p4)-6:]+"d")
	if _, err := a.svcs.EnsureVM(ctx, tn.ID, p4, managed.KindRRFlow, "u"); err != managed.ErrNoBoundary {
		t.Fatalf("no boundary: %v", err)
	}
}

func mustVMs(t *testing.T, a *API, tenant string) []managed.Instance {
	t.Helper()
	v, err := a.svcs.VMInstances(context.Background(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	return v
}
