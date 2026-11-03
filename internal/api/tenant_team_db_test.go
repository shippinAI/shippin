package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shippin.cloud/console/internal/auth"
)

// fakeOrgs is an in-memory Nomen org for the Team page.
type fakeOrgs struct {
	members []auth.OrgMember
	users   map[string]string // email -> id
	calls   []string
}

func (f *fakeOrgs) Members(context.Context, string) ([]auth.OrgMember, error) { return f.members, nil }
func (f *fakeOrgs) UserByEmail(_ context.Context, e string) (string, error)   { return f.users[e], nil }
func (f *fakeOrgs) Invite(_ context.Context, _, e, _, _ string) (string, error) {
	f.calls = append(f.calls, "invite "+e)
	return "new-" + e, nil
}
func (f *fakeOrgs) SetMember(_ context.Context, _, u string, roles []string, add bool) error {
	f.calls = append(f.calls, "set "+u+" "+strings.Join(roles, ","))
	return nil
}
func (f *fakeOrgs) RemoveMember(_ context.Context, _, u string) error {
	f.calls = append(f.calls, "remove "+u)
	return nil
}

func TestTenantTeamWrites(t *testing.T) {
	a, _, tn := notifyAPI(t)
	f := &fakeOrgs{members: []auth.OrgMember{{UserID: "own", Roles: []string{"ORG_OWNER"}}, {UserID: "adm", Roles: []string{"ORG_USER_MANAGER"}}},
		users: map[string]string{"known@x.test": "known"}}
	a.tenantIdP = f
	as := func(role string) {
		c := &caller{id: &auth.Identity{Sub: role}, managers: a.cfg.Roles.Managers, tenants: []tenantView{{Tenant: tn, Roles: []string{role}}}}
		a.resolveCaller = func(http.ResponseWriter, *http.Request) (*caller, bool) { return c, true }
	}
	mux := http.NewServeMux()
	a.tenantTeamRoutesDirect(mux)
	do := func(method, path, body string) int {
		w := httptest.NewRecorder()
		mux.ServeHTTP(w, httptest.NewRequest(method, "/v1/tenants/"+tn.ID+path, strings.NewReader(body)))
		return w.Code
	}
	cases := []struct {
		role, method, path, body string
		want                     int
	}{
		{"org_member", "POST", "/members", `{"email":"known@x.test","roles":["ORG_USER_MANAGER"]}`, 403},
		{"org_admin", "POST", "/members", `{"email":"known@x.test","roles":["ORG_OWNER"]}`, 403},
		{"org_admin", "POST", "/members", `{"email":"known@x.test","roles":["IAM_OWNER"]}`, 403},
		{"org_admin", "POST", "/members", `{"email":"known@x.test","roles":["ORG_USER_MANAGER"]}`, 201},
		{"org_admin", "POST", "/members", `{"email":"new@x.test","roles":["ORG_USER_MANAGER"]}`, 422},
		{"org_owner", "POST", "/members", `{"email":"new@x.test","first_name":"N","last_name":"U","roles":["ORG_OWNER"]}`, 201},
		{"org_owner", "PATCH", "/members/own", `{"roles":["ORG_USER_MANAGER"]}`, 403}, // last owner
		{"org_admin", "PATCH", "/members/adm", `{"roles":["ORG_OWNER"]}`, 403},
		{"org_owner", "PATCH", "/members/adm", `{"roles":["ORG_OWNER"]}`, 200},
		{"org_owner", "PATCH", "/members/ghost", `{"roles":["ORG_USER_MANAGER"]}`, 404},
		{"org_owner", "DELETE", "/members/own", ``, 403},
		{"org_owner", "POST", "/teams", `{"name":"backend"}`, 201},
		{"org_owner", "POST", "/teams", `{"name":"backend"}`, 409},
		{"org_owner", "POST", "/teams", `{"name":"Bad"}`, 400},
		{"org_member", "GET", "/team", ``, 200},
	}
	for i, c := range cases {
		as(c.role)
		if got := do(c.method, c.path, c.body); got != c.want {
			t.Errorf("%d %s %s %s: %d, want %d", i, c.role, c.method, c.path, got, c.want)
		}
	}
	want := []string{"set known ORG_USER_MANAGER", "invite new@x.test", "set new-new@x.test ORG_OWNER", "set adm ORG_OWNER"}
	if strings.Join(f.calls, "|") != strings.Join(want, "|") {
		t.Errorf("IdP calls = %v, want %v", f.calls, want)
	}
	teams, _ := a.db.Teams(context.Background(), tn.ID)
	as("org_owner")
	if do("PUT", "/teams/"+teams[0].ID+"/members/ghost", `{}`) != 404 || do("PUT", "/teams/"+teams[0].ID+"/members/adm", `{"role":"maintainer"}`) != 204 {
		t.Error("team member add: outsider must be refused, org member accepted")
	}
	var n int
	_ = a.db.Pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_log WHERE tenant_id=$1 AND action LIKE 'tenant.%'`, tn.ID).Scan(&n)
	if n != 5 { // 2 invites, 1 role change, 1 team create, 1 team member set
		t.Errorf("audit rows = %d, want 5", n)
	}
}
