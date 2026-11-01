package api

import (
	"net/http/httptest"
	"testing"
)

// In legacy mode the tenant→admin switch (provider.panel) follows the old IAM_OWNER gate only.
func TestProviderPanelCapabilityLegacyIAMOwner(t *testing.T) {
	a := staffAPI(newFakeStore("legacy"), newFakeIdP())
	r := httptest.NewRequest("GET", "/v1/me", nil)
	for roles, want := range map[string]bool{"IAM_OWNER": true, "IAM_ORG_MANAGER": false, "IAM_USER_MANAGER": false, "": false} {
		c := &caller{id: testID("u1")}
		if roles != "" {
			c.op = a.opRole([]string{roles})
		}
		got := a.capabilities(r, c)
		if got["provider.panel"] != want || got["tenants.create"] != want {
			t.Errorf("%q: provider.panel=%v want %v", roles, got["provider.panel"], want)
		}
	}
	if a.capabilities(r, &caller{id: testID("u1"), op: "org_owner"})["provider.panel"] {
		t.Error("a tenant owner role must not open the provider panel")
	}
}

// In enforce mode the switch comes from staff roles: any permission opens the panel, and
// tenants.create needs tenants.lifecycle.
func TestProviderPanelCapabilityFromStaffRoles(t *testing.T) {
	idp := newFakeIdP().add("sup", "staff.support").add("ops", "staff.ops").add("none")
	a := staffAPI(newFakeStore("enforce"), idp)
	r := httptest.NewRequest("GET", "/v1/me", nil)
	for u, want := range map[string][2]bool{"sup": {true, false}, "ops": {true, true}, "none": {false, false}} {
		got := a.capabilities(r, &caller{id: testID(u)})
		if got["provider.panel"] != want[0] || got["tenants.create"] != want[1] {
			t.Errorf("%s: panel=%v create=%v, want %v", u, got["provider.panel"], got["tenants.create"], want)
		}
	}
}
