package db

import "testing"

func TestTheSystemTenantExistsOnceAndCustomerPathsNeverReturnIt(t *testing.T) {
	d, ctx := testDB(t)
	sys, err := d.SystemTenant(ctx)
	if err != nil || sys.Name == "" || sys.Status != "active" {
		t.Fatalf("%+v %v", sys, err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO tenants(id, idp_org_id, name, plan, status, kind)
		VALUES (gen_random_uuid(), 'system:other', 'Other', 'system', 'active', 'system')`); err == nil {
		t.Fatal("a second system tenant must be refused")
	}
	if ts, err := d.TenantsByOrgs(ctx, nil); err != nil || len(ts) != 0 {
		t.Fatalf("the list of all tenants must leave it out: %+v %v", ts, err)
	}
	if ts, err := d.TenantsByOrgs(ctx, []string{sys.IdpOrgID}); err != nil || len(ts) != 0 {
		t.Fatalf("its org id must not make anyone a member: %+v %v", ts, err)
	}
	if ts, err := d.TenantsByIDs(ctx, []string{sys.ID}); err != nil || len(ts) != 0 {
		t.Fatalf("a grant must not reach it: %+v %v", ts, err)
	}
	if got, err := d.TenantByID(ctx, sys.ID); err == nil && got != nil {
		t.Fatalf("no lifecycle route may load it: %+v", got)
	}
	if err := d.SetTenantStatus(ctx, sys.ID, "suspended", "", ""); err != nil {
		t.Fatal(err)
	}
	if after, err := d.SystemTenant(ctx); err != nil || after.Status != "active" {
		t.Fatalf("it must not be possible to suspend it: %+v %v", after, err)
	}
}
