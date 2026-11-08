package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"regexp"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/orgs"
)

var orgIDRe = regexp.MustCompile(`^[0-9]{6,32}$`)

// createTenant creates an explicitly owned local customer in draft, or maps an
// existing external organization and requests provisioning. Both require an
// authorized staff session; local creation does not allocate infrastructure.
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
	if !a.staffSessionGate(w, r, c, "tenants.lifecycle") {
		return
	}
	var in struct {
		IdpOrgID    string `json:"idp_org_id"`
		Name        string `json:"name"`
		Plan        string `json:"plan_id"`
		OwnerUserID string `json:"owner_user_id"`
	}
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, 16<<10))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&in) != nil || in.Name == "" || len(in.Name) > 120 || (in.OwnerUserID == "") == (in.IdpOrgID == "") {
		auth.Problem(w, http.StatusBadRequest, "Provide a name and exactly one of owner_user_id or idp_org_id.")
		return
	}
	if in.Plan == "" {
		in.Plan = a.cfg.Workspaces.DefaultPlan
	}
	if in.OwnerUserID != "" {
		a.idempotent(w, r, c.id.Sub+":organization", func(w http.ResponseWriter, r *http.Request) {
			id, err := a.orgStore().CreateLocalCustomer(r.Context(), in.Name, in.Plan, in.OwnerUserID, c.id.Sub)
			if errors.Is(err, orgs.ErrBadInput) || errors.Is(err, orgs.ErrNotFound) {
				auth.Problem(w, http.StatusBadRequest, "Choose an active local owner, an active plan and a valid organization name.")
				return
			}
			if err != nil {
				auth.Problem(w, http.StatusServiceUnavailable, "The organization could not be saved.")
				return
			}
			t, err := a.db.TenantByID(r.Context(), id)
			if err != nil {
				auth.Problem(w, http.StatusServiceUnavailable, "The saved organization could not be read.")
				return
			}
			writeStatus(w, http.StatusCreated, t)
		})
		return
	}
	if !orgIDRe.MatchString(in.IdpOrgID) {
		auth.Problem(w, http.StatusBadRequest, "The external organization ID is invalid.")
		return
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
