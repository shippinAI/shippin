package api

import (
	"context"
	"errors"
	"log"
	"time"

	"github.com/google/uuid"
	"shippin.cloud/console/internal/db"
)

// errTenantLocked: provisioning never reactivates a suspended tenant or resurrects a deleted one.
var errTenantLocked = errors.New("tenant is suspended or deleted")

// ensureTenant maps an IdP org to a tenant (UUIDv7, created once) and runs idempotent
// provisioning through shippin-hostd. Shared by operator create and self-service onboarding.
func (a *API) ensureTenant(ctx context.Context, orgID, name, plan string) (*db.Tenant, error) {
	t, err := a.db.TenantByOrg(ctx, orgID)
	if err != nil {
		return nil, err
	}
	if t != nil && (t.Status == "suspended" || t.Status == "deleted" || t.Status == "deleting") {
		return t, errTenantLocked
	}
	if t == nil {
		id, _ := uuid.NewV7()
		t = &db.Tenant{ID: id.String(), IdpOrgID: orgID, Name: name, Plan: plan, Status: "provisioning"}
		if err := a.db.CreateTenant(ctx, *t); err != nil {
			return nil, err
		}
	}
	pctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	p, err := a.host.Provision(pctx, t.ID, t.Plan)
	if err != nil {
		_ = a.db.SetTenantStatus(pctx, t.ID, "failed", "", err.Error())
		t.Status = "failed"
		return t, err
	}
	_ = a.db.SetTenantStatus(pctx, t.ID, "active", p.SSHIP, "")
	t.Status, t.SSHIP = "active", p.SSHIP
	a.resyncWorkspaces(pctx, *t)
	go a.ensureVaultixWorkspace(t.ID, "")
	return t, nil
}

// resyncWorkspaces republishes every workspace's route and ports (needed after hostd migrated
// the tenant network and workspace addresses changed). Failures are recorded per workspace.
func (a *API) resyncWorkspaces(ctx context.Context, t db.Tenant) {
	ws, err := a.db.Workspaces(ctx, t.ID)
	if err != nil {
		return
	}
	tv := tenantView{Tenant: t}
	for _, w := range ws {
		if w.State != "running" && w.State != "stopped" {
			continue
		}
		if err := a.host.Action(ctx, t.ID, w.ID, "sync", a.hostReq(&tv, w)); err != nil {
			log.Printf("resync workspace %s: %v", w.ID, err)
			_ = a.db.SetWorkspaceState(ctx, w.ID, "failed", "", "Re-sync after network change failed: "+err.Error())
		}
	}
}
