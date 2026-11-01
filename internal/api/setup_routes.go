package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// Setup checklist API: tenants read their own; staff read any (tenants.read). POST .../rrflow
// provisions rrflow/state for every project that has a devspace but no instance (idempotent).
func (a *API) setupRoutes(mux Router) {
	mux.HandleFunc("GET /v1/tenants/{tenantId}/setup", a.flow.Require(a.tenantSetup))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/setup/rrflow", a.flow.Require(a.tenantSetupRRFlow))
}

func (a *API) writeSetup(w http.ResponseWriter, r *http.Request, t *db.Tenant) {
	checks, err := a.setupChecks(r.Context(), t)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Setup state is unavailable right now.")
		return
	}
	done := 0
	for _, c := range checks {
		if c.State == "ok" {
			done++
		}
	}
	writeJSON(w, map[string]any{"tenant_id": t.ID, "checks": checks, "done": done, "total": len(checks)})
}

func (a *API) tenantSetup(w http.ResponseWriter, r *http.Request) {
	_, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if !t.member() {
		auth.Problem(w, http.StatusForbidden, "Only members of this tenant can see its setup.")
		return
	}
	a.writeSetup(w, r, &t.Tenant)
}

func (a *API) opTenantSetup(w http.ResponseWriter, r *http.Request, _ *caller) {
	if t, ok := a.opTenant(w, r); ok {
		a.writeSetup(w, r, t)
	}
}

func (a *API) tenantSetupRRFlow(w http.ResponseWriter, r *http.Request) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if !a.cfg.Services.RRFlow.Allows(t.ID) {
		auth.Problem(w, http.StatusForbidden, "rrflow isn't available in this tenant yet.")
		return
	}
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "Provisioning rrflow needs the org owner or admin role.")
		return
	}
	a.setupRRFlow(w, r, &t.Tenant, c.id.Sub, c.id.Name)
}
