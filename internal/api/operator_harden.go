package api

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// opHardenInner applies the pj- restriction set to a tenant VM's existing inner projects
// (P0-1). ?restart=1 restarts running workspaces that still use the shared idmap; without it
// those projects are reported pending and left unrestricted until their next restart.
func (a *API) opHardenInner(w http.ResponseWriter, r *http.Request, c *caller) {
	t, ok := a.opTenant(w, r)
	if !ok {
		return
	}
	restart := r.URL.Query().Get("restart") == "1"
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	res, err := a.host.HardenInner(ctx, t.ID, t.Plan, restart)
	if err != nil {
		auth.Problem(w, http.StatusBadGateway, "Hardening failed: "+err.Error())
		return
	}
	restricted := 0
	for _, p := range res {
		if p.Restricted {
			restricted++
		}
	}
	detail := fmt.Sprintf("%d/%d inner projects restricted, restart=%v", restricted, len(res), restart)
	log.Printf("operator %s: harden tenant %s: %s", c.id.Sub, t.ID, detail)
	e := db.AuditEntry{TenantID: t.ID, Actor: c.id.Sub, ActorName: displayName(c), Action: "tenant.vm.harden",
		Detail: detail, IP: clientIP(r)}
	if err := a.store().AddAudit(r.Context(), e); err != nil {
		log.Printf("audit tenant.vm.harden for %s: %v", t.ID, err)
	}
	writeJSON(w, map[string]any{"projects": res})
}
