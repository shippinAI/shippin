package api

import (
	"net/http"
	"shippin.cloud/console/internal/runtime/devspace"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/ports"
)

type opWorkspace struct {
	devspace.View
	TenantName   string `json:"tenant_name"`
	TenantStatus string `json:"tenant_status"`
}

func (a *API) opWorkspaces(w http.ResponseWriter, r *http.Request, _ *caller) {
	ts, err := a.db.TenantsByOrgs(r.Context(), nil)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The tenant store is unavailable.")
		return
	}
	wss, err := devspace.On(a.db.Pool).All(r.Context())
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "The workspace store is unavailable.")
		return
	}
	names := map[string]string{}
	if users, err := a.idpUsers(r.Context()); err == nil {
		for _, u := range users {
			names[u.ID] = u.Login
		}
	}
	out := []opWorkspace{}
	for _, t := range ts {
		var live map[string]ports.Guest
		if t.Status != "deleted" {
			if guests, complete := (guestPort{a}).Inventory(r.Context(), t.ID); complete {
				live = guests
			}
		}
		for _, ws := range wss {
			if ws.TenantID == t.ID {
				if n := names[ws.CreatedBy]; n != "" {
					ws.CreatedBy = n
				}
				out = append(out, opWorkspace{a.devspaces().Show(orgOf(t), ws, live), t.Name, t.Status})
			}
		}
	}
	writeJSON(w, map[string]any{"items": out})
}
