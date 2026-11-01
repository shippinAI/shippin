package api

import (
	"encoding/json"
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// putPreviews replaces the workspace's preview ports (each private or public) and syncs the edge.
func (a *API) putPreviews(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	items, err := a.liveViews(r, t)
	ws := a.findWorkspace(r, items)
	if err != nil || ws == nil {
		auth.Problem(w, http.StatusNotFound, "Workspace not found.")
		return
	}
	if !c.canManageWS(t, ws.ID) && !(t.member() && ws.CreatedBy == c.id.Sub) {
		auth.Problem(w, http.StatusForbidden, "Only the workspace owner or an org admin can change previews.")
		return
	}
	var in struct {
		Previews []db.Preview `json:"previews"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10)).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Invalid request body.")
		return
	}
	if in.Previews == nil {
		in.Previews = []db.Preview{}
	}
	if msg := checkPreviews(in.Previews, a.cfg.Workspaces); msg != "" {
		auth.Problem(w, http.StatusBadRequest, msg)
		return
	}
	if !a.auditUse(r, c, t, ws.ID, "change", "previews") {
		refuseUnaudited(w)
		return
	}
	if ws.State != "running" {
		auth.Problem(w, http.StatusConflict, "Start the workspace to change previews.")
		return
	}
	next := ws.Workspace
	next.Previews = in.Previews
	if err := a.host.Action(r.Context(), t.ID, ws.ID, "previews", a.hostReq(t, next)); err != nil {
		auth.Problem(w, http.StatusConflict, "Couldn't publish previews: "+err.Error())
		return
	}
	if err := a.db.SetWorkspacePreviews(r.Context(), ws.ID, in.Previews); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Previews were published but couldn't be saved; try again.")
		return
	}
	writeJSON(w, a.view(t, next, nil).PreviewURLs)
}

// listening reports ports the workspace currently listens on, for "add preview" suggestions.
func (a *API) listening(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	items, err := a.liveViews(r, t)
	ws := a.findWorkspace(r, items)
	if err != nil || ws == nil {
		auth.Problem(w, http.StatusNotFound, "Workspace not found.")
		return
	}
	ports, err := a.host.Listening(r.Context(), t.ID, ws.ID)
	if err != nil {
		writeJSON(w, map[string]any{"items": []any{}, "note": "Start the workspace to detect ports."})
		return
	}
	writeJSON(w, map[string]any{"items": ports})
}
