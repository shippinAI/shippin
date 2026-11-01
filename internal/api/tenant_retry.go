package api

import (
	"errors"
	"log"
	"net/http"

	"shippin.cloud/console/internal/auth"
)

// customerProvisionError is what non-operators see; operators get the raw cause.
const customerProvisionError = "We couldn't finish setting up your organization's resources. Please try again in a few minutes; if it keeps failing, contact support."

func (a *API) provisionProblem(w http.ResponseWriter, c *caller, err error) {
	if errors.Is(err, errTenantLocked) {
		auth.Problem(w, http.StatusConflict, "This organization is suspended or deleted. Contact support.")
		return
	}
	if c.op != "" {
		auth.Problem(w, http.StatusBadGateway, "Provisioning failed: "+err.Error())
		return
	}
	auth.Problem(w, http.StatusBadGateway, customerProvisionError)
}

// retryProvision re-runs idempotent provisioning for a tenant (owner/admin or operator).
func (a *API) retryProvision(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	if !ok {
		return
	}
	if !c.canDecide(t) {
		auth.Problem(w, http.StatusForbidden, "Retrying setup needs the owner or admin role.")
		return
	}
	nt, err := a.ensureTenant(r.Context(), t.IdpOrgID, t.Name, t.Plan)
	if err != nil {
		log.Printf("retry provision %s: %v", t.ID, err)
		a.provisionProblem(w, c, err)
		return
	}
	writeJSON(w, map[string]string{"tenant_id": nt.ID, "status": nt.Status})
}

// listTenants: operator view of every tenant with status and raw error.
func (a *API) listTenants(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	if c.op == "" {
		auth.Problem(w, http.StatusForbidden, "Operators only.")
		return
	}
	ts, err := a.db.TenantsByOrgs(r.Context(), nil)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The tenant store is unavailable.")
		return
	}
	writeJSON(w, map[string]any{"items": ts})
}
