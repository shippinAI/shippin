package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/hostclient"
)

// tenantCtx resolves caller + tenant from the path, or writes a problem.
func (a *API) tenantCtx(w http.ResponseWriter, r *http.Request) (*caller, *tenantView, bool) {
	c, ok := a.caller(w, r)
	if !ok {
		return nil, nil, false
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	return c, t, ok
}

func (a *API) liveViews(r *http.Request, t *tenantView) ([]workspaceView, error) {
	rows, err := a.db.Workspaces(r.Context(), t.ID)
	if err != nil {
		return nil, err
	}
	var live map[string]hostclient.Instance
	if insts, partial, err := a.host.TenantInventory(r.Context(), t.ID); err == nil && !partial {
		live = map[string]hostclient.Instance{}
		for _, i := range insts {
			live[i.Name] = i
		}
	}
	out := make([]workspaceView, 0, len(rows))
	for _, row := range rows {
		if t.sees(row.ID) {
			out = append(out, a.view(t, row, live))
		}
	}
	return out, nil
}

func (a *API) listWorkspaces(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	items, err := a.liveViews(r, t)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Workspaces are unavailable right now.")
		return
	}
	plan, _ := a.host.Plan(r.Context(), t.Plan)
	writeJSON(w, map[string]any{"items": items, "plan": map[string]any{"name": t.Plan, "limits": plan}, "page": map[string]any{"next_page_token": nil},
		"can_manage": c.canManage(t), "tenant_status": t.Status})
}

func (a *API) getWorkspace(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	items, err := a.liveViews(r, t)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Workspaces are unavailable right now.")
		return
	}
	for _, it := range items {
		if it.ID == r.PathValue("workspaceId") {
			writeJSON(w, it)
			return
		}
	}
	auth.Problem(w, http.StatusNotFound, "Workspace not found.")
}
