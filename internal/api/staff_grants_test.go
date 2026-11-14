package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"shippin.cloud/console/internal/db"
)

// Only the Owner preset holds the granular catalog permissions, and it holds all of them.
func TestOnlyOwnerPresetHoldsCatalogPerms(t *testing.T) {
	gran := GranularCatalogPerms()
	if len(gran) != 13 {
		t.Fatalf("want 13 granular catalog permissions, got %d: %v", len(gran), gran)
	}
	for _, p := range gran {
		if p == "acquisition.read" || p == "acquisition.sync" {
			t.Fatalf("new acquisition grants must not be inherited by dev accounts: %s", p)
		}
	}
	for _, p := range db.StaffPresets {
		got := permsFor(db.StaffPresets, []string{p.Key})
		for _, g := range gran {
			if got[g] != (p.Key == "staff.owner") {
				t.Errorf("%s holds %s = %v", p.Key, g, got[g])
			}
		}
	}
	for _, k := range []string{"billing.read", "billing.write"} {
		if !permIndex[k].Reserved {
			t.Errorf("%s must be reserved again (grants nothing)", k)
		}
	}
}

func TestAcquisitionPermissionsAreGrantableAndOwnerPresetOnly(t *testing.T) {
	for _, key := range []string{"acquisition.read", "acquisition.sync"} {
		p, ok := permIndex[key]
		if !ok || !p.Grantable {
			t.Fatalf("%s is not grantable", key)
		}
		wantRisk := key == "acquisition.sync"
		if p.Risk != wantRisk {
			t.Errorf("%s high-risk=%v, want %v", key, p.Risk, wantRisk)
		}
		for _, role := range db.StaffPresets {
			if slices.Contains(role.Permissions, key) != (role.Key == "staff.owner") {
				t.Errorf("%s in role %s incorrectly", key, role.Key)
			}
		}
	}
}

func grantAPI() (*API, *fakeStore, *fakeGrants) {
	s := newFakeStore("enforce")
	idp := newFakeIdP().add("own", "staff.owner").add("sup", "staff.support").add("aud", "staff.auditor").add("norole")
	a := staffAPI(s, idp)
	g := &fakeGrants{store: s}
	a.permDB = g
	return a, s, g
}

// A direct grant opens exactly its routes for a staff member; without a staff role it opens nothing.
func TestDirectGrantOpensOnlyItsRoutes(t *testing.T) {
	a, _, g := grantAPI()
	g.gs = []db.PermGrant{{Subject: "sup", Permission: "catalog.plans.read"}, {Subject: "norole", Permission: "catalog.plans.read"}}
	mux := stubMux(a)
	for _, rt := range a.catalogRoutes() {
		want := 403
		if rt.perm == "catalog.plans.read" {
			want = 204
		}
		if got := call(mux, rt.pattern, "sup", false); got != want {
			t.Errorf("sup %s: got %d, want %d", rt.pattern, got, want)
		}
		if got := call(mux, rt.pattern, "norole", false); got != 403 {
			t.Errorf("norole %s: got %d, want 403", rt.pattern, got)
		}
		if got := call(mux, rt.pattern, "aud", false); got != 403 {
			t.Errorf("auditor %s: got %d, want 403", rt.pattern, got)
		}
	}
}

func send(a *API, method, path, user, body string) int {
	mux := http.NewServeMux()
	a.registerOperator(mux)
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer test")
	req.Header.Set("X-Test-User", user)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	return rec.Code
}

// Grant and revoke: Owner only, grantable permissions only, never yourself, always audited.
func TestGrantAPI(t *testing.T) {
	a, s, g := grantAPI()
	const p = "/v1/operator/team/permission-grants"
	body := func(sub, perm string) string {
		return `{"subject":"` + sub + `","permission":"` + perm + `","reason":"pricing review Q4"}`
	}
	cases := []struct {
		user, body string
		want       int
	}{
		{"sup", body("aud", "catalog.plans.read"), 403},
		{"own", body("sup", "team.write"), 400},
		{"own", body("own", "catalog.plans.read"), 400},
		{"own", body("ghost", "catalog.plans.read"), 404},
		{"own", `{"subject":"sup","permission":"catalog.plans.read","reason":"x"}`, 400},
		{"own", body("sup", "customers.notes.read"), 201},
		{"own", body("sup", "customers.notes.read"), 409},
	}
	for _, c := range cases {
		if got := send(a, "POST", p, c.user, c.body); got != c.want {
			t.Errorf("POST %s by %s: got %d, want %d", c.body, c.user, got, c.want)
		}
	}
	if len(g.gs) != 1 || !slices.Contains(s.actions(), "team.perm_grant") {
		t.Fatalf("grant not stored/audited: %v %v", g.gs, s.actions())
	}
	if !a.staffOf(context.Background(), &caller{id: testID("sup")}).Perms["customers.notes.read"] {
		t.Fatal("grant not effective")
	}
	if got := send(a, "DELETE", p+"/sup/customers.notes.read?reason=review+done", "sup", ""); got != 403 {
		t.Errorf("support revoke: got %d, want 403", got)
	}
	if got := send(a, "DELETE", p+"/sup/customers.notes.read?reason=review+done", "own", ""); got != 204 {
		t.Errorf("owner revoke: got %d, want 204", got)
	}
	if len(g.gs) != 0 || !slices.Contains(s.actions(), "team.perm_revoke") {
		t.Fatal("revoke not applied/audited")
	}
	if a.staffOf(context.Background(), &caller{id: testID("sup")}).Perms["customers.notes.read"] {
		t.Fatal("revoked grant still effective (cache not flushed)")
	}
}

// A granular permission can't be put into a role other than the Owner preset.
func TestGranularPermsStayOutOfRoles(t *testing.T) {
	a, _, _ := grantAPI()
	body := `{"key":"staff.pricing","name":"Pricing","description":"","permissions":["catalog.prices.read"],"reason":"pricing team"}`
	if got := send(a, "POST", "/v1/operator/team/roles", "own", body); got != 400 {
		t.Errorf("create role with a granular permission: got %d, want 400", got)
	}
	upd := `{"name":"Support","description":"","permissions":["tenants.read","customers.read"],"reason":"support wants it"}`
	if got := send(a, "PUT", "/v1/operator/team/roles/staff.support", "own", upd); got != 400 {
		t.Errorf("add a granular permission to Support: got %d, want 400", got)
	}
}
