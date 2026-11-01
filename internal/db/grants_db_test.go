package db

import (
	"testing"
	"time"
)

func TestGrantLifecycle(t *testing.T) {
	d, ctx := testDB(t)
	tn, _ := seed(t, d, ctx)
	g := reqGrant(tn, "", "admin1", "pending", 3600)
	if err := d.CreateGrant(ctx, g, AuditEntry{TenantID: tn.ID, GrantID: g.ID, Actor: "admin1", Action: "access.request"}); err != nil {
		t.Fatal(err)
	}
	if open, err := d.OpenGrant(ctx, tn.ID, "", "admin1"); err != nil || !open {
		t.Fatalf("pending request must count as open: %v %v", open, err)
	}
	if gs, _ := d.ActiveGrants(ctx, "admin1"); len(gs) != 0 {
		t.Fatal("pending grant must not be active")
	}
	if won, err := d.TransitionGrant(ctx, g.ID, "revoke", "o", "O"); err != nil || won {
		t.Fatalf("revoke of a pending grant must not apply: %v %v", won, err)
	}
	if won, err := d.TransitionGrant(ctx, g.ID, "approve", "owner", "Owner", AuditEntry{TenantID: tn.ID, GrantID: g.ID, Actor: "owner", Action: "access.approve"}); err != nil || !won {
		t.Fatalf("approve: %v %v", won, err)
	}
	if won, _ := d.TransitionGrant(ctx, g.ID, "approve", "owner", "Owner"); won {
		t.Fatal("double approve must not apply")
	}
	gs, err := d.ActiveGrants(ctx, "admin1")
	if err != nil || len(gs) != 1 || gs[0].ExpiresAt == nil || time.Until(*gs[0].ExpiresAt) < 59*time.Minute || gs[0].DecidedBy != "owner" {
		t.Fatalf("approved grant: %+v %v", gs, err)
	}
	if gs, _ := d.ActiveGrants(ctx, "admin2"); len(gs) != 0 {
		t.Fatal("a grant belongs to its requester only")
	}
	if won, _ := d.TransitionGrant(ctx, g.ID, "revoke", "owner", "Owner"); !won {
		t.Fatal("revoke of an active grant must apply")
	}
	if gs, _ := d.ActiveGrants(ctx, "admin1"); len(gs) != 0 {
		t.Fatal("revoked grant must not be active")
	}
	if _, err := d.TransitionGrant(ctx, g.ID, "drop table", "x", "x"); err != ErrUnknownTransition {
		t.Fatal("unknown transitions must be refused")
	}
	if es, _ := d.TenantAudit(ctx, tn.ID, 10); len(es) != 2 {
		t.Fatalf("want 2 audit entries, got %d", len(es))
	}
}

func TestBreakGlassExpiryAndAudit(t *testing.T) {
	d, ctx := testDB(t)
	tn, ws := seed(t, d, ctx)
	g := reqGrant(tn, ws.ID, "admin1", "approved", 1)
	g.Kind, g.Category = "breakglass", "security_incident"
	if err := d.CreateGrant(ctx, g); err != nil {
		t.Fatal(err)
	}
	if gs, _ := d.ActiveGrants(ctx, "admin1"); len(gs) != 1 || gs[0].WorkspaceName != "w" {
		t.Fatalf("break-glass must be active at once, scoped to its workspace: %+v", gs)
	}
	time.Sleep(1200 * time.Millisecond)
	if gs, _ := d.ActiveGrants(ctx, "admin1"); len(gs) != 0 {
		t.Fatal("expired grant must not be active even before the sweep")
	}
	got, err := d.ExpireGrants(ctx, func(g Grant) AuditEntry {
		return AuditEntry{TenantID: g.TenantID, GrantID: g.ID, Actor: "system", Action: "breakglass.end", BreakGlass: true}
	})
	if err != nil || len(got) != 1 || got[0].Status != "expired" {
		t.Fatalf("sweep: %+v %v", got, err)
	}
	if again, _ := d.ExpireGrants(ctx, func(Grant) AuditEntry { return AuditEntry{} }); len(again) != 0 {
		t.Fatal("a grant expires once")
	}
	if es, _ := d.AllAudit(ctx, tn.ID, 10); len(es) != 1 || !es[0].BreakGlass || es[0].Action != "breakglass.end" {
		t.Fatalf("expiry audit: %+v", es)
	}
}

func TestDelayedNoticeHiddenFromCustomer(t *testing.T) {
	d, ctx := testDB(t)
	tn, _ := seed(t, d, ctx)
	later := time.Now().Add(720 * time.Hour)
	g := reqGrant(tn, "", "admin1", "approved", 3600)
	g.Kind, g.Category, g.NoticeAt = "breakglass", "legal_hold", &later
	e := AuditEntry{TenantID: tn.ID, GrantID: g.ID, Actor: "admin1", Action: "breakglass.notice_withheld", BreakGlass: true}
	if err := d.CreateGrant(ctx, g, e); err != nil {
		t.Fatal(err)
	}
	if gs, _ := d.TenantGrants(ctx, tn.ID, true, 10); len(gs) != 0 {
		t.Fatal("customer must not see a delayed-notice grant before its notice time")
	}
	if es, _ := d.TenantAudit(ctx, tn.ID, 10); len(es) != 0 {
		t.Fatal("customer must not see its audit entries yet")
	}
	if gs, _ := d.TenantGrants(ctx, tn.ID, false, 10); len(gs) != 1 {
		t.Fatal("operators see it")
	}
	if es, _ := d.AllAudit(ctx, "", 10); len(es) != 1 {
		t.Fatal("operators see the withheld-notice entry")
	}
}
