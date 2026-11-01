package api

import (
	"testing"

	"shippin.cloud/console/internal/config"
)

func TestLocalRolesFollowConfig(t *testing.T) {
	a := &API{cfg: &config.Config{}}
	a.cfg.Operator.Role = "op_superadmin"
	a.cfg.Roles.IAM = []config.RoleMap{{From: "IAM_ORG_MANAGER", To: "op_admin"}, {From: "IAM_OWNER", To: "op_superadmin"}}
	if got := a.localIAMRole("owner"); got != "IAM_OWNER" {
		t.Fatalf("owner: got %q", got)
	}
	for _, r := range []string{"admin", "member", ""} {
		if got := a.localIAMRole(r); got != "" {
			t.Fatalf("%s must not be an operator, got %q", r, got)
		}
	}
	a.cfg.Operator.Role = "op_missing"
	if got := a.localIAMRole("owner"); got != "" {
		t.Fatalf("no mapping to operator.role must grant nothing, got %q", got)
	}
	for in, want := range map[string]string{"owner": "ORG_OWNER", "admin": "ORG_ADMIN", "member": "ORG_MEMBER", "odd": "ORG_MEMBER"} {
		if got := localOrgRole(in); got != want {
			t.Fatalf("%s: got %s want %s", in, got, want)
		}
	}
}
