package api

import (
	"encoding/json"
	"net/http"
	"regexp"

	"shippin.cloud/console/internal/auth"
)

var orgIDRe = regexp.MustCompile(`^[0-9]{6,32}$`)

// createTenant maps an existing Nomen org to a new tenant UUID and provisions its silo.
// Platform admin only. Re-posting for a mapped org re-runs idempotent provisioning.
func (a *API) createTenant(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	if !a.can(r, c, "tenants.lifecycle") {
		a.auditOnce(r, c, "authz.denied", "tenants.lifecycle", "refused")
		auth.Problem(w, http.StatusForbidden, "Creating tenants needs the tenants.lifecycle permission.")
		return
	}
	var in struct {
		IdpOrgID string `json:"idp_org_id"`
		Name     string `json:"name"`
		Plan     string `json:"plan_id"`
	}
	if json.NewDecoder(r.Body).Decode(&in) != nil || !orgIDRe.MatchString(in.IdpOrgID) || in.Name == "" || len(in.Name) > 64 {
		auth.Problem(w, http.StatusBadRequest, "idp_org_id and name are required.")
		return
	}
	if in.Plan == "" {
		in.Plan = a.cfg.Workspaces.DefaultPlan
	}
	t, err := a.ensureTenant(r.Context(), in.IdpOrgID, in.Name, in.Plan)
	if t == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The tenant store is unavailable.")
		return
	}
	if err != nil {
		a.provisionProblem(w, c, err)
		return
	}
	writeStatus(w, http.StatusCreated, t)
}
