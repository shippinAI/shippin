package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"shippin.cloud/console/internal/db"
)

// teamCall runs one real Team request as user (enforce mode, fakes behind it).
func teamCall(a *API, method, path, user, body string) (int, map[string]any) {
	mux := http.NewServeMux()
	a.registerOperator(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-Test-User", user)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	out := map[string]any{}
	_ = json.Unmarshal(rec.Body.Bytes(), &out)
	return rec.Code, out
}

func ownerTeam() (*fakeStore, *fakeIdP, *API) {
	s, i := newFakeStore("enforce"), newFakeIdP().add("jo", "staff.owner").add("sam", "staff.support").add("hal")
	return s, i, staffAPI(s, i)
}

const why = `"reason":"test change"`

func TestLastOwnerCantBeRemoved(t *testing.T) {
	s, idp, a := ownerTeam()
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/members/jo/roles", "jo", `{"roles":[],`+why+`}`); c != 409 {
		t.Fatalf("removing the last Owner's role: %d, want 409", c)
	}
	idp.add("kim", "staff.owner")
	if c, _ := teamCall(a, "POST", "/v1/operator/team/members/kim/deactivate", "jo", `{`+why+`}`); c != 200 {
		t.Fatalf("deactivating one of two Owners: %d, want 200", c)
	}
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/members/jo/roles", "jo", `{"roles":[],`+why+`}`); c != 409 {
		t.Fatalf("last Owner again after deactivation: %d, want 409", c)
	}
	if c, _ := teamCall(a, "DELETE", "/v1/operator/team/roles/staff.owner", "jo", ``); c != 409 {
		t.Fatalf("deleting the only Owner role: %d, want 409", c)
	}
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/roles/staff.owner", "jo", `{"name":"Owner","permissions":["team.read"],`+why+`}`); c != 409 {
		t.Fatalf("stripping team.write from the only Owner role: %d, want 409", c)
	}
	if len(idp.calls) != 1 || len(s.audit) == 0 {
		t.Fatalf("only the allowed deactivation may reach Nomen: %v", idp.calls)
	}
}

func TestNoSelfGrant(t *testing.T) {
	_, idp, a := ownerTeam()
	idp.add("kim", "staff.owner")
	idp.grants["sam"] = staffGrant{ID: "g-sam", Roles: []string{"staff.support"}}
	a.staffDB.(*fakeStore).roles[3].Permissions = append(a.staffDB.(*fakeStore).roles[3].Permissions, "team.write")
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/members/sam/roles", "sam", `{"roles":["staff.support","staff.owner"],`+why+`}`); c != 403 {
		t.Fatalf("self-grant: %d, want 403", c)
	}
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/roles/staff.support", "sam", `{"name":"Support","permissions":["tenants.read","team.write","team.roles"],`+why+`}`); c != 403 {
		t.Fatalf("adding permissions to a role you hold: %d, want 403", c)
	}
	if c, _ := teamCall(a, "POST", "/v1/operator/team/members/jo/deactivate", "jo", `{`+why+`}`); c != 409 {
		t.Fatalf("deactivating yourself: %d, want 409", c)
	}
}

func TestOnlyOperatorOrgUsersAndKnownRoles(t *testing.T) {
	_, idp, a := ownerTeam()
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/members/customer1/roles", "jo", `{"roles":["staff.support"],`+why+`}`); c != 404 {
		t.Fatalf("customer user: %d, want 404", c)
	}
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/members/hal/roles", "jo", `{"roles":["staff.ghost"],`+why+`}`); c != 400 {
		t.Fatalf("unknown role: %d, want 400", c)
	}
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/members/hal/roles", "jo", `{"roles":["staff.support"]}`); c != 400 {
		t.Fatalf("missing reason: %d, want 400", c)
	}
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/members/hal/roles", "jo", `{"roles":["staff.support"],"admin":true,`+why+`}`); c != 400 {
		t.Fatalf("unknown field: %d, want 400", c)
	}
	if len(idp.calls) != 0 {
		t.Fatalf("refusals must not touch Nomen: %v", idp.calls)
	}
}

func TestAuditBeforeNomenWrite(t *testing.T) {
	s, idp, a := ownerTeam()
	s.failAudit = true
	for _, rq := range [][3]string{
		{"PUT", "/v1/operator/team/members/hal/roles", `{"roles":["staff.support"],` + why + `}`},
		{"POST", "/v1/operator/team/members/sam/deactivate", `{` + why + `}`},
		{"POST", "/v1/operator/team/members", `{"email":"new@example.test","first_name":"New","last_name":"Person","roles":[],` + why + `}`},
		{"POST", "/v1/operator/team/roles", `{"key":"staff.dns","name":"DNS","permissions":["dns.read"],` + why + `}`},
	} {
		if c, _ := teamCall(a, rq[0], rq[1], "jo", rq[2]); c < 500 {
			t.Errorf("%s %s with a failing audit: %d, want 5xx", rq[0], rq[1], c)
		}
	}
	if len(idp.calls) != 0 {
		t.Fatalf("no Nomen write may happen without its audit entry: %v", idp.calls)
	}
	s.failAudit = false
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/members/hal/roles", "jo", `{"roles":["staff.support"],`+why+`}`); c != 200 {
		t.Fatalf("grant: %d", c)
	}
	if !slices.Contains(s.actions(), "team.roles") || !slices.Equal(idp.calls, []string{"roles hal"}) {
		t.Fatalf("audit %v, calls %v", s.actions(), idp.calls)
	}
}

func TestInviteAnyoneAndNomenFailureIsAudited(t *testing.T) {
	s, idp, a := ownerTeam()
	c, _ := teamCall(a, "POST", "/v1/operator/team/members", "jo", `{"email":"Pat@Example.test","first_name":"Pat","last_name":"Lee","roles":["staff.auditor"],`+why+`}`)
	if c != 200 || !slices.Equal(idp.calls, []string{"invite pat@example.test", "roles new-Pat"}) {
		t.Fatalf("invite: %d %v", c, idp.calls)
	}
	idp.fail = true
	if c, _ := teamCall(a, "PUT", "/v1/operator/team/members/hal/roles", "jo", `{"roles":["staff.support"],`+why+`}`); c != 502 {
		t.Fatalf("Nomen failure: %d, want 502", c)
	}
	if !slices.Contains(s.actions(), "team.roles_failed") {
		t.Fatal("a failed Nomen write must be audited")
	}
	if c, _ := teamCall(a, "POST", "/v1/operator/team/roles", "jo", `{"key":"staff.dns","name":"DNS","permissions":["dns.read"],`+why+`}`); c != 502 {
		t.Fatalf("role create with Nomen down: %d", c)
	}
	if slices.ContainsFunc(s.roles, func(r db.StaffRole) bool { return r.Key == "staff.dns" }) {
		t.Fatal("a role Nomen refused must be removed again")
	}
}
