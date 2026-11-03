package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"shippin.cloud/console/internal/auth"
)

// A simulated Nomen userinfo with the founder project role passes the bastion admin-host
// check in every mode; the same user without it, or with it granted by another org, does not.
func TestFounderClaimPassesAdminHost(t *testing.T) {
	for _, mode := range []string{"legacy", "shadow", "enforce"} {
		a := &API{cfg: testCfg(), staffDB: newFakeStore(mode), staffIdP: newFakeIdP()}
		a.cfg.Operator.PlatformOrgID, a.cfg.Staff.OrgID = "op-org", ""
		a.cfg.IdentityDefaults()
		a.cfg.Operator.AdminHosts = []string{"bastion.example.test"}
		r := httptest.NewRequest("GET", "/auth/ws-login", nil)
		mk := func(sub, claims string) *caller {
			var ui map[string]any
			_ = json.Unmarshal([]byte(claims), &ui)
			id := &auth.Identity{Sub: sub, ProjectRoles: auth.ProjectRoles(ui, []string{"urn:nomen:iam:org:project:roles"})}
			return &caller{id: id, founder: a.claimRole(id, a.cfg.Identity.FounderRole)}
		}
		f := mk("operator", `{"urn:nomen:iam:org:project:roles":{"founder":{"op-org":"shippin.cloud"}}}`)
		if !f.founder || !a.adminPermit(r, f, "bastion.example.test") {
			t.Fatalf("%s: founder must open the bastion", mode)
		}
		for _, c := range []*caller{
			mk("other", `{}`),
			mk("staffish", `{"urn:nomen:iam:org:project:roles":{"staff.support":{"op-org":"x"}}}`),
			mk("tenant", `{"urn:nomen:iam:org:project:roles":{"founder":{"tenant-org":"x"}}}`),
		} {
			if c.founder || a.adminPermit(r, c, "bastion.example.test") {
				t.Errorf("%s: %s must get 403 on the bastion", mode, c.id.Sub)
			}
		}
	}
}
