package api

import (
	"net/http/httptest"
	"testing"

	"shippin.cloud/console/internal/auth"
)

func TestAdminHostsArePlatformAdminOnly(t *testing.T) {
	a := &API{cfg: testCfg(), staffDB: newFakeStore("legacy"), staffIdP: newFakeIdP()}
	r := httptest.NewRequest("GET", "/auth/ws-check", nil)
	a.cfg.Operator.AdminHosts = []string{"mail-admin.example.test"}
	who := func(op string) *caller { return &caller{id: &auth.Identity{Sub: "u-" + op}, op: op} }
	if !a.adminPermit(r, who("op_superadmin"), "mail-admin.example.test") {
		t.Fatal("platform admin must open the admin host")
	}
	for _, op := range []string{"op_admin", "op_support", ""} {
		if a.adminPermit(r, who(op), "mail-admin.example.test") {
			t.Errorf("role %q must not open the admin host", op)
		}
	}
	if a.adminPermit(r, who("op_superadmin"), "other.example.test") || a.adminHost("ws-1.ws.example.test") {
		t.Fatal("only configured hosts are admin hosts")
	}
}

// Enforced: the admin tools open only with host.admin_tools from a staff role.
func TestAdminHostsNeedAdminToolsPermission(t *testing.T) {
	idp := newFakeIdP().add("pa", "staff.platform_admin").add("sup", "staff.support")
	a := staffAPI(newFakeStore("enforce"), idp)
	a.cfg.Operator.AdminHosts = []string{"mail-admin.example.test"}
	r := httptest.NewRequest("GET", "/auth/ws-check", nil)
	if !a.adminPermit(r, &caller{id: testID("pa")}, "mail-admin.example.test") {
		t.Fatal("platform admin preset must open the admin host")
	}
	if a.adminPermit(r, &caller{id: testID("sup")}, "mail-admin.example.test") {
		t.Fatal("support must not open the admin host")
	}
}
