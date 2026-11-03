package db

import (
	"slices"
	"testing"
)

var granular = []string{"catalog.plans.read", "catalog.prices.read", "catalog.plans.write", "config.defaults.write",
	"customers.read", "customers.contacts.read", "customers.notes.read", "customers.write",
	"customers.services.write", "config.effective.read", "provision.intent.create"}

// Migration 012 on a database that already has the presets (as on Gold): only staff.owner
// gains the granular permissions; every other row is unchanged.
func TestMigration012OnlyOwnerGains(t *testing.T) {
	d, ctx := testDB(t)
	for _, r := range StaffPresets {
		p := slices.DeleteFunc(slices.Clone(r.Permissions), func(x string) bool { return slices.Contains(granular, x) })
		if _, err := d.Pool.Exec(ctx, `INSERT INTO staff_roles(key,name,description,permissions,preset) VALUES($1,$2,'',$3,true)
			ON CONFLICT (key) DO UPDATE SET permissions=$3`, r.Key, r.Name, p); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO staff_roles(key,name,description,permissions) VALUES('staff.custom','C','','{billing.read}')`); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `DELETE FROM schema_migrations WHERE version='migrations/012_catalog_access.sql'`); err != nil {
		t.Fatal(err)
	}
	if err := d.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	rs, err := d.StaffRoles(ctx)
	if err != nil {
		t.Fatal(err)
	}
	for _, r := range rs {
		for _, g := range granular {
			if slices.Contains(r.Permissions, g) != (r.Key == "staff.owner") {
				t.Errorf("%s holds %s = %v", r.Key, g, slices.Contains(r.Permissions, g))
			}
		}
	}
}

func TestPermGrantsAudited(t *testing.T) {
	d, ctx := testDB(t)
	e := AuditEntry{Actor: "own", ActorName: "Own", Action: "team.perm_grant", Detail: "catalog.plans.read -> u2"}
	if err := d.GrantPerm(ctx, PermGrant{Subject: "u2", Permission: "catalog.plans.read", Reason: "review", GrantedBy: "own"}, e); err != nil {
		t.Fatal(err)
	}
	if err := d.GrantPerm(ctx, PermGrant{Subject: "u2", Permission: "catalog.plans.read", GrantedBy: "own"}, e); err != ErrNotFound {
		t.Fatalf("duplicate grant: %v", err)
	}
	gs, _ := d.PermGrantsOf(ctx, "u2")
	if len(gs) != 1 || gs[0].Reason != "review" {
		t.Fatalf("grants: %+v", gs)
	}
	e.Action = "team.perm_revoke"
	if err := d.RevokePerm(ctx, "u2", "catalog.plans.read", e); err != nil {
		t.Fatal(err)
	}
	if err := d.RevokePerm(ctx, "u2", "catalog.plans.read", e); err != ErrNotFound {
		t.Fatalf("second revoke: %v", err)
	}
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log WHERE action IN ('team.perm_grant','team.perm_revoke')`).Scan(&n)
	if n != 2 {
		t.Fatalf("audit rows: %d", n)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO staff_perm_grants(subject,permission,granted_by) VALUES('u3','Bad Key','x')`); err == nil {
		t.Fatal("permission keys are checked by the table")
	}
}
