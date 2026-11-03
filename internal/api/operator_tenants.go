package api

import (
	"context"
	"log"
	"net/http"
	"slices"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

type opTenant struct {
	db.Tenant
	Owner      string `json:"owner"`
	OwnerEmail string `json:"owner_email"`
	Subnet     string `json:"subnet"`
	Router     string `json:"router"`
	Workspaces int    `json:"workspace_count"`
}

func (a *API) opTenants(w http.ResponseWriter, r *http.Request, _ *caller) {
	ts, err := a.db.TenantsByOrgs(r.Context(), nil)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The tenant store is unavailable.")
		return
	}
	wss, _ := a.db.AllWorkspaces(r.Context())
	count := map[string]int{}
	for _, ws := range wss {
		count[ws.TenantID]++
	}
	nets, nerr := a.host.TenantNetworks(r.Context())
	out := make([]opTenant, 0, len(ts))
	for _, t := range ts {
		o := opTenant{Tenant: t, Workspaces: count[t.ID]}
		if n := nets["t-"+t.ID]; n != nil {
			o.Subnet, o.Router = n["subnet"], n["router"]
		}
		o.Owner, o.OwnerEmail = a.orgOwner(r.Context(), t.IdpOrgID)
		out = append(out, o)
	}
	res := map[string]any{"items": out}
	if nerr != nil && a.cfg.Incus.HostdURL != "" {
		res["warning"] = "Host networks unavailable: " + nerr.Error()
	}
	writeJSON(w, res)
}

// orgOwner returns the first ORG_OWNER of a Nomen org (display name, login/email).
func (a *API) orgOwner(ctx context.Context, org string) (string, string) {
	var res struct {
		Result []struct {
			DisplayName        string   `json:"displayName"`
			Email              string   `json:"email"`
			PreferredLoginName string   `json:"preferredLoginName"`
			Roles              []string `json:"roles"`
		} `json:"result"`
	}
	if err := a.flow.IdPRead(ctx, a.cfg.IDPReadToken, "POST", "/management/v1/orgs/me/members/_search", org, map[string]any{}, &res); err != nil {
		return "", ""
	}
	for _, m := range res.Result {
		if slices.Contains(m.Roles, "ORG_OWNER") {
			mail := m.Email
			if mail == "" {
				mail = m.PreferredLoginName
			}
			return m.DisplayName, mail
		}
	}
	return "", ""
}

func (a *API) opTenant(w http.ResponseWriter, r *http.Request) (*db.Tenant, bool) {
	t, err := a.db.TenantByID(r.Context(), r.PathValue("tenantId"))
	if err != nil || t == nil {
		auth.Problem(w, http.StatusNotFound, "Tenant not found.")
		return nil, false
	}
	return t, true
}

func (a *API) opRetry(w http.ResponseWriter, r *http.Request, c *caller) {
	t, ok := a.opTenant(w, r)
	if !ok {
		return
	}
	if t.Status == "deleted" || t.Status == "suspended" {
		auth.Problem(w, http.StatusConflict, "Resume the tenant before re-running provisioning.")
		return
	}
	if !a.auditOperator(w, r, c, t.ID, "tenant.provision_retry", t.Status) {
		return
	}
	nt, err := a.ensureTenant(r.Context(), t.IdpOrgID, t.Name, t.Plan)
	if err != nil {
		log.Printf("operator retry %s: %v", t.ID, err)
		a.provisionProblem(w, c, err)
		return
	}
	writeJSON(w, map[string]string{"tenant_id": nt.ID, "status": nt.Status})
}
