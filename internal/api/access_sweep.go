package api

import (
	"context"
	"log"
	"time"

	"shippin.cloud/console/internal/db"
)

// RunSweeper closes expired grants and writes their expire/end audit entries until ctx ends.
// Enforcement never waits for it: every check compares expires_at with now.
func (a *API) RunSweeper(ctx context.Context) {
	t := time.NewTicker(time.Duration(a.cfg.Access.SweepSeconds) * time.Second)
	defer t.Stop()
	for {
		a.sweep(ctx)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

func (a *API) sweep(ctx context.Context) {
	gs, err := a.db.ExpireGrants(ctx, expiryEntry)
	if err != nil {
		log.Printf("access sweep: %v", err)
		return
	}
	for _, g := range gs {
		log.Printf("access grant %s (%s) for tenant %s expired", g.ID, g.Kind, g.TenantID)
		a.notifyExpired(g)
	}
}

// expiryEntry is the system's audit entry for a grant reaching its end time.
func expiryEntry(g db.Grant) db.AuditEntry {
	act := "access.expire"
	if g.Kind == "breakglass" {
		act = "breakglass.end"
	}
	return db.AuditEntry{TenantID: g.TenantID, WorkspaceID: g.WorkspaceID, GrantID: g.ID, Actor: "system",
		ActorName: "Shippin (automatic)", Action: act, Detail: "Time limit reached", BreakGlass: g.Kind == "breakglass"}
}
