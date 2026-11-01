package api

import (
	"context"
	"log"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// stopAll stops every live workspace of a tenant (used by suspend). Returns the ids that failed.
func (a *API) stopAll(ctx context.Context, t *db.Tenant) []string {
	wss, err := a.db.Workspaces(ctx, t.ID)
	if err != nil {
		return []string{"(workspace list unavailable)"}
	}
	tv, failed := &tenantView{Tenant: *t}, []string{}
	for _, ws := range wss {
		if ws.State == "stopped" {
			continue
		}
		if err := a.host.Action(ctx, t.ID, ws.ID, "stop", a.hostReq(tv, ws)); err != nil {
			log.Printf("suspend %s: stop %s: %v", t.ID, ws.ID, err)
			_ = a.db.SetWorkspaceState(ctx, ws.ID, "failed", "", "Could not stop for suspension: "+err.Error())
			failed = append(failed, ws.ID)
			continue
		}
		_ = a.db.SetWorkspaceState(ctx, ws.ID, "stopped", "", "")
	}
	return failed
}

// opSuspend: suspend flips the status first (so no new starts are accepted), then stops every
// workspace and reports any that failed; resume only flips the status (workspaces stay stopped).
func (a *API) opSuspend(suspend bool) func(http.ResponseWriter, *http.Request, *caller) {
	return func(w http.ResponseWriter, r *http.Request, c *caller) {
		t, ok := a.opTenant(w, r)
		if !ok {
			return
		}
		want, from := "suspended", "active"
		if !suspend {
			want, from = "active", "suspended"
		}
		action := "tenant.suspend"
		if !suspend {
			action = "tenant.resume"
		}
		if !a.auditOperator(w, r, c, t.ID, action, t.Status+" -> "+want) {
			return
		}
		won, err := a.db.BeginTenantOp(r.Context(), t.ID, want, from)
		if err != nil {
			auth.Problem(w, http.StatusServiceUnavailable, "The tenant store is unavailable.")
			return
		}
		if !won {
			auth.Problem(w, http.StatusConflict, "Tenant is "+t.Status+", expected "+from+".")
			return
		}
		log.Printf("operator %s: tenant %s -> %s", c.id.Sub, t.ID, want)
		res := map[string]any{"tenant_id": t.ID, "status": want}
		if suspend {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
			defer cancel()
			if failed := a.stopAll(ctx, t); len(failed) > 0 {
				res["stop_failed"] = failed
			}
		}
		writeJSON(w, res)
	}
}
