package api

import (
	"net/http"

	"shippin.cloud/console/internal/runtime/devspace"
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

// liveViews are the organization's development spaces the caller sees, for the routes on one space
// that have not moved into internal/runtime/devspace yet.
func (a *API) liveViews(r *http.Request, t *tenantView) ([]devspace.View, error) {
	return a.devspaces().Live(r.Context(), orgOf(t.Tenant), t.sees)
}
