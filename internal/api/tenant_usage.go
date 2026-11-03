package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
)

// getUsage: per-project CPU, memory, disk, egress and cost for the tenant (Incus metrics via hostd).
// Any tenant member may read it: it is the tenant's own bill.
func (a *API) getUsage(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	u, err := a.host.Metrics(r.Context(), t.ID)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Usage is not available right now.")
		return
	}
	if u.Projects == nil {
		writeJSON(w, map[string]any{"projects": []any{}, "rates": u.Rates})
		return
	}
	writeJSON(w, u)
}

// panelRoutes: one-click Publish (workspace_site.go) and per-project usage.
func (a *API) panelRoutes(mux Router) {
	a.siteRoutes(mux)
	mux.HandleFunc("GET /v1/tenants/{tenantId}/usage", a.flow.Require(a.getUsage))
}
