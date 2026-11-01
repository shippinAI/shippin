package api

import (
	"context"
	"net/http"
	"strings"

	"shippin.cloud/console/internal/auth"
	"shippin.cloud/console/internal/db"
)

// setupRRFlow queues rrflow for every project devspace of t that still needs it (the Setup
// checklist's rrflow_attach fixes), audited first; callers have checked the rollout and the actor.
func (a *API) setupRRFlow(w http.ResponseWriter, r *http.Request, t *db.Tenant, actor, actorName string) {
	checks, err := a.setupChecks(r.Context(), t)
	if err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Setup state is unavailable right now.")
		return
	}
	ws, _ := a.db.Workspaces(r.Context(), t.ID)
	queued := []string{}
	for _, ch := range checks {
		if !strings.HasPrefix(ch.ID, "rrflow:") || ch.Fix != "rrflow_attach" {
			continue
		}
		for _, x := range ws {
			if x.ProjectID == ch.Project && x.State != "deleted" && x.State != "failed" {
				e := db.AuditEntry{TenantID: t.ID, TenantName: t.Name, Actor: actor, ActorName: actorName,
					Action: "rrflow.provision", Detail: "project " + ch.Project, IP: clientIP(r)}
				if !a.auditFirst(w, r, e) {
					return
				}
				if _, _, err := a.queueRRFlow(r.Context(), t.ID, ch.Project, x.ID, actor); err != nil {
					auth.Problem(w, http.StatusServiceUnavailable, "rrflow could not be provisioned right now.")
					return
				}
				queued = append(queued, ch.Project)
				break
			}
		}
	}
	writeStatus(w, http.StatusAccepted, map[string]any{"queued_projects": queued})
}

// opSetupRRFlow is the staff path for the same setup (a change ticket's executor through a staff
// session, never as the tenant's users): tenants.lifecycle, and only where the rollout allows.
func (a *API) opSetupRRFlow(w http.ResponseWriter, r *http.Request, c *caller) {
	t, err := a.db.TenantByID(r.Context(), r.PathValue("tenantId"))
	if err != nil || t == nil {
		auth.Problem(w, http.StatusNotFound, "No such tenant.")
		return
	}
	if !a.cfg.Services.RRFlow.Allows(t.ID) {
		auth.Problem(w, http.StatusForbidden, "rrflow isn't available in this tenant yet.")
		return
	}
	if a.svcs == nil {
		auth.Problem(w, http.StatusServiceUnavailable, "rrflow is off until migration 026 is applied.")
		return
	}
	// The per-tenant D36 move (026 is schema only): only here, only for this tenant, audited first.
	if legacy, err := a.svcs.LegacyPending(r.Context(), t.ID); err != nil {
		auth.Problem(w, http.StatusServiceUnavailable, "Setup state is unavailable right now.")
		return
	} else if legacy {
		e := db.AuditEntry{TenantID: t.ID, TenantName: t.Name, Actor: c.id.Sub, ActorName: displayName(c),
			Action: "rrflow.migrate", Detail: "per-project instances -> shared instance per tenant VM (D36)", IP: clientIP(r)}
		if !a.auditFirst(w, r, e) {
			return
		}
		if _, err := a.svcs.MigrateTenant(r.Context(), t.ID, c.id.Sub); err != nil {
			auth.Problem(w, http.StatusServiceUnavailable, "rrflow could not be moved right now.")
			return
		}
		if a.runner != nil {
			a.runner.Wake()
		}
	}
	a.setupRRFlow(w, r, t, c.id.Sub, displayName(c))
}

// rrflowVMAllowed: managed services are on and the tenant has no unmoved pre-D36 instance.
func (a *API) rrflowVMAllowed(ctx context.Context, tenant string) error {
	if a.svcs == nil {
		return errRRFlowHeld
	}
	if legacy, err := a.svcs.LegacyPending(ctx, tenant); err != nil {
		return err
	} else if legacy {
		return errRRFlowLegacy
	}
	return nil
}
