package main

import (
	"context"
	"fmt"

	"shippin.cloud/console/internal/db"
	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/managed"
)

// attachRRFlow mirrors the console's service.provision job for one workspace: ensure the tenant
// VM's shared rrflow/state instance (Notion D36) with the project's namespace, then bind it (hostd mints the binding token straight into the
// workspace; this process never sees it). Operator test path only.
func attachRRFlow(ctx context.Context, d *db.DB, host *hostclient.Client, tenant, ws, by string) error {
	w, err := d.WorkspaceByID(ctx, ws)
	if err != nil || w == nil || w.ProjectID == "" {
		return fmt.Errorf("workspace %s has no project", ws)
	}
	tn, err := d.TenantByID(ctx, tenant)
	if err != nil || tn == nil {
		return fmt.Errorf("tenant lookup failed")
	}
	svcs := &managed.Store{Pool: d.Pool}
	inst, err := svcs.EnsureVM(ctx, tenant, w.ProjectID, managed.KindRRFlow, by)
	if err != nil {
		return err
	}
	b, err := svcs.Bind(ctx, tenant, inst.ID, ws, w.ProjectID)
	if err != nil {
		return err
	}
	at := hostclient.Svc{Tenant: tenant, Project: w.ProjectID, ID: inst.ID, Shared: true}
	ep, err := host.EnsureRRFlow(ctx, at, tn.Plan)
	if err != nil {
		_ = svcs.SetState(ctx, inst.ID, "error", nil, "rrflow could not be started.")
		return err
	}
	if err := svcs.SetState(ctx, inst.ID, "ready", ep, ""); err != nil {
		return err
	}
	if err := host.BindService(ctx, at, b.ID, ws); err != nil {
		_ = svcs.SetBinding(ctx, b.ID, "error", "The devspace must be running to attach rrflow.")
		return err
	}
	fmt.Printf("rrflow instance %s bound to %s (binding %s)\n", inst.ID, ws, b.ID)
	return svcs.SetBinding(ctx, b.ID, "bound", "")
}
