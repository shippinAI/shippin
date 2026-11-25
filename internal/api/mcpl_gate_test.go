package api

import (
	"net/http"
	"testing"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// The member gate, for every kind of caller a development space has: who may read one, who may
// manage one, and that a support grant for one space reaches that space and nothing else.
func TestMemberGate(t *testing.T) {
	soon, past := time.Now().Add(time.Hour), time.Now().Add(-time.Hour)
	grant := func(kind, ws string, until time.Time) db.Grant {
		return db.Grant{ID: "g-" + kind + ws, Kind: kind, Status: "approved", WorkspaceID: ws, ExpiresAt: &until}
	}
	who := &caller{id: &auth.Identity{Sub: "someone"}, managers: []string{"org_owner", "org_admin"}}
	callers := map[string]*tenantView{
		"owner":            {Roles: []string{"org_owner"}},
		"member":           {Roles: []string{"org_member"}},
		"grant for all":    {Roles: []string{}, grants: []db.Grant{grant("support", "", soon)}},
		"grant for A":      {Roles: []string{}, grants: []db.Grant{grant("support", "A", soon)}},
		"break-glass on A": {Roles: []string{}, grants: []db.Grant{grant("breakglass", "A", soon)}},
		"expired grant":    {Roles: []string{}, grants: []db.Grant{grant("support", "A", past)}},
		"pending grant":    {Roles: []string{}, grants: []db.Grant{{ID: "p", Kind: "support", Status: "pending", WorkspaceID: "A", ExpiresAt: &soon}}},
	}
	const ok, forbidden, missing = 0, http.StatusForbidden, http.StatusNotFound
	cases := []struct {
		caller, role string
		scoped       bool
		item         string
		want         int
	}{
		// Reading one space: a member reads any, a grant holder only what the grant covers.
		{"owner", "", true, "A", ok},
		{"member", "", true, "B", ok},
		{"grant for all", "", true, "B", ok},
		{"grant for A", "", true, "A", ok},
		{"grant for A", "", true, "B", missing},
		{"break-glass on A", "", true, "A", ok},
		{"break-glass on A", "", true, "B", missing},
		{"expired grant", "", true, "A", missing},
		{"pending grant", "", true, "A", missing},
		// Managing one space: a manager, or a grant that covers it.
		{"owner", "manage", true, "A", ok},
		{"member", "manage", true, "A", forbidden},
		{"grant for all", "manage", true, "B", ok},
		{"grant for A", "manage", true, "A", ok},
		{"grant for A", "manage", true, "B", forbidden},
		{"break-glass on A", "manage", true, "A", ok},
		{"expired grant", "manage", true, "A", forbidden},
		{"pending grant", "manage", true, "A", forbidden},
		// A grant never decides for the organization, whatever it covers.
		{"owner", "decide", true, "A", ok},
		{"member", "decide", true, "A", forbidden},
		{"grant for all", "decide", true, "A", forbidden},
		{"grant for A", "decide", true, "A", forbidden},
		// A list or a create names no space: a list is open and filtered, the rest ask the organization.
		{"grant for A", "", true, "", ok},
		{"grant for A", "manage", true, "", forbidden},
		{"grant for all", "manage", true, "", ok},
		{"grant for A", "decide", true, "", forbidden},
		// An action that is not scoped never counts a grant for one space.
		{"grant for A", "manage", false, "A", forbidden},
		{"grant for all", "manage", false, "A", ok},
		{"owner", "manage", false, "", ok},
		{"member", "manage", false, "", forbidden},
		{"owner", "nonsense", true, "A", forbidden},
	}
	for _, tc := range cases {
		got, why := memberGate(who, callers[tc.caller], tc.role, tc.scoped, tc.item, "", nil)
		if got != tc.want {
			t.Errorf("%s, role %q, scoped %v, item %q: status %d, want %d", tc.caller, tc.role, tc.scoped, tc.item, got, tc.want)
		}
		if (got == 0) != (why == "") {
			t.Errorf("%s, role %q: status %d with message %q", tc.caller, tc.role, got, why)
		}
	}
}

// An action that names a staff permission for grant holders is refused to a grant holder without it,
// whatever the grant covers, and never asks it of a member.
func TestMemberGateStaffPermissionUnderAGrant(t *testing.T) {
	soon := time.Now().Add(time.Hour)
	who := &caller{id: &auth.Identity{Sub: "someone"}, managers: []string{"org_owner"}}
	holder := &tenantView{Roles: []string{}, grants: []db.Grant{{ID: "g", Kind: "support", Status: "approved", WorkspaceID: "A", ExpiresAt: &soon}}}
	owner := &tenantView{Roles: []string{"org_owner"}}
	asked := []string{}
	may := func(answer bool) func(string) bool {
		return func(perm string) bool { asked = append(asked, perm); return answer }
	}
	if got, _ := memberGate(who, holder, "manage", true, "A", "workspaces.act", may(true)); got != 0 {
		t.Errorf("a grant for A with the staff permission: %d", got)
	}
	if got, why := memberGate(who, holder, "manage", true, "A", "workspaces.act", may(false)); got != http.StatusForbidden || why == "" {
		t.Errorf("a grant for A without the staff permission: %d %q", got, why)
	}
	if got, _ := memberGate(who, holder, "manage", true, "A", "workspaces.act", nil); got != http.StatusForbidden {
		t.Errorf("a grant for A with no way to ask the staff permission: %d", got)
	}
	if got, _ := memberGate(who, holder, "manage", true, "B", "workspaces.act", may(true)); got != http.StatusForbidden {
		t.Errorf("the staff permission reached a space the grant does not cover: %d", got)
	}
	if len(asked) != 3 || asked[0] != "workspaces.act" {
		t.Errorf("asked %v", asked)
	}
	asked = nil
	if got, _ := memberGate(who, owner, "manage", true, "A", "workspaces.act", may(false)); got != 0 || len(asked) != 0 {
		t.Errorf("a member was asked for a staff permission: %d %v", got, asked)
	}
}

// A list keeps the spaces the caller sees, and work started under a grant is recorded with the grant
// that covers that space, break-glass first so its use is never recorded as ordinary support.
func TestGrantScopeForOneSpace(t *testing.T) {
	soon := time.Now().Add(time.Hour)
	tv := &tenantView{Roles: []string{}, grants: []db.Grant{
		{ID: "support-a", Kind: "support", Status: "approved", WorkspaceID: "A", ExpiresAt: &soon},
		{ID: "glass-a", Kind: "breakglass", Status: "approved", WorkspaceID: "A", ExpiresAt: &soon},
		{ID: "support-b", Kind: "support", Status: "approved", WorkspaceID: "B", ExpiresAt: &soon},
	}}
	if !tv.sees("A") || !tv.sees("B") || tv.sees("C") {
		t.Fatalf("sees A=%v B=%v C=%v, want true true false", tv.sees("A"), tv.sees("B"), tv.sees("C"))
	}
	if g := tv.grantFor("A"); g == nil || g.ID != "glass-a" {
		t.Fatalf("grant for A = %+v, want the break-glass one", g)
	}
	if g := tv.grantFor("B"); g == nil || g.ID != "support-b" {
		t.Fatalf("grant for B = %+v, want support-b", g)
	}
	if g := tv.grantFor(""); g != nil {
		t.Fatalf("grant for the whole organization = %+v, want none", g)
	}
}
