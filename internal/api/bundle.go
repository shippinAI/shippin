package api

import (
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// Bundled Stack per tenant (Notion 3bea94d3-02db-813e-9eee-e47fa7f919ae). GET shows each
// component's real state for this tenant; POST reconcile (tenant managers) provisions what the
// platform can provide today. Planned components are reported as not available, never faked.
type bundleItem struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Role   string `json:"role"`
	Kind   string `json:"kind"`
	State  string `json:"state"` // ready | missing | not_available
	Detail string `json:"detail,omitempty"`
}

func (a *API) bundleRoutes(mux Router) {
	mux.HandleFunc("GET /v1/tenants/{tenantId}/bundle", a.flow.Require(func(w http.ResponseWriter, r *http.Request) { a.bundle(w, r, false) }))
	mux.HandleFunc("POST /v1/tenants/{tenantId}/bundle/reconcile", a.flow.Require(func(w http.ResponseWriter, r *http.Request) { a.bundle(w, r, true) }))
}

func (a *API) bundle(w http.ResponseWriter, r *http.Request, reconcile bool) {
	c, t, ok := a.tenantCtx(w, r)
	if !ok {
		return
	}
	if reconcile {
		if !c.canManage(t) {
			auth.Problem(w, http.StatusForbidden, "Only tenant managers can reconcile the bundle.")
			return
		}
		e := db.AuditEntry{TenantID: t.ID, TenantName: t.Name, Actor: c.id.Sub, ActorName: c.id.Name, Action: "bundle.reconcile",
			Detail: "release " + a.cfg.Bundle.Release, IP: clientIP(r)}
		if !a.auditFirst(w, r, e) {
			return
		}
	}
	out := []bundleItem{}
	for _, x := range a.cfg.Bundle.Components {
		it := bundleItem{ID: x.ID, Name: x.Name, Role: x.Role, Kind: x.Kind, State: "not_available", Detail: x.Notes}
		switch x.Kind {
		case "workspace-image":
			it.State, it.Detail = "ready", "in every workspace from "+a.cfg.Bundle.Release
		case "nomen-org":
			it.State, it.Detail = "missing", "no Nomen organization yet"
			if t.IdpOrgID != "" {
				it.State, it.Detail = "ready", "Nomen organization "+t.IdpOrgID
			}
		case "starter-db":
			it.State, it.Detail = "missing", "reconcile to create"
			if a.host != nil { // idempotent ensure: also the status check
				if s, err := a.host.EnsureStarterDB(r.Context(), t.ID); err == nil {
					it.State, it.Detail = "ready", "database "+s.Database+" on "+s.Addr
				} else {
					it.Detail = "provisioning failed; try again"
				}
			}
		}
		out = append(out, it)
	}
	writeJSON(w, map[string]any{"release": a.cfg.Bundle.Release, "items": out})
}
