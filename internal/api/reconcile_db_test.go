package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/projects"
)

func reconcileAPI(t *testing.T) (*API, *db.DB, db.Tenant) {
	a, d, tn := notifyAPI(t)
	a.jobs = jobs.NewStore(d.Pool)
	a.cfg.Jobs = testJobsCfg()
	return a, d, tn
}

func testJobsCfg() (j config.Jobs) {
	j.Enabled, j.Workers, j.MaxAttempts, j.LeaseSeconds, j.HeartbeatSeconds = true, 1, 3, 60, 20
	j.PollSeconds, j.BackoffSeconds, j.BackoffMaxSecs = 1, 1, 1
	j.ReconcileSeconds, j.DriftSeconds, j.KeepDays, j.IdempotencyHours = 60, 600, 30, 24
	return j
}

func TestReconcileRequeuesOrFailsOrphans(t *testing.T) {
	a, d, tn := reconcileAPI(t)
	ctx := context.Background()
	fresh := provisioningWS(t, a, d, tn)
	orphan := provisioningWS(t, a, d, tn)
	deadEnd := provisioningWS(t, a, d, tn)
	old := time.Now().Add(-time.Hour)
	_, _ = d.Pool.Exec(ctx, `UPDATE workspaces SET updated_at=$1 WHERE id = ANY($2::uuid[])`, old, []string{orphan.ID, deadEnd.ID})
	j, _, _ := a.jobs.Enqueue(ctx, a.workspaceJob(*deadEnd, "u"))
	_, _ = d.Pool.Exec(ctx, `UPDATE jobs SET status='failed', error='disk full', finished_at=now() WHERE id=$1`, j.ID)
	if !a.reconcileOnce(ctx, false) {
		t.Fatal("reconciler did not take the leader lock")
	}
	if _, err := a.jobs.Live(ctx, jobs.KindWorkspaceProvision, "workspace", orphan.ID); err != nil {
		t.Fatalf("orphan not requeued: %v", err)
	}
	if _, err := a.jobs.Live(ctx, jobs.KindWorkspaceProvision, "workspace", fresh.ID); err != jobs.ErrNotFound {
		t.Fatalf("a just-created workspace must be left alone: %v", err)
	}
	if w, _ := d.WorkspaceByID(ctx, deadEnd.ID); w.State != "failed" || w.Error != "provisioning failed: disk full" {
		t.Fatalf("dead end: %s %q", w.State, w.Error)
	}
	if !a.reconcileOnce(ctx, false) {
		t.Fatal("second pass")
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM jobs WHERE target_id=$1`, orphan.ID).Scan(&n)
	if n != 1 {
		t.Fatalf("second pass duplicated the job: %d", n)
	}
}

func TestReconcileQueuesStuckTenant(t *testing.T) {
	a, d, tn := reconcileAPI(t)
	ctx := context.Background()
	_, _ = d.Pool.Exec(ctx, `UPDATE tenants SET status='provisioning', updated_at=now()-interval '1 hour' WHERE id=$1`, tn.ID)
	a.reconcileOnce(ctx, false)
	if _, err := a.jobs.Live(ctx, jobs.KindTenantProvision, "tenant", tn.ID); err != nil {
		t.Fatalf("stuck tenant not queued: %v", err)
	}
}

func TestBoundaryJobBindsVM(t *testing.T) {
	a, d, tn := reconcileAPI(t)
	ctx := context.Background()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(hostclient.TenantVM{Mode: "vm", Name: "tvm-" + tn.ID[:8], CPU: "8",
			MemLimit: "16GiB", RootSize: "50GiB", DataSize: "200GiB"})
	}))
	defer srv.Close()
	a.host = hostclient.New(srv.URL)
	_ = d.SetTenantStatus(ctx, tn.ID, "active", "203.0.113.9", "")
	st := &projects.Store{Pool: d.Pool}
	b, _ := st.PlanBoundary(ctx, tn.ID, 8, 16384, 50, 200)
	j := &jobs.Job{ID: "x", TenantID: tn.ID, TargetID: b, Attempts: 1, MaxAttempts: 3}
	if _, err := a.runBoundaryJob(ctx, j, func(string) {}); err != nil {
		t.Fatal(err)
	}
	if state, vm, _ := st.BoundaryState(ctx, tn.ID, b); state != "ready" || vm != "tvm-"+tn.ID[:8] {
		t.Fatalf("boundary %s %s", state, vm)
	}
	b2, _ := st.PlanBoundary(ctx, tn.ID, 8, 16384, 50, 200)
	_, err := a.runBoundaryJob(ctx, &jobs.Job{TenantID: tn.ID, TargetID: b2, Attempts: 1, MaxAttempts: 3}, func(string) {})
	if !jobs.IsPermanent(err) {
		t.Fatalf("second boundary on one VM must fail permanently: %v", err)
	}
	if state, _, _ := st.BoundaryState(ctx, tn.ID, b2); state != "error" {
		t.Fatalf("second boundary state %s", state)
	}
}

func TestDriftDetail(t *testing.T) {
	ws := []db.Workspace{{ID: "a", State: "running"}, {ID: "b", State: "provisioning"}, {ID: "c", State: "stopped"}}
	inv := []hostclient.Instance{{Config: map[string]string{"user.shippin.workspace": "a"}},
		{Config: map[string]string{"user.shippin.workspace": "z"}}, {Name: "tvm-x"}}
	if got := driftDetail(ws, inv); got != "missing instances: w-c; instances without a workspace: w-z" {
		t.Fatalf("got %q", got)
	}
	var m driftMemo
	if !m.changed("t", "x") || m.changed("t", "x") || m.changed("t", "") || !m.changed("t", "x") {
		t.Fatal("memo should report changes only")
	}
}
