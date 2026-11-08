package db

import "testing"

func TestOrganizationWorkspaceUpgradePreservesCustomerOwnership(t *testing.T) {
	d, ctx := testDB(t)
	if _, err := d.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	if err := d.migrateFS(ctx, migrations, "migrations/*.sql", "065_shippin_organization_ownership.sql"); err != nil {
		t.Fatal(err)
	}
	var user, customer, project, environment string
	if err := d.Pool.QueryRow(ctx, `INSERT INTO identity_users(id,email,name,role,email_verified) VALUES(gen_random_uuid(),'owner@example.invalid','Owner','owner',true) RETURNING id::text`).Scan(&user); err != nil {
		t.Fatal(err)
	}
	if err := d.Pool.QueryRow(ctx, `INSERT INTO identity_orgs(id,name) VALUES(gen_random_uuid(),'Existing customer') RETURNING id::text`).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan) VALUES($1,$2,'Existing customer','starter')`, customer, customer); err != nil {
		t.Fatal(err)
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO identity_members(org_id,user_id,role) VALUES($1,$2,'owner')`, customer, user); err != nil {
		t.Fatal(err)
	}
	if err := d.Pool.QueryRow(ctx, `INSERT INTO projects(id,tenant_id,name,created_by) VALUES(gen_random_uuid(),$1,'existing-project','before-upgrade') RETURNING id::text`, customer).Scan(&project); err != nil {
		t.Fatal(err)
	}
	if err := d.Pool.QueryRow(ctx, `INSERT INTO environments(id,tenant_id,project_id,name) VALUES(gen_random_uuid(),$1,$2,'dev') RETURNING id::text`, customer, project).Scan(&environment); err != nil {
		t.Fatal(err)
	}
	if err := d.migrate(ctx); err != nil {
		t.Fatal(err)
	}
	var preserved, internal, grouped bool
	if err := d.Pool.QueryRow(ctx, `SELECT
 EXISTS(SELECT 1 FROM identity_members m JOIN tenants t ON t.idp_org_id=m.org_id::text WHERE m.user_id=$1 AND t.id=$2 AND t.kind='customer' AND m.role='owner'),
 EXISTS(SELECT 1 FROM orgs o JOIN tenants t ON t.id=o.tenant_id JOIN identity_members m ON m.org_id=o.identity_org_id WHERE o.type='operations' AND t.kind='system' AND m.user_id=$1 AND o.identity_org_id<>$2),
 EXISTS(SELECT 1 FROM projects p JOIN shippin_workspaces w ON w.id=p.workspace_id JOIN environments e ON e.project_id=p.id WHERE p.id=$3 AND e.id=$4 AND p.tenant_id=$2 AND w.tenant_id=p.tenant_id AND w.is_default)`, user, customer, project, environment).Scan(&preserved, &internal, &grouped); err != nil {
		t.Fatal(err)
	}
	if !preserved || !internal || !grouped {
		t.Fatalf("customer preserved=%v internal separated=%v project/environment preserved=%v", preserved, internal, grouped)
	}
	if err := d.migrate(ctx); err != nil {
		t.Fatal("repeat startup", err)
	}
}
