package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/orgs"
)

// orgRoutes (CP-P0b): the org switcher list, the staff-only Operations views and support grants.
func (a *API) orgRoutes(mux Router) {
	mux.HandleFunc("GET /v1/orgs", a.flow.Require(a.listOrgs))
	mux.HandleFunc("GET /v1/ops/servers", a.flow.Require(a.opsGate(false, a.opsServers)))
	mux.HandleFunc("GET /v1/ops/support-grants", a.flow.Require(a.opsGate(false, a.listSupport)))
	mux.HandleFunc("POST /v1/ops/support-grants", a.flow.Require(a.opsGate(true, a.grantSupport)))
	mux.HandleFunc("POST /v1/ops/support-grants/{id}/revoke", a.flow.Require(a.opsGate(true, a.revokeSupport)))
}

type orgItem struct {
	orgs.Org
	Roles    []string `json:"roles"`
	Devspace bool     `json:"has_devspace"`
	TenantID string   `json:"tenant_id,omitempty"` // customer orgs: where the switcher navigates
}

// listOrgs: Operations (staff only) plus one customer org per tenant membership; one Nomen identity.
func (a *API) listOrgs(w http.ResponseWriter, r *http.Request) {
	if !a.cfg.Orgs.Enabled {
		auth.Problem(w, http.StatusNotFound, "Orgs aren't enabled.")
		return
	}
	c, ok := a.resolve(w, r)
	if !ok {
		return
	}
	s := a.orgStore()
	roles, err := a.opsRoles(r, c)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Orgs are unavailable right now.")
		return
	}
	items := []orgItem{}
	if len(roles) > 0 {
		if o, err := s.Operations(r.Context()); err == nil {
			items = append(items, orgItem{Org: *o, Roles: roles})
		}
	}
	for _, t := range c.tenants {
		if t.IdpOrgID == "" || len(t.Roles) == 0 {
			continue // platform-grant-only visibility is not membership
		}
		o, err := s.EnsureCustomer(r.Context(), t.IdpOrgID, orgSlug(t.Name, t.ID), t.Name)
		if err != nil {
			auth.Problem(w, http.StatusServiceUnavailable, "Orgs are unavailable right now.")
			return
		}
		items = append(items, orgItem{Org: *o, Roles: t.Roles, Devspace: true, TenantID: t.ID})
	}
	writeJSON(w, map[string]any{"items": items, "staff_roles": roles})
}

func (a *API) opsServers(w http.ResponseWriter, r *http.Request, _ *caller, roles []string) {
	items, err := a.serverStore().ListAll(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Servers are unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"items": items, "can_admin": orgs.CanAdminPlatform(roles)})
}

func (a *API) listSupport(w http.ResponseWriter, r *http.Request, _ *caller, _ []string) {
	items, err := a.orgStore().SupportGrants(r.Context(), 200)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Support grants are unavailable right now.")
		return
	}
	writeJSON(w, map[string]any{"items": items, "max_seconds": a.cfg.Orgs.SupportGrantMaxSeconds})
}

type supportIn struct {
	StaffSub   string `json:"staff_sub"`
	OrgID      string `json:"org_id"`
	ProjectID  string `json:"project_id"`
	Reason     string `json:"reason"`
	TTLSeconds int    `json:"ttl_seconds"`
}

// grantSupport is audited before it is written (no audit row, no grant).
func (a *API) grantSupport(w http.ResponseWriter, r *http.Request, c *caller, _ []string) {
	var in supportIn
	if json.NewDecoder(http.MaxBytesReader(w, r.Body, 4096)).Decode(&in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Send staff_sub, org_id, reason and ttl_seconds.")
		return
	}
	e := db.AuditEntry{Actor: c.id.Sub, ActorName: displayName(c), Action: "ops.support_grant",
		Detail: in.StaffSub + " " + in.OrgID + " " + time.Duration(in.TTLSeconds*int(time.Second)).String(), IP: clientIP(r)}
	if !a.auditFirst(w, r, e) {
		return
	}
	max := time.Duration(a.cfg.Orgs.SupportGrantMaxSeconds) * time.Second
	g, err := a.orgStore().GrantSupport(r.Context(), in.StaffSub, in.OrgID, in.ProjectID, in.Reason, c.id.Sub,
		time.Duration(in.TTLSeconds)*time.Second, max)
	switch {
	case errors.Is(err, orgs.ErrTooLong):
		auth.Problem(w, http.StatusBadRequest, "Support grants are limited to "+max.String()+".")
	case errors.Is(err, orgs.ErrBadInput), errors.Is(err, orgs.ErrNotFound):
		auth.Problem(w, http.StatusBadRequest, "Pick a customer org, a staff member and a reason (10+ characters).")
	case err != nil:
		auth.Problem(w, http.StatusServiceUnavailable, "The grant couldn't be saved.")
	default:
		writeStatus(w, http.StatusCreated, g)
	}
}

func (a *API) revokeSupport(w http.ResponseWriter, r *http.Request, c *caller, _ []string) {
	id := r.PathValue("id")
	if !a.auditFirst(w, r, db.AuditEntry{Actor: c.id.Sub, ActorName: displayName(c), Action: "ops.support_revoke", Detail: id, IP: clientIP(r)}) {
		return
	}
	g, err := a.orgStore().RevokeSupport(r.Context(), id)
	if err != nil {
		auth.Problem(w, http.StatusNotFound, "No such grant.")
		return
	}
	writeJSON(w, g)
}
