package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/projects"
)

// projectRoutes mounts the M1 projects API on tenant membership: any member reads, only an org
// owner/admin (canDecide) writes. Boundaries never appear in these responses.
func (a *API) projectRoutes(mux Router) {
	for i, rt := range (&projects.Handler{}).Routes() {
		write := rt.Pattern[:4] != "GET "
		mux.HandleFunc(rt.Pattern, a.flow.Require(func(w http.ResponseWriter, r *http.Request) {
			c, t, ok := a.tenantCtx(w, r)
			if !ok {
				return
			}
			if !t.member() && t.grantFor("") == nil {
				auth.Problem(w, http.StatusForbidden, "A devspace support grant does not grant access to organization projects or workspace groups.")
				return
			}
			if write && !c.canDecide(t) {
				auth.Problem(w, http.StatusForbidden, "Changing projects needs the org owner or admin role.")
				return
			}
			h := &projects.Handler{S: &projects.Store{Pool: a.db.Pool}}
			run := func(w http.ResponseWriter, r *http.Request) { h.Routes()[i].H(w, r, t.ID, c.id.Sub) }
			if rt.Pattern[:5] == "POST " {
				a.idempotent(w, r, t.ID+":"+c.id.Sub, run)
				return
			}
			run(w, r)
		}))
	}
}

// inProject places a new workspace in project/env (both empty = legacy tenant placement).
func (a *API) inProject(w http.ResponseWriter, r *http.Request, tenant, project, env string, ws *db.Workspace) bool {
	if project == "" {
		return true
	}
	if env == "" {
		env = "dev"
	}
	p, err := (&projects.Store{Pool: a.db.Pool}).Place(r.Context(), tenant, project, env)
	if err != nil {
		auth.Problem(w, http.StatusConflict, "That project or environment isn't ready.")
		return false
	}
	ws.ProjectID, ws.EnvironmentID = project, p.EnvID
	if p.Boundary != nil {
		ws.BoundaryID = *p.Boundary
	}
	return true
}
