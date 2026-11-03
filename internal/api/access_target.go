package api

import (
	"encoding/json"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// decodeAccess reads an access/break-glass body (small, strict).
func decodeAccess(w http.ResponseWriter, r *http.Request) (*accessIn, bool) {
	in := &accessIn{}
	dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
	dec.DisallowUnknownFields()
	if dec.Decode(in) != nil {
		auth.Problem(w, http.StatusBadRequest, "Invalid request body.")
		return nil, false
	}
	return in, true
}

// accessTarget loads the tenant (and workspace) an admin asks for. Deleted tenants are refused;
// the admin must not already be a member (members don't need grants).
func (a *API) accessTarget(w http.ResponseWriter, r *http.Request, c *caller, in *accessIn) (*db.Tenant, string, bool) {
	t, err := a.db.TenantByID(r.Context(), in.TenantID)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The tenant store is unavailable.")
		return nil, "", false
	}
	if t == nil || t.Status == "deleting" || t.Status == "deleted" {
		auth.Problem(w, http.StatusNotFound, "Tenant not found.")
		return nil, "", false
	}
	if v := c.grantedTenant(t.ID); v != nil && v.member() {
		auth.Problem(w, http.StatusConflict, "You're a member of this organization; no grant is needed.")
		return nil, "", false
	}
	scope := "whole tenant"
	if in.WorkspaceID != "" {
		ws, err := a.db.WorkspaceByID(r.Context(), in.WorkspaceID)
		if err != nil || ws == nil || ws.TenantID != t.ID {
			auth.Problem(w, http.StatusNotFound, "Workspace not found in this tenant.")
			return nil, "", false
		}
		scope = "workspace " + ws.Name
	}
	return t, scope, true
}

// displayName is how an admin is shown to customers (never empty).
func displayName(c *caller) string {
	for _, s := range []string{c.id.Name, c.id.Email} {
		if s != "" {
			return s
		}
	}
	return c.id.Sub
}

// present rewrites approved-but-past-expiry grants as expired for display.
func present(gs []db.Grant) []db.Grant {
	now := time.Now()
	for i := range gs {
		gs[i].Status = gs[i].Effective(now)
	}
	return gs
}
