package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
)

type opRoute struct {
	pattern string
	perm    string // catalog permission (or permPanel); a route without one stops startup
	h       func(http.ResponseWriter, *http.Request, *caller)
}

// operatorRoutes is the whole provider panel API; each entry declares its permission.
func (a *API) operatorRoutes() []opRoute {
	return append([]opRoute{
		{"GET /v1/operator/access", permPanel, a.opAccess},
		{"GET /v1/operator/system", "tenants.read", a.opSystem},
		{"GET /v1/operator/tenants", "tenants.read", a.opTenants},
		{"POST /v1/operator/tenants/{tenantId}/provision", "tenants.lifecycle", a.opRetry},
		{"POST /v1/operator/tenants/{tenantId}/setup/rrflow", "tenants.lifecycle", a.opSetupRRFlow},
		{"POST /v1/operator/tenants/{tenantId}/suspend", "tenants.lifecycle", a.opSuspend(true)},
		{"POST /v1/operator/tenants/{tenantId}/resume", "tenants.lifecycle", a.opSuspend(false)},
		{"DELETE /v1/operator/tenants/{tenantId}", "tenants.delete", a.opDeleteTenant},
		{"GET /v1/operator/tenants/{tenantId}/vm", "tenants.read", a.opTenantVM},
		{"POST /v1/operator/tenants/{tenantId}/vm", "tenants.lifecycle", a.opEnableTenantVM},
		{"POST /v1/operator/tenants/{tenantId}/vm/{action}", "tenants.lifecycle", a.opTenantVMAction},
		{"GET /v1/operator/tenants/{tenantId}/apps", "tenants.read", a.opTenantApps},
		{"POST /v1/operator/tenants/{tenantId}/desktop/prepare", "tenants.lifecycle", a.opDesktopPrepare},
		{"POST /v1/operator/tenants/{tenantId}/vm/harden", "tenants.lifecycle", a.opHardenInner},
		{"GET /v1/operator/workspaces", "workspaces.read", a.opWorkspaces},
		{"GET /v1/operator/tenants/{tenantId}/workspaces/{workspaceId}/discover", "projects.create", a.opDiscover},
		{"POST /v1/operator/tenants/{tenantId}/projects", "projects.create", a.opCreateProject},
		{"POST /v1/operator/workspaces/{workspaceId}/{action}", "workspaces.act", a.opWorkspaceAction},
		{"GET /v1/operator/identity", "identity.read", a.opIdentity},
		{"GET /v1/operator/templates", "templates.read", a.opTemplates},
		{"POST /v1/operator/templates", "templates.write", a.opSaveTemplate},
		{"PUT /v1/operator/templates/{templateId}", "templates.write", a.opSaveTemplate},
		{"DELETE /v1/operator/templates/{templateId}", "templates.write", a.opDeleteTemplate},
		{"GET /v1/operator/host", "host.read", a.opHost},
		{"GET /v1/operator/edge", "edge.read", a.opEdge},
		{"GET /v1/operator/access-requests", "access.request", a.opGrants},
		{"POST /v1/operator/access-requests", "access.request", a.opRequestAccess},
		{"POST /v1/operator/access-requests/{grantId}/end", "access.request", a.opEndGrant},
		{"POST /v1/operator/break-glass", "access.breakglass", a.opBreakGlass},
		{"GET /v1/operator/break-glass/pending", "access.approve_breakglass", a.opBreakGlassPending},
		{"POST /v1/operator/break-glass/{grantId}/{decision}", "access.approve_breakglass", a.opDecideBreakGlass},
		{"GET /v1/operator/audit", "audit.read", a.opAudit},
		{"GET /v1/operator/tenants/{tenantId}", "tenants.read", a.opTenantDetail},
		{"GET /v1/operator/tenants/{tenantId}/setup", "tenants.read", a.opTenantSetup},
	}, append(append(a.teamRoutes(), a.grantRoutes()...), append(a.catalogRoutes(), append(a.boundaryRoutes(), append(a.staffSwitchRoutes(), a.platformRoutes()...)...)...)...)...)
}

// registerOperator mounts the provider panel API. Every handler is gated server-side.
func (a *API) registerOperator(mux Router) {
	a.mountOperator(mux, a.operatorRoutes())
}

// mountOperator refuses to start with a route that has no catalog permission (deny by default
// can't be forgotten).
func (a *API) mountOperator(mux Router, routes []opRoute) {
	for _, rt := range routes {
		if rt.perm != permPanel && !knownPerm(rt.perm) {
			panic("operator route " + rt.pattern + " has no catalog permission (" + rt.perm + ")")
		}
		if permIndex[rt.perm].Reserved {
			panic("operator route " + rt.pattern + " uses reserved permission " + rt.perm)
		}
		mux.HandleFunc(rt.pattern, a.operatorOrigin(a.flow.Require(a.gate(rt.perm, rt.h))))
	}
}

// gate allows the request only when the caller holds perm (see can); refusals are audited.
func (a *API) gate(perm string, h func(http.ResponseWriter, *http.Request, *caller)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, ok := a.resolve(w, r)
		if !ok {
			return
		}
		if !a.can(r, c, perm) {
			a.auditOnce(r, c, "authz.denied", perm, "refused")
			auth.Problem(w, http.StatusForbidden, "Your staff role doesn't include this ("+perm+").")
			return
		}
		if !a.staffSessionGate(w, r, c, perm) {
			return
		}
		a.touchStaff(r, c)
		h(w, r, c)
	}
}

func (a *API) opAccess(w http.ResponseWriter, r *http.Request, c *caller) {
	writeJSON(w, map[string]any{"role": c.op, "name": c.id.Name, "user_id": c.id.Sub, "staff": a.staffSummary(r, c)})
}

// resolve is the caller lookup used by the gate (a test seam; defaults to the IdP lookup).
func (a *API) resolve(w http.ResponseWriter, r *http.Request) (*caller, bool) {
	if a.resolveCaller != nil {
		return a.resolveCaller(w, r)
	}
	return a.caller(w, r)
}
