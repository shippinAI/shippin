package api

import (
	"context"
	"os"
	"testing"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/managed"
)

// TestRRFlowLegacyUntilMoved (audit pa2 decision 4): 026 is schema only. A tenant's pre-D36
// per-project instance keeps serving (chat sees its binding) and gets no shared instance until
// MigrateTenant runs for that tenant; moving one tenant leaves every other tenant's rows alone.
func TestRRFlowLegacyUntilMoved(t *testing.T) {
	a, d, tn := notifyAPI(t)
	ctx := context.Background()
	a.svcs = &managed.Store{Pool: d.Pool}
	a.cfg.Services.RRFlow = config.ServiceToggle{Enabled: true, Tenants: []string{tn.ID}}
	exec := func(q string, args ...any) {
		t.Helper()
		if _, err := d.Pool.Exec(ctx, q, args...); err != nil {
			t.Fatal(err)
		}
	}
	other := newUUID()
	exec(`INSERT INTO tenants(id,idp_org_id,name,plan,status) VALUES($1,$2,'Other Co','founder','active')`, other, "org-"+other[:8])
	inst := map[string]string{}
	var w1 string
	for _, tid := range []string{tn.ID, other} {
		b, p, si := newUUID(), newUUID(), newUUID()
		exec(`INSERT INTO boundaries(id,tenant_id,state,cpu,memory_mib,root_gib,data_gib) VALUES($1,$2,'ready',2,2048,20,20)`, b, tid)
		exec(`INSERT INTO projects(id,tenant_id,boundary_id,name,created_by) VALUES($1,$2,$3,$4,'u')`, p, tid, b, "p"+p[:6])
		exec(`INSERT INTO service_instances(id,tenant_id,project_id,boundary_id,kind,state,created_by) VALUES($1,$2,$3,$4,'rrflow-state','ready','u')`, si, tid, p, b)
		inst[tid] = si
		if tid == tn.ID {
			w := provisioningWS(t, a, d, tn)
			exec(`UPDATE workspaces SET project_id=$1 WHERE id=$2`, p, w.ID)
			exec(`INSERT INTO service_bindings(id,tenant_id,instance_id,workspace_id,state) VALUES($1,$2,$3,$4,'bound')`, newUUID(), tid, si, w.ID)
			w.ProjectID, w1 = p, w.ID
			if !a.chatRRFlowBound(ctx, tid, w) {
				t.Fatal("the pre-D36 binding must keep serving chat")
			}
			if err := a.attachRRFlow(ctx, tid, p, w.ID, "u"); err != errRRFlowLegacy {
				t.Fatalf("no shared instance before the move: %v", err)
			}
		}
	}
	sql, err := os.ReadFile("../db/migrations/026_rrflow_per_vm.sql")
	if err != nil {
		t.Fatal(err)
	}
	exec(string(sql))
	ids, err := a.svcs.MigrateTenant(ctx, tn.ID, "u")
	if err != nil || len(ids) != 1 {
		t.Fatalf("move queued %v: %v", ids, err)
	}
	if again, _ := a.svcs.MigrateTenant(ctx, tn.ID, "u"); len(again) != 0 {
		t.Fatalf("second move queued again: %v", again)
	}
	var st, ost string
	_ = d.Pool.QueryRow(ctx, `SELECT state FROM service_instances WHERE id=$1`, inst[tn.ID]).Scan(&st)
	_ = d.Pool.QueryRow(ctx, `SELECT state||'|'||coalesce(migrate_to::text,'') FROM service_instances WHERE id=$1`, inst[other]).Scan(&ost)
	if st != "migrating" || ost != "ready|" {
		t.Fatalf("moved %q, other tenant %q (want migrating, ready|)", st, ost)
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM service_instances WHERE tenant_id=$1 AND scope='vm'`, other).Scan(&n)
	if n != 0 || w1 == "" {
		t.Fatalf("other tenant got %d shared instances", n)
	}
	if legacy, _ := a.svcs.LegacyPending(ctx, other); !legacy {
		t.Fatal("other tenant must still be on its per-project instance")
	}
}
