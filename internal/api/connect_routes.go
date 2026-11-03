package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// connectRoutes: "Connect accounts" for one workspace. The workspace's creator or a tenant org
// owner/admin (a member holding a cfg.Roles.Managers role) may connect or open a terminal.
// Staff reaching the tenant only through an access grant are never allowed (not members).
func (a *API) connectRoutes(mux Router) {
	base := "/v1/tenants/{tenantId}/workspaces/{workspaceId}/connections"
	mux.HandleFunc("GET "+base, a.flow.Require(a.listConnections))
	mux.HandleFunc("POST "+base+"/{provider}/login", a.flow.Require(a.startConnectLogin))
	mux.HandleFunc("GET "+base+"/{provider}/login", a.flow.Require(a.connectLoginState))
	mux.HandleFunc("POST "+base+"/{provider}/code", a.flow.Require(a.submitConnectCode))
	mux.HandleFunc("PUT "+base+"/{provider}/token", a.flow.Require(a.putConnectToken))
	mux.HandleFunc("DELETE "+base+"/{provider}", a.flow.Require(a.disconnectProvider))
	mux.HandleFunc("GET /v1/tenants/{tenantId}/workspaces/{workspaceId}/terminal", a.flow.Require(a.terminalSocket))
}

type connectCtx struct {
	c        *caller
	t        *tenantView
	ws       *workspaceView
	provider string
}

// connectFor resolves the caller, tenant, workspace (creator only) and, when asked, provider.
func (a *API) connectFor(w http.ResponseWriter, r *http.Request, wantProvider bool) (*connectCtx, bool) {
	if !a.cfg.Connect.Enabled {
		auth.Problem(w, http.StatusNotFound, "Connect accounts isn't switched on.")
		return nil, false
	}
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return nil, false
	}
	items, err := a.liveViews(r, t)
	ws := a.findWorkspace(r, items)
	if err != nil || ws == nil {
		auth.Problem(w, http.StatusNotFound, "Workspace not found.")
		return nil, false
	}
	if !connectAllowed(t.member(), ws.CreatedBy == c.id.Sub, t.member() && c.canManage(t)) {
		auth.Problem(w, http.StatusForbidden, "Only the workspace's creator or a tenant owner or admin can connect accounts or open its terminal.")
		return nil, false
	}
	p := r.PathValue("provider")
	if _, known := a.cfg.Connect.Provider(p); wantProvider && !known {
		auth.Problem(w, http.StatusNotFound, "Unknown provider.")
		return nil, false
	}
	return &connectCtx{c: c, t: t, ws: ws, provider: p}, true
}

func (a *API) connectAudit(r *http.Request, x *connectCtx, action string) bool {
	return a.db.AddAudit(r.Context(), db.AuditEntry{TenantID: x.t.ID, WorkspaceID: x.ws.ID, Actor: x.c.id.Sub,
		ActorName: x.c.id.Name, Action: action, Detail: x.provider, IP: clientIP(r)}) == nil
}

// connectAllowed (t1029): members only; the creator, or an org owner/admin (manager) of the tenant.
func connectAllowed(member, creator, manager bool) bool {
	return member && (creator || manager)
}
