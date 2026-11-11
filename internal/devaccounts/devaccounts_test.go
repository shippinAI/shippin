package devaccounts

import (
	"context"
	"errors"
	"os"
	"testing"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/identity"
	"shippin.cloud/console/internal/orgs"
)

func TestParseAcceptsOnlyEmailAndAKnownKind(t *testing.T) {
	got, err := Parse(" Admin@Example.test=staff , user@example.test=customer:business:managed ,")
	if err != nil || len(got) != 2 || got[0] != (Account{Email: "admin@example.test", Kind: Staff}) || got[1] != (Account{"user@example.test", Customer, "business", "managed"}) {
		t.Fatalf("%+v %v", got, err)
	}
	for _, raw := range []string{"", "admin@example.test", "admin@example.test=owner", "not-an-email=staff", "a@example.test=staff,a@example.test=customer", "a@example.test=staff:pro", "a@example.test=customer::managed"} {
		if _, err := Parse(raw); err == nil {
			t.Errorf("%q was accepted", raw)
		}
	}
}

func TestOnlyListedStaffAccountsAreExempt(t *testing.T) {
	t.Setenv("SHIPPIN_DEV_ACCOUNTS", "admin@example.test=staff,user@example.test=customer")
	if !ListedStaff(" Admin@Example.test ") || ListedStaff("user@example.test") || ListedStaff("someone@example.test") {
		t.Fatal("only listed staff accounts are exempt")
	}
	t.Setenv("SHIPPIN_DEV_ACCOUNTS", "")
	if ListedStaff("admin@example.test") {
		t.Fatal("nothing is exempt when no accounts are listed")
	}
}

// TestAccountsOnARealDatabase runs when SHIPPIN_TEST_DATABASE_URL points at a disposable PostgreSQL.
func TestAccountsOnARealDatabase(t *testing.T) {
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL is not set")
	}
	ctx := context.Background()
	d, err := db.Open(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer d.Pool.Close()
	if _, err := d.Pool.Exec(ctx, `DELETE FROM identity_users WHERE email LIKE '%@devaccounts.test'`); err != nil {
		t.Fatal(err)
	}
	accounts := []Account{{Email: "staff@devaccounts.test", Kind: Staff}, {"user@devaccounts.test", Customer, "business", "managed"}}
	if err := Ensure(ctx, d.Pool, identity.HashPassword, accounts, "a-long-test-password", testGrants); err != nil && err.Error() != "staff@devaccounts.test: the installation has no owner yet; finish setup and restart" {
		t.Fatal(err)
	}
	tx, _ := d.Pool.Begin(ctx)
	if _, err := orgs.ConfigureInternal(ctx, tx, "Operations"); err != nil {
		t.Fatal(err)
	}
	if err := tx.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	// A real install fills the role catalog at startup; this database never started a panel.
	if _, err := d.Pool.Exec(ctx, `INSERT INTO staff_roles(key, name, description, permissions, preset) VALUES
	  ('staff.owner','Owner','',ARRAY['tenants.read'],true), ('staff.ops','Operations','',ARRAY['tenants.read'],true) ON CONFLICT DO NOTHING`); err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ {
		if err := Ensure(ctx, d.Pool, identity.HashPassword, accounts, "a-long-test-password", testGrants); err != nil {
			t.Fatalf("run %d: %v", i+1, err)
		}
	}
	var staffRoles, customerRoles, customerOrgs int
	var hasPassword bool
	_ = d.Pool.QueryRow(ctx, `SELECT coalesce(cardinality(r.roles),0), u.password_hash <> '' FROM identity_users u LEFT JOIN local_staff_roles r ON r.user_id=u.id WHERE u.email='staff@devaccounts.test'`).Scan(&staffRoles, &hasPassword)
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM local_staff_roles r JOIN identity_users u ON u.id=r.user_id WHERE u.email='user@devaccounts.test'`).Scan(&customerRoles)
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM identity_members m JOIN orgs o ON o.identity_org_id=m.org_id JOIN identity_users u ON u.id=m.user_id
	  WHERE o.type='customer' AND m.role='owner' AND u.email='user@devaccounts.test'`).Scan(&customerOrgs)
	var plan, coverage string
	_ = d.Pool.QueryRow(ctx, `SELECT t.plan, coalesce(t.coverage, '') FROM tenants t JOIN orgs o ON o.tenant_id=t.id JOIN identity_members m ON m.org_id=o.identity_org_id
	  JOIN identity_users u ON u.id=m.user_id WHERE u.email='user@devaccounts.test'`).Scan(&plan, &coverage)
	if plan != "business" || coverage != "managed" {
		t.Fatalf("the customer organization is on %q with %q coverage, want business with managed", plan, coverage)
	}
	if staffRoles == 0 || !hasPassword || customerRoles != 0 || customerOrgs != 1 {
		t.Fatalf("staff roles %d, password %v, customer staff rows %d, customer organizations %d", staffRoles, hasPassword, customerRoles, customerOrgs)
	}
	var granted, customerGrants int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM staff_perm_grants g JOIN identity_users u ON g.subject='local:'||u.id WHERE u.email='staff@devaccounts.test' AND g.granted_by='devaccounts'`).Scan(&granted)
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM staff_perm_grants g JOIN identity_users u ON g.subject='local:'||u.id WHERE u.email='user@devaccounts.test'`).Scan(&customerGrants)
	if granted != len(testGrants.Perms) || customerGrants != 0 {
		t.Fatalf("the staff account has %d direct grants, want %d; the customer has %d, want 0", granted, len(testGrants.Perms), customerGrants)
	}
	var hasOwner bool
	_ = d.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM local_staff_roles r JOIN identity_users u ON u.id=r.user_id WHERE u.email='staff@devaccounts.test' AND 'staff.owner' = ANY(r.roles))`).Scan(&hasOwner)
	if hasOwner {
		t.Fatal("a development account was given the Owner role")
	}
	if _, err := d.Pool.Exec(ctx, `INSERT INTO identity_users (id, email, email_verified, name, role) VALUES (gen_random_uuid(),'owner@devaccounts.test',true,'Owner','owner')`); err != nil {
		t.Fatal(err)
	}
	if err := Ensure(ctx, d.Pool, identity.HashPassword, []Account{{Email: "owner@devaccounts.test", Kind: Staff}}, "a-long-test-password", testGrants); !errors.Is(err, ErrOwner) {
		t.Fatalf("the owner's account must never be changed: %v", err)
	}
	if err := Ensure(ctx, d.Pool, identity.HashPassword, accounts, "short", testGrants); err == nil {
		t.Fatal("a short password was accepted")
	}
}

var testGrants = Grants{Perms: []string{"catalog.plans.read", "catalog.plans.write"}, Subject: func(id string) string { return "local:" + id }}
