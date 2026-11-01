package api

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

func newUUID() string { u, _ := uuid.NewV7(); return u.String() }

// The owner of a tenant's org becomes a tenant_owners row; a plain member and a staff grant
// holder do not; the upsert is throttled per user.
func TestSyncAccountOwners(t *testing.T) {
	a, d, tn := notifyAPI(t)
	ctx := context.Background()
	tn2 := db.Tenant{ID: newUUID(), IdpOrgID: newUUID(), Name: "Other", Plan: "p"}
	if err := d.CreateTenant(ctx, tn2); err != nil {
		t.Fatal(err)
	}
	c := &caller{id: &auth.Identity{Sub: "owner-sub-" + tn.ID, Email: "o@acme.test"},
		tenants: []tenantView{{Tenant: tn, Roles: []string{"org_owner"}}, {Tenant: tn2, Roles: []string{"org_member"}}}}
	a.syncAccount(ctx, c)
	own, err := d.TenantOwners(ctx, tn.ID)
	if err != nil || len(own) != 1 {
		t.Fatalf("owner not recorded: %v %v", own, err)
	}
	if own2, _ := d.TenantOwners(ctx, tn2.ID); len(own2) != 0 {
		t.Fatalf("member recorded as owner: %v", own2)
	}
	// Throttled: a second call in the window does not touch the DB (owner row stays single).
	c.tenants[1].Roles = []string{"org_owner"}
	a.syncAccount(ctx, c)
	if own2, _ := d.TenantOwners(ctx, tn2.ID); len(own2) != 0 {
		t.Fatal("throttle ignored")
	}
}
