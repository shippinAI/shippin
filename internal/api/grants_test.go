package api

import (
	"testing"
	"time"

	"shippin.cloud/console/internal/db"
)

func grant(kind, ws string, exp time.Duration) db.Grant {
	e := time.Now().Add(exp)
	return db.Grant{ID: kind + ws, Kind: kind, WorkspaceID: ws, Status: "approved", ExpiresAt: &e}
}

func mgr() *caller { return &caller{op: "op_superadmin", managers: []string{"org_owner", "org_admin"}} }

func TestOperatorRoleAloneManagesNothing(t *testing.T) {
	// Even a member-less view with the operator's own role string must not manage.
	if mgr().canManage(&tenantView{Roles: []string{"op_superadmin"}}) {
		t.Fatal("operator role must not grant management")
	}
}

func TestMembersByConfiguredRoles(t *testing.T) {
	c := mgr()
	for role, want := range map[string]bool{"org_owner": true, "org_admin": true, "org_member": false, "org_owner_viewer": false} {
		tv := &tenantView{Roles: []string{role}}
		if c.canManage(tv) != want || c.canDecide(tv) != want {
			t.Errorf("%s: canManage/canDecide want %v", role, want)
		}
	}
}

func TestTenantWideGrant(t *testing.T) {
	c, tv := mgr(), &tenantView{grants: []db.Grant{grant("request", "", time.Hour)}}
	if !c.canManage(tv) || !c.canManageWS(tv, "w1") || !tv.sees("w9") {
		t.Fatal("tenant-wide grant must cover every workspace")
	}
	if c.canDecide(tv) {
		t.Fatal("a grant holder must never approve or revoke access")
	}
}

func TestWorkspaceScopedGrant(t *testing.T) {
	c, tv := mgr(), &tenantView{grants: []db.Grant{grant("request", "w1", time.Hour)}}
	if c.canManage(tv) {
		t.Fatal("workspace grant must not give tenant-level management")
	}
	if !c.canManageWS(tv, "w1") || c.canManageWS(tv, "w2") || !tv.sees("w1") || tv.sees("w2") {
		t.Fatal("workspace grant must cover exactly its workspace")
	}
}

func TestExpiredOrClosedGrantGivesNothing(t *testing.T) {
	c := mgr()
	expired := &tenantView{grants: []db.Grant{grant("request", "", -time.Second)}}
	revoked := grant("request", "", time.Hour)
	revoked.Status = "revoked"
	pending := grant("request", "", time.Hour)
	pending.Status = "pending"
	for name, tv := range map[string]*tenantView{"expired": expired, "revoked": {grants: []db.Grant{revoked}},
		"pending": {grants: []db.Grant{pending}}} {
		if c.canManage(tv) || c.canManageWS(tv, "w1") || tv.sees("w1") {
			t.Errorf("%s grant must give no access", name)
		}
	}
	if s := expired.grants[0].Effective(time.Now()); s != "expired" {
		t.Fatalf("effective status %q, want expired", s)
	}
}

func TestBreakGlassPreferredForAudit(t *testing.T) {
	tv := &tenantView{grants: []db.Grant{grant("request", "", time.Hour), grant("breakglass", "", time.Hour)}}
	if g := tv.grantFor("w1"); g == nil || g.Kind != "breakglass" {
		t.Fatal("overlapping break-glass grant must be the one audited")
	}
	if kindPrefix(tv.grantFor("w1")) != "breakglass." || kindPrefix(&tv.grants[0]) != "access." {
		t.Fatal("break-glass entries must be distinct")
	}
}

func TestExpiryEntry(t *testing.T) {
	if e := expiryEntry(db.Grant{Kind: "breakglass"}); e.Action != "breakglass.end" || !e.BreakGlass || e.Actor != "system" {
		t.Fatalf("break-glass expiry entry: %+v", e)
	}
	if e := expiryEntry(db.Grant{Kind: "request"}); e.Action != "access.expire" || e.BreakGlass {
		t.Fatalf("request expiry entry: %+v", e)
	}
}
