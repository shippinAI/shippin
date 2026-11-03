package api

import (
	"encoding/json"
	"net/http"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// putAccess updates firewall rules or custom domains (field = "firewall" | "domains").
func (a *API) putAccess(field string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		c, t, ok := a.tenantCtx(w, r)
		if !ok {
			return
		}
		items, err := a.liveViews(r, t)
		ws := a.findWorkspace(r, items)
		if err != nil || ws == nil {
			auth.Problem(w, http.StatusNotFound, "Workspace not found.")
			return
		}
		if !c.canManageWS(t, ws.ID) {
			auth.Problem(w, http.StatusForbidden, "This needs the org owner or admin role.")
			return
		}
		var in struct {
			Rules   []db.Rule `json:"rules"`
			Domains []string  `json:"domains"`
		}
		if json.NewDecoder(http.MaxBytesReader(w, r.Body, 32<<10)).Decode(&in) != nil {
			auth.Problem(w, http.StatusBadRequest, "Invalid request body.")
			return
		}
		next := ws.Workspace
		if field == "firewall" {
			if msg := checkRules(in.Rules, a.cfg.Workspaces); msg != "" {
				auth.Problem(w, http.StatusBadRequest, msg)
				return
			}
			next.Firewall = in.Rules
		} else {
			if msg := checkDomains(in.Domains, a.cfg.Workspaces); msg != "" {
				auth.Problem(w, http.StatusBadRequest, msg)
				return
			}
			for _, d := range in.Domains {
				if o, err := a.db.WorkspaceByHost(r.Context(), d); err != nil || (o != nil && o.ID != ws.ID) {
					auth.Problem(w, http.StatusConflict, d+" is already attached to another workspace.")
					return
				}
			}
			next.Domains = in.Domains
		}
		if !a.auditUse(r, c, t, ws.ID, "change", field) {
			refuseUnaudited(w)
			return
		}
		if err := a.host.Action(r.Context(), t.ID, ws.ID, field, a.hostReq(t, next)); err != nil {
			auth.Problem(w, http.StatusConflict, "Couldn't apply the change: "+err.Error())
			return
		}
		_ = a.db.SetWorkspaceAccess(r.Context(), ws.ID, "", next.Domains, next.Firewall)
		writeJSON(w, map[string]any{"firewall": next.Firewall, "domains": next.Domains})
	}
}
