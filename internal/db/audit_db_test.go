package db

import "testing"

func TestAuditAndGrantsAreAppendOnly(t *testing.T) {
	d, ctx := testDB(t)
	tn, _ := seed(t, d, ctx)
	g := reqGrant(tn, "", "admin1", "pending", 60)
	if err := d.CreateGrant(ctx, g, AuditEntry{TenantID: tn.ID, GrantID: g.ID, Actor: "a", Action: "access.request"}); err != nil {
		t.Fatal(err)
	}
	for _, q := range []string{`UPDATE audit_log SET detail='x'`, `DELETE FROM audit_log`, `TRUNCATE audit_log`,
		`DELETE FROM access_grants`, `TRUNCATE access_grants CASCADE`} {
		if _, err := d.Pool.Exec(ctx, q); err == nil {
			t.Errorf("%s: succeeded, want append-only error", q)
		}
	}
	if es, _ := d.AllAudit(ctx, "", 10); len(es) != 1 {
		t.Fatal("entry must survive")
	}
}

func TestDeletedTenantGrantsInactive(t *testing.T) {
	d, ctx := testDB(t)
	tn, _ := seed(t, d, ctx)
	if err := d.CreateGrant(ctx, reqGrant(tn, "", "admin1", "approved", 3600)); err != nil {
		t.Fatal(err)
	}
	if err := d.MarkTenantDeleted(ctx, tn.ID); err != nil {
		t.Fatal(err)
	}
	if gs, _ := d.ActiveGrants(ctx, "admin1"); len(gs) != 0 {
		t.Fatal("grants on a deleted tenant must not be active")
	}
}
