package db

import "testing"

func TestAccountsAndOwners(t *testing.T) {
	d, ctx := testDB(t)
	tn, _ := seed(t, d, ctx)
	a1, err := d.UpsertAccount(ctx, "sub-1", "a@x.test")
	if err != nil {
		t.Fatal(err)
	}
	a2, err := d.UpsertAccount(ctx, "sub-1", "")
	if err != nil || a2 != a1 {
		t.Fatalf("upsert must return the same id: %s %s %v", a1, a2, err)
	}
	var email string
	_ = d.Pool.QueryRow(ctx, `SELECT email FROM accounts WHERE id=$1`, a1).Scan(&email)
	if email != "a@x.test" {
		t.Fatalf("empty email blanked the stored one: %q", email)
	}
	if _, err := d.UpsertAccount(ctx, "sub-1", "b@x.test"); err != nil {
		t.Fatal(err)
	}
	_ = d.Pool.QueryRow(ctx, `SELECT email FROM accounts WHERE id=$1`, a1).Scan(&email)
	if email != "b@x.test" {
		t.Fatalf("email not updated: %q", email)
	}
	for i := 0; i < 2; i++ {
		if err := d.AddTenantOwner(ctx, tn.ID, a1); err != nil {
			t.Fatal(err)
		}
	}
	own, err := d.TenantOwners(ctx, tn.ID)
	if err != nil || len(own) != 1 || own[0] != a1 {
		t.Fatalf("owners %v %v", own, err)
	}
}
