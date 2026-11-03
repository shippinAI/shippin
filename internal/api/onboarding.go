package api

import (
	"encoding/json"
	"log"
	"net/http"
	"slices"
	"strings"
	"sync"
	"unicode/utf8"

	"shippin.cloud/console/internal/auth"
)

var onboardingLocks sync.Map

// onboard: a signed-in user with no tenant (e.g. new Google/GitHub sign-up) creates their own
// org in the IdP (they become ORG_OWNER) and its tenant via the normal provisioning path.
// Idempotent per user: an org they already own without a tenant is reused.
func (a *API) onboard(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	if !a.cfg.Onboarding.Enabled || a.cfg.IDPServiceToken == "" {
		auth.Problem(w, http.StatusForbidden, "Self-service sign-up is not open yet. Ask for an invite.")
		return
	}
	if len(c.tenants) > 0 || c.op != "" {
		auth.Problem(w, http.StatusConflict, "You already belong to an organization.")
		return
	}
	var in struct {
		Name string `json:"name"`
	}
	_ = json.NewDecoder(http.MaxBytesReader(w, r.Body, 4<<10)).Decode(&in)
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 2 || n > 48 {
		auth.Problem(w, http.StatusBadRequest, "Use 2 to 48 characters for the organization name.")
		return
	}
	if _, busy := onboardingLocks.LoadOrStore(c.id.Sub, true); busy {
		auth.Problem(w, http.StatusConflict, "Your organization is already being set up.")
		return
	}
	defer onboardingLocks.Delete(c.id.Sub)
	orgID := ""
	for _, m := range c.id.Memberships {
		if !m.IAM && m.OrgID != c.id.OrgID && slices.Contains(m.Roles, "ORG_OWNER") {
			orgID = m.OrgID
		}
	}
	if orgID == "" {
		id, err := a.flow.CreateOrg(r.Context(), a.cfg.IDPServiceToken, in.Name, c.id.Sub)
		if err != nil {
			log.Printf("onboarding: create org for %s: %v", c.id.Sub, err)
			auth.Problem(w, http.StatusBadGateway, "We couldn't create your organization. Try again in a minute.")
			return
		}
		orgID = id
	}
	plan := a.cfg.Onboarding.Plan
	if plan == "" {
		plan = a.cfg.Workspaces.DefaultPlan
	}
	t, err := a.ensureTenant(r.Context(), orgID, in.Name, plan)
	a.flow.Forget(a.flow.SessionToken(r))
	a.recordOwner(r.Context(), c, t)
	if err != nil {
		log.Printf("onboarding: provision org %s: %v", orgID, err)
		a.provisionProblem(w, c, err)
		return
	}
	writeStatus(w, http.StatusCreated, map[string]string{"tenant_id": t.ID, "name": t.Name, "status": t.Status})
}
