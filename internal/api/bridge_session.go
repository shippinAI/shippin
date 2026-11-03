package api

import (
	"encoding/json"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/bridge"
	"shippin.cloud/console/internal/db"
)

// bridgeCaller authenticates a bridge bearer token: a live, unrevoked session resolved into the
// same caller (tenants, roles, grants) the console uses, from the account bound at approval.
func (a *API) bridgeCaller(w http.ResponseWriter, r *http.Request) (*caller, *bridge.Session, bool) {
	if !a.bridgeOn(w) {
		return nil, nil, false
	}
	tok, _ := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !strings.HasPrefix(tok, a.n2.br.cfg.TokenPrefix) {
		auth.Problem(w, http.StatusUnauthorized, "Sign in with the Shippin bridge first (shippin-bridge login).")
		return nil, nil, false
	}
	ses, err := a.n2.br.store.ByToken(r.Context(), tok)
	if err != nil || ses == nil {
		auth.Problem(w, http.StatusUnauthorized, "This bridge session expired or was revoked. Sign in again.")
		return nil, nil, false
	}
	var id auth.Identity
	if json.Unmarshal(ses.Identity, &id) != nil || id.Sub != ses.Sub {
		auth.Problem(w, http.StatusUnauthorized, "This bridge session is not bound to an account. Sign in again.")
		return nil, nil, false
	}
	c, err := a.callerFromIdentity(r, &id)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The tenant store is unavailable.")
		return nil, nil, false
	}
	return c, ses, true
}

// bridgeWorkspace finds a workspace the caller may open: a member who manages the tenant or
// created it, or a staff grant covering it (audited). Same rule as the IDE (wsAllowed).
func (a *API) bridgeWorkspace(w http.ResponseWriter, r *http.Request, c *caller, id string) (*db.Workspace, *tenantView, bool) {
	ws, err := a.db.WorkspaceByID(r.Context(), id)
	if err == nil && ws != nil {
		for i := range c.tenants {
			t := &c.tenants[i]
			if t.ID != ws.TenantID {
				continue
			}
			if t.member() && (c.canManage(t) || ws.CreatedBy == c.id.Sub) {
				return ws, t, true
			}
			if !t.member() && c.canManageWS(t, ws.ID) {
				if !a.auditUse(r, c, t, ws.ID, "bridge.open", ws.Name) {
					refuseUnaudited(w)
					return nil, nil, false
				}
				return ws, t, true
			}
		}
	}
	auth.Problem(w, http.StatusNotFound, "Workspace not found.")
	return nil, nil, false
}

// bridgeMe: the account, and the projects and workspaces this session can reach.
func (a *API) bridgeMe(w http.ResponseWriter, r *http.Request) {
	c, ses, ok := a.bridgeCaller(w, r)
	if !ok {
		return
	}
	type wsOut struct {
		ID, Name, State, ProjectID, Tenant, TenantID string
	}
	out := []wsOut{}
	for i := range c.tenants {
		t := &c.tenants[i]
		list, err := a.db.Workspaces(r.Context(), t.ID)
		if err != nil {
			continue
		}
		for _, x := range list {
			if (t.member() && (c.canManage(t) || x.CreatedBy == c.id.Sub)) || (!t.member() && c.canManageWS(t, x.ID)) {
				out = append(out, wsOut{x.ID, x.Name, x.State, x.ProjectID, t.Name, t.ID})
			}
		}
	}
	writeJSON(w, map[string]any{"account": map[string]string{"id": ses.Sub, "email": ses.Email},
		"session": map[string]any{"id": ses.ID, "expires_at": ses.ExpiresAt}, "workspaces": out,
		"ssh_user": a.n2.br.cfg.SSHUser})
}

func (a *API) bridgeSessions(w http.ResponseWriter, r *http.Request) {
	if !a.bridgeOn(w) {
		return
	}
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	list, err := a.n2.br.store.List(r.Context(), c.id.Sub)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Sessions couldn't be read.")
		return
	}
	writeJSON(w, map[string]any{"items": list})
}

func (a *API) bridgeRevoke(w http.ResponseWriter, r *http.Request) {
	if !a.bridgeOn(w) {
		return
	}
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	id := r.PathValue("id")
	if !a.auditFirst(w, r, db.AuditEntry{Actor: c.id.Sub, ActorName: c.id.Name, Action: "bridge.session_revoked", Detail: id, IP: clientIP(r)}) {
		return
	}
	done, err := a.n2.br.store.Revoke(r.Context(), c.id.Sub, id)
	if err != nil || !done {
		auth.Problem(w, http.StatusNotFound, "Session not found.")
		return
	}
	writeJSON(w, map[string]any{"revoked": true, "relays_closed": a.n2.br.closeAll(id)})
}
