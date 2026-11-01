package api

import (
	"context"
	"log"
	"slices"
	"strings"
	"sync"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
)

// driftMemo remembers the last drift reported per tenant so the audit log gets changes only.
type driftMemo struct {
	mu   sync.Mutex
	last map[string]string
}

// changed records detail for tenant and reports whether it differs from the last report.
func (m *driftMemo) changed(tenant, detail string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.last == nil {
		m.last = map[string]string{}
	}
	if m.last[tenant] == detail {
		return false
	}
	m.last[tenant] = detail
	return detail != ""
}

// checkDrift compares each active tenant's workspace rows with hostd's inventory and audits
// "inventory.drift": rows whose instance is missing, and labelled instances with no row.
// It never changes anything; a partial inventory (tenant VM unreachable) is skipped.
func (a *API) checkDrift(ctx context.Context) {
	ts, err := a.db.TenantsByOrgs(ctx, nil)
	if err != nil {
		log.Printf("drift: tenants: %v", err)
		return
	}
	for _, t := range ts {
		if t.Status != "active" {
			continue
		}
		inv, partial, err := a.host.TenantInventory(ctx, t.ID)
		if err != nil || partial {
			continue
		}
		ws, err := a.db.Workspaces(ctx, t.ID)
		if err != nil {
			continue
		}
		detail := driftDetail(ws, inv)
		if a.drift.changed(t.ID, detail) {
			_ = a.db.AddAudit(ctx, db.AuditEntry{TenantID: t.ID, Actor: reconcilerActor, ActorName: "reconciler",
				Action: "inventory.drift", Detail: detail})
		}
	}
}

// driftDetail is "" when rows and instances agree, else a stable description of the difference.
func driftDetail(ws []db.Workspace, inv []hostclient.Instance) string {
	have := map[string]bool{}
	for _, i := range inv {
		if id := i.Config["user.shippin.workspace"]; id != "" {
			have[id] = true
		}
	}
	known := map[string]bool{}
	var missing, orphans []string
	for _, w := range ws {
		known[w.ID] = true
		if (w.State == "running" || w.State == "stopped") && !have[w.ID] {
			missing = append(missing, "w-"+w.ID)
		}
	}
	for id := range have {
		if !known[id] {
			orphans = append(orphans, "w-"+id)
		}
	}
	slices.Sort(orphans)
	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "missing instances: "+strings.Join(missing, ", "))
	}
	if len(orphans) > 0 {
		parts = append(parts, "instances without a workspace: "+strings.Join(orphans, ", "))
	}
	return strings.Join(parts, "; ")
}
