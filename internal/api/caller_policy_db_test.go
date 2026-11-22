package api

import (
	"context"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/ext"
)

type fakeCallerPolicy struct {
	rules  map[string]ext.OrgRule
	access map[string][]ext.Access
	asked  string
}

func (f *fakeCallerPolicy) OrgRules(context.Context, ext.Runtime, []string) (map[string]ext.OrgRule, error) {
	return f.rules, nil
}

func (f *fakeCallerPolicy) Access(_ context.Context, _ ext.Runtime, sub string, may func(string) bool) (map[string][]ext.Access, error) {
	if may == nil {
		panic("no permission check")
	}
	f.asked = sub
	return f.access, nil
}

// A module's caller policy is the one the panel asks: its rules label the organizations, and its access
// opens an organization the caller is not a member of, audited as break-glass when it says so.
func TestAModuleCallerPolicyDecidesAccess(t *testing.T) {
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("SHIPPIN_TEST_DATABASE_URL not set")
	}
	d, err := db.Open(context.Background(), url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(d.Pool.Close)
	id, _ := uuid.NewV7()
	org, _ := uuid.NewV7()
	tn := db.Tenant{ID: id.String(), IdpOrgID: org.String(), Name: "Policy Co", Plan: "p"}
	if err := d.CreateTenant(context.Background(), tn); err != nil {
		t.Fatal(err)
	}

	f := &fakeCallerPolicy{
		rules: map[string]ext.OrgRule{tn.ID: {Label: "Gold", SignIn: "second_factor"}},
		access: map[string][]ext.Access{tn.ID: {
			{ID: "a1", OrgID: tn.ID, WorkspaceID: "w1", Kind: "support", ExpiresAt: time.Now().Add(time.Hour)},
			{ID: "a2", OrgID: tn.ID, WorkspaceID: "w1", BreakGlass: true, ExpiresAt: time.Now().Add(time.Hour)},
			{ID: "a3", OrgID: tn.ID, ExpiresAt: time.Now().Add(-time.Minute)},
		}},
	}
	defer ext.Swap(ext.Module{Name: "probe", Caller: f})()
	a := &API{cfg: testCfg(), db: d}

	rules, err := a.orgRules(context.Background(), []string{tn.ID})
	if err != nil || rules[tn.ID].Label != "Gold" || rules[tn.ID].SignIn != "second_factor" {
		t.Fatalf("rules: %+v %v", rules, err)
	}

	c := &caller{id: &auth.Identity{Sub: "staff-1"}, managers: []string{"org_owner"}}
	if err := a.addAccess(httptest.NewRequest("GET", "/", nil), c); err != nil {
		t.Fatal(err)
	}
	if f.asked != "staff-1" {
		t.Errorf("the policy was asked for %q", f.asked)
	}
	tv := c.grantedTenant(tn.ID)
	if tv == nil || tv.member() {
		t.Fatalf("the organization is not reachable through the policy's access: %+v", c.tenants)
	}
	if !c.canManageWS(tv, "w1") || c.canManageWS(tv, "w2") || c.canManage(tv) {
		t.Error("the access must reach exactly its workspace")
	}
	if g := tv.grantFor("w1"); g == nil || kindPrefix(g) != "breakglass." || g.ID != "a2" {
		t.Errorf("break-glass access must be the one audited: %+v", g)
	}

	// A member's own organization is never listed twice.
	member := &caller{id: &auth.Identity{Sub: "staff-1"}, tenants: []tenantView{{Tenant: tn, Roles: []string{"org_owner"}}}}
	if err := a.addAccess(httptest.NewRequest("GET", "/", nil), member); err != nil || len(member.tenants) != 1 {
		t.Fatalf("member: %d views, %v", len(member.tenants), err)
	}
}

// Without a module the panel answers as it did before policies existed.
func TestNoModuleKeepsTheCurrentCallerPolicy(t *testing.T) {
	defer ext.Swap()()
	if _, ok := (&API{}).callerPolicy().(currentCaller); !ok {
		t.Fatal("the panel's own caller policy changed")
	}
	if g := grantFromAccess("o", ext.Access{ID: "x", Kind: "breakglass"}); g.Kind != "request" || g.Active(time.Now()) {
		t.Errorf("an access that does not say break-glass must not be audited as one, and without expiry is not active: %+v", g)
	}
}
