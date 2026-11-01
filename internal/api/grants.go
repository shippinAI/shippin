package api

import (
	"net/http"
	"slices"
	"time"

	"shippin.cloud/console/internal/db"
)

// addGrantedTenants appends the tenants the admin holds an active grant for (and isn't a
// member of). The grant is re-read on every request, so revoke and expiry apply at once.
func (a *API) addGrantedTenants(r *http.Request, c *caller) error {
	gs, err := a.db.ActiveGrants(r.Context(), c.id.Sub)
	if err != nil || len(gs) == 0 {
		return err
	}
	byTenant := map[string][]db.Grant{}
	for _, g := range gs {
		if !slices.ContainsFunc(c.tenants, func(t tenantView) bool { return t.ID == g.TenantID }) {
			byTenant[g.TenantID] = append(byTenant[g.TenantID], g)
		}
	}
	ids := make([]string, 0, len(byTenant))
	for id := range byTenant {
		ids = append(ids, id)
	}
	ts, err := a.db.TenantsByIDs(r.Context(), ids)
	if err != nil {
		return err
	}
	for _, t := range ts {
		c.tenants = append(c.tenants, tenantView{Tenant: t, Roles: []string{}, grants: byTenant[t.ID]})
	}
	return nil
}

// grantedTenant returns the caller's view of tenant id (membership or grant), or nil.
func (c *caller) grantedTenant(id string) *tenantView {
	for i := range c.tenants {
		if c.tenants[i].ID == id {
			return &c.tenants[i]
		}
	}
	return nil
}

// member: the caller sees this tenant through org membership, not a grant.
func (t *tenantView) member() bool { return len(t.grants) == 0 }

// grantFor returns the active grant covering workspace wsID ("" = tenant-level), preferring a
// break-glass grant so its use is always audited as break-glass.
func (t *tenantView) grantFor(wsID string) *db.Grant {
	var found *db.Grant
	now := time.Now()
	for i := range t.grants {
		g := &t.grants[i]
		if g.Active(now) && g.Covers(wsID) {
			if found == nil || g.Kind == "breakglass" {
				found = g
			}
		}
	}
	return found
}

// canManage: tenant-level management. Members need a configured manager role; a platform
// admin needs an active tenant-wide grant. Operator roles alone never qualify.
func (c *caller) canManage(t *tenantView) bool {
	ok := t.grantFor("") != nil
	if t.member() {
		ok = slices.ContainsFunc(t.Roles, func(r string) bool { return slices.Contains(c.managers, r) })
	}
	c.fga.compare(c, t, "", "can_manage", ok)
	return ok
}

// canManageWS: management of one workspace (tenant-level, or a grant scoped to it).
func (c *caller) canManageWS(t *tenantView, wsID string) bool {
	ok := c.canManage(t) || (!t.member() && t.grantFor(wsID) != nil)
	c.fga.compare(c, t, wsID, "can_manage", ok)
	return ok
}

// canDecide: only member owners/admins approve, deny or revoke access; never a grant holder.
func (c *caller) canDecide(t *tenantView) bool {
	ok := t.member() && c.canManage(t)
	c.fga.compare(c, t, "", "can_decide", ok)
	return ok
}

// sees: workspaces a workspace-scoped grant holder may list.
func (t *tenantView) sees(wsID string) bool { return t.member() || t.grantFor(wsID) != nil }

type grantRef struct {
	Kind      string    `json:"kind"`
	Scope     string    `json:"scope"`
	ExpiresAt time.Time `json:"expires_at"`
}

func (t *tenantView) accessRef(now time.Time) *grantRef {
	for _, g := range t.grants {
		if g.Active(now) {
			scope := "tenant"
			if g.WorkspaceID != "" {
				scope = g.WorkspaceName
			}
			return &grantRef{g.Kind, scope, *g.ExpiresAt}
		}
	}
	return nil
}
