package api

import "net/http"

func (a *API) tenant(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	if !ok {
		return
	}
	out := *t
	if c.op == "" {
		out.Error = "" // raw provisioning errors are for operators only
	}
	writeJSON(w, out)
}

func (a *API) tenantSettings(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	t, ok := c.tenant(w, r.PathValue("tenantId"))
	if !ok {
		return
	}
	writeJSON(w, map[string]any{
		"tenant_id": t.ID, "idp_org_id": t.IdpOrgID, "display_name": t.Name, "plan_name": t.Plan,
		"status": t.Status, "default_domain": a.cfg.Domain, "governance_domains": a.cfg.GovernanceDomains,
		"idp_issuer": a.cfg.IDP.Issuer, "region": a.cfg.Incus.HostLabel,
		"incus_project": "t-" + t.ID, "ssh_ip": t.SSHIP,
	})
}
