package api

import (
	"shippin.cloud/console/internal/auth"
	"slices"
	"strings"
)

// orgRole maps a Nomen org role key to a tenant role via config (roles.org); unlisted keys
// become "org_<key>" so new IdP roles show up without granting anything.
func (a *API) orgRole(r string) string {
	for _, m := range a.cfg.Roles.Org {
		if m.From == r {
			return m.To
		}
	}
	return "org_" + strings.ToLower(strings.TrimPrefix(r, "ORG_"))
}

// opRole maps Nomen instance roles to an operator role via config (roles.iam, first match).
func (a *API) opRole(roles []string) string {
	for _, m := range a.cfg.Roles.IAM {
		if slices.Contains(roles, m.From) {
			return m.To
		}
	}
	return a.cfg.Roles.IAMDefault
}

// isPlatformAdmin: exactly the configured platform admin role (operator.role).
func (a *API) isPlatformAdmin(c *caller) bool {
	return c.op != "" && c.op == a.cfg.Operator.Role
}

// claimRole: the identity holds the Nomen project role, granted by the operator org (a role
// granted by any other org, e.g. a tenant through a project grant, does not count).
func (a *API) claimRole(id *auth.Identity, role string) bool {
	if id == nil || role == "" {
		return false
	}
	return slices.Contains(id.ProjectRoles[role], a.cfg.StaffOrg())
}
