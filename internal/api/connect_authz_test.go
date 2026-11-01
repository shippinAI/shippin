package api

import "testing"

func TestConnectAllowed(t *testing.T) {
	cases := []struct {
		name                     string
		member, creator, manager bool
		want                     bool
	}{
		{"creator member", true, true, false, true},
		{"owner/admin member, not creator", true, false, true, true},
		{"plain member, not creator", true, false, false, false},
		{"non-member creator (grant only)", false, true, false, false},
		{"non-member staff with grant", false, false, true, false},
		{"nobody", false, false, false, false},
	}
	for _, c := range cases {
		if got := connectAllowed(c.member, c.creator, c.manager); got != c.want {
			t.Errorf("%s: got %v want %v", c.name, got, c.want)
		}
	}
}

func TestCanManageOwnerRole(t *testing.T) {
	c := &caller{managers: []string{"org_owner", "org_admin"}}
	for _, tc := range []struct {
		roles []string
		want  bool
	}{{[]string{"org_owner"}, true}, {[]string{"org_admin"}, true}, {[]string{"org_member"}, false}, {nil, false}} {
		tv := &tenantView{Roles: tc.roles}
		if got := connectAllowed(tv.member(), false, tv.member() && c.canManage(tv)); got != tc.want {
			t.Errorf("roles %v: got %v want %v", tc.roles, got, tc.want)
		}
	}
}
