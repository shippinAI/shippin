package api

import (
	"net/http"
	"strings"
	"time"
)

func (a *API) me(w http.ResponseWriter, r *http.Request) {
	c, ok := a.caller(w, r)
	if !ok {
		return
	}
	type ref struct {
		TenantID string   `json:"tenant_id"`
		Name     string   `json:"name"`
		Roles    []string `json:"roles"`
		Status   string   `json:"status"`
		Manage   bool     `json:"can_manage"`
		// Access is set when the tenant is visible only through a platform access grant.
		Access *grantRef `json:"access,omitempty"`
	}
	tenants := []ref{}
	for i := range c.tenants {
		t := &c.tenants[i]
		tenants = append(tenants, ref{t.ID, t.Name, t.Roles, t.Status, c.canManage(t), t.accessRef(time.Now())})
	}
	platform := []string{}
	if c.op != "" {
		platform = append(platform, c.op)
	}
	writeJSON(w, map[string]any{
		"user_id": c.id.Sub, "display_name": c.id.Name, "email": c.id.Email,
		"platform_roles": platform, "tenants": tenants,
		"capabilities": a.capabilities(r, c), "staff": a.staffSummary(r, c),
	})
}

// capabilities is what the UI may show. provider.panel (the "Admin panel" switch) and
// tenants.create come from the same decision the API gates use; /admin and /v1/operator/*
// enforce it again per route.
func (a *API) capabilities(r *http.Request, c *caller) map[string]bool {
	create, _ := a.decide(r.Context(), c, "tenants.lifecycle")
	tools, _ := a.decide(r.Context(), c, "host.admin_tools")
	ops := false
	if a.cfg.Orgs.Enabled {
		roles, _ := a.opsRoles(r, c)
		ops = len(roles) > 0 // staff land on Operations even with no devspace
	}
	out := map[string]bool{"operator.view": c.op != "", "tenants.create": create, "ops.view": ops,
		"host.admin_tools": tools,
		"infra.hosts":      a.infraCan(r.Context(), c, "infra.read"),
		"provider.panel":   a.panelAllowed(r, c),
		"onboarding":       len(c.tenants) == 0 && c.op == "" && !ops && a.cfg.Onboarding.Enabled && a.cfg.IDPServiceToken != ""}
	for _, p := range permCatalog {
		if p.Section == "Infrastructure" {
			out[p.Key] = a.infraCan(r.Context(), c, p.Key)
		}
	}
	out["account.local"] = strings.HasPrefix(c.id.Sub, localSubPrefix)
	for k, on := range a.features() {
		out[k] = on
	}
	out["feature.external_review"] = a.externalReviewConnected(r.Context())
	return out
}
