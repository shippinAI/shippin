package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/projects"
)

// discoverProjects lists devspace folders that look like projects (git repo or package manifest),
// marking the ones already adopted. Read-only on the devspace.
func (a *API) discoverProjects(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if v, err := a.liveViews(r, t); err != nil || a.findWorkspace(r, v) == nil {
		auth.Problem(w, http.StatusNotFound, "Devspace not found.")
		return
	}
	ws := r.PathValue("workspaceId")
	items, err := a.host.Discover(r.Context(), t.ID, ws)
	if err != nil {
		writeJSON(w, map[string]any{"items": []any{}, "note": "Start the devspace to find projects in it."})
		return
	}
	adopted, _ := (&projects.Store{Pool: a.db.Pool}).AdoptedPaths(r.Context(), t.ID)
	out := make([]map[string]any, 0, len(items))
	for _, f := range items {
		out = append(out, map[string]any{"name": f.Name, "path": f.Path, "git": f.Git, "remote": f.Remote,
			"manifests": f.Manifests, "slug": projects.SlugFromSource(f.Path), "adopted": adopted[f.Path] != "", "project_id": adopted[f.Path]})
	}
	writeJSON(w, map[string]any{"items": out})
}

// adoptProject turns one suggested folder into a project owned by the caller (one click; no name asked).
func (a *API) adoptProject(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "Adding projects needs the org owner or admin role.")
		return
	}
	var in struct {
		Path string `json:"path"`
	}
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil || in.Path == "" {
		auth.Problem(w, http.StatusBadRequest, "Pick a folder to adopt.")
		return
	}
	if v, err := a.liveViews(r, t); err != nil || a.findWorkspace(r, v) == nil {
		auth.Problem(w, http.StatusNotFound, "Devspace not found.")
		return
	}
	ws := r.PathValue("workspaceId")
	a.idempotent(w, r, t.ID+":"+c.id.Sub, func(w http.ResponseWriter, r *http.Request) {
		f, err := a.pickFound(r.Context(), t.ID, ws, in.Path)
		if err != nil {
			auth.Problem(w, http.StatusNotFound, "That folder isn't in your devspace any more.")
			return
		}
		a.finishOwned(w, r, ownedReq{TenantID: t.ID, WorkspaceID: ws, OwnerSub: c.id.Sub, By: c.id.Sub, Found: f, RequireDevspaceOwner: true},
			db.AuditEntry{TenantID: t.ID, TenantName: t.Name, WorkspaceID: ws, Actor: c.id.Sub, ActorName: c.id.Name,
				Action: "project.adopt", IP: clientIP(r)})
	})
}

// finishOwned runs createOwned, maps its errors, audits success and replies 201.
func (a *API) finishOwned(w http.ResponseWriter, r *http.Request, in ownedReq, ae db.AuditEntry) {
	res, err := a.createOwned(r.Context(), in)
	switch {
	case err == nil:
	case errors.Is(err, projects.ErrNameTaken):
		auth.Problem(w, http.StatusConflict, "That folder is already a project.")
		return
	case errors.Is(err, projects.ErrNoOrg), errors.Is(err, projects.ErrWorkspaceTaken), errors.Is(err, projects.ErrPlacement):
		auth.Problem(w, http.StatusConflict, err.Error())
		return
	case errors.Is(err, projects.ErrNotDevspaceOwner), errors.Is(err, errNotOrgMember):
		auth.Problem(w, http.StatusForbidden, err.Error())
		return
	default:
		auth.Problem(w, http.StatusBadGateway, "The project couldn't be created; nothing was kept. Try again.")
		return
	}
	ae.Detail = res.Slug + " " + res.Project.PublicID + " " + in.Found.Path + " rrflow=" + res.RRFlow
	_ = a.store().AddAudit(r.Context(), ae)
	writeStatus(w, http.StatusCreated, res)
}
