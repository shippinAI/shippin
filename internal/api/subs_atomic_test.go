package api

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/subs"
)

func TestSubscriptionReportAtomicAndRevocationSticky(t *testing.T) {
	a, _, tn, _ := subsAPI(t)
	ctx := context.Background()
	subject := "atomic-owner-" + uuid.NewString()
	acct, _ := a.db.UpsertAccount(ctx, subject, "")
	h, _ := a.subs.EnsureHome(ctx, tn.ID, acct, "", a.terminalHost(tn.ID, acct))
	for _, p := range []string{"claude", "codex"} {
		if err := a.subs.SetStatus(ctx, h, p, subs.NotConnected, "", true); err != nil {
			t.Fatal(err)
		}
	}
	if err := a.subs.SetReport(ctx, h, map[string]string{"claude": subs.Connected, "codex": "invalid"}); err == nil {
		t.Fatal("bad second row accepted")
	}
	cs, _ := a.subs.Connections(ctx, acct)
	for _, c := range cs {
		if c.Status != subs.NotConnected {
			t.Fatal("partial report committed")
		}
	}
	if err := a.subs.SetStatus(ctx, h, "codex", subs.Disconnected, "", true); err != nil {
		t.Fatal(err)
	}
	if err := a.subs.SetReport(ctx, h, map[string]string{"codex": subs.Connected}); err != nil {
		t.Fatal(err)
	}
	cs, _ = a.subs.Connections(ctx, acct)
	for _, c := range cs {
		if c.ProviderID == "codex" && c.Status != subs.Disconnected {
			t.Fatal("late report undid revocation")
		}
	}
}

func TestSubscriptionHostsBindTenantAndAccount(t *testing.T) {
	a, d, tn, _ := subsAPI(t)
	ctx := context.Background()
	subject := "multi-owner-" + uuid.NewString()
	other := tn
	other.ID = uuid.NewString()
	other.IdpOrgID = uuid.NewString()
	if _, err := d.Pool.Exec(ctx, `INSERT INTO tenants(id,idp_org_id,name,plan) VALUES($1,$2,'other','founder')`, other.ID, other.IdpOrgID); err != nil {
		t.Fatal(err)
	}
	acct, _ := d.UpsertAccount(ctx, subject, "")
	h1, err := a.subs.EnsureHome(ctx, tn.ID, acct, "", a.terminalHost(tn.ID, acct))
	if err != nil {
		t.Fatal(err)
	}
	h2, err := a.subs.EnsureHome(ctx, other.ID, acct, "", a.terminalHost(other.ID, acct))
	if err != nil {
		t.Fatal(err)
	}
	if h1.UtilityHost == h2.UtilityHost {
		t.Fatal("tenant host collision")
	}
	c := &caller{id: &auth.Identity{Sub: subject}, tenants: []tenantView{{Tenant: tn, Roles: []string{"org_member"}}}}
	if !a.utilityOwner(ctx, c, h1.UtilityHost) || a.utilityOwner(ctx, c, h2.UtilityHost) {
		t.Fatal("terminal owner authorization crossed tenant")
	}
	c.tenants = []tenantView{{Tenant: other, Roles: []string{"org_member"}}}
	if a.utilityOwner(ctx, c, h1.UtilityHost) || !a.utilityOwner(ctx, c, h2.UtilityHost) {
		t.Fatal("membership revocation failed")
	}
}
