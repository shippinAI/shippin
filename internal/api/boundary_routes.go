package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/projects"
)

// boundaryRoutes is the operator-only, read-only view of boundary VMs (users never see them).
func (a *API) boundaryRoutes() []opRoute {
	return []opRoute{{"GET /v1/operator/boundaries", "tenants.read", func(w http.ResponseWriter, r *http.Request, _ *caller) {
		out, err := (&projects.Store{Pool: a.db.Pool}).Boundaries(r.Context())
		if err != nil {
			auth.Problem(w, http.StatusInternalServerError, "Boundaries are unavailable right now.")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Cache-Control", "no-store")
		_, _ = w.Write(out)
	}}}
}
