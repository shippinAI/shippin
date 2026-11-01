package api

import (
	"encoding/json"
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hostclient"
)

type opWorkspace struct {
	workspaceView
	TenantName   string `json:"tenant_name"`
	TenantStatus string `json:"tenant_status"`
}

func (a *API) opWorkspaces(w http.ResponseWriter, r *http.Request, _ *caller) {
	ts, err := a.db.TenantsByOrgs(r.Context(), nil)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The tenant store is unavailable.")
		return
	}
	wss, err := a.db.AllWorkspaces(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The workspace store is unavailable.")
		return
	}
	names := map[string]string{}
	if users, err := a.idpUsers(r.Context()); err == nil {
		for _, u := range users {
			names[u.ID] = u.Login
		}
	}
	out := []opWorkspace{}
	for _, t := range ts {
		tv := &tenantView{Tenant: t}
		var live map[string]hostclient.Instance
		if t.Status != "deleted" {
			if insts, partial, err := a.host.TenantInventory(r.Context(), t.ID); err == nil && !partial {
				live = map[string]hostclient.Instance{}
				for _, i := range insts {
					live[i.Name] = i
				}
			}
		}
		for _, ws := range wss {
			if ws.TenantID == t.ID {
				if n := names[ws.CreatedBy]; n != "" {
					ws.CreatedBy = n
				}
				out = append(out, opWorkspace{a.view(tv, ws, live), t.Name, t.Status})
			}
		}
	}
	writeJSON(w, map[string]any{"items": out})
}

// opWorkspaceAction lets a platform admin start, stop or delete a workspace (202 + polling)
// only while holding a customer-approved or break-glass grant covering it; each use is
// audited. Delete needs the exact name in {"confirm": ...} and runs the owner's delete path.
func (a *API) opWorkspaceAction(w http.ResponseWriter, r *http.Request, c *caller) {
	act := r.PathValue("action")
	if act != "start" && act != "stop" && act != "delete" {
		auth.Problem(w, http.StatusBadRequest, "Only start, stop and delete are available here.")
		return
	}
	ws, err := a.db.WorkspaceActionTarget(r.Context(), r.PathValue("workspaceId"))
	if err != nil || ws == nil {
		auth.Problem(w, http.StatusNotFound, "Workspace not found.")
		return
	}
	if act == "delete" {
		var body struct {
			Confirm string `json:"confirm"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&body) != nil || body.Confirm != ws.Name {
			auth.Problem(w, http.StatusBadRequest, "Type the workspace name exactly to confirm deletion.")
			return
		}
	}
	t := c.grantedTenant(ws.TenantID)
	if t == nil || !c.canManageWS(t, ws.ID) {
		auth.Problem(w, http.StatusForbidden, "Workspace actions need the customer's approval. Request access first.")
		return
	}
	if !a.auditUse(r, c, t, ws.ID, "action", act) {
		refuseUnaudited(w)
		return
	}
	a.runAction(w, r, t, *ws, act, "operator "+c.id.Sub)
}
