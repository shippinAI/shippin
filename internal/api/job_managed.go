package api

import (
	"context"
	"errors"
	"log"

	"shippin.cloud/console/internal/hostclient"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/managed"
)

// runServiceJob (service.provision): ensure the tenant VM's shared rrflow/state instance (Notion
// D36), move any pre-D36 per-project instance's state into its project namespace, then bind every
// pending binding (hostd mints each binding token and hands it straight to the workspace; the
// console never sees it). Idempotent.
func (a *API) runServiceJob(ctx context.Context, j *jobs.Job, step func(string)) (any, error) {
	if a.svcs == nil {
		return nil, jobs.Permanent(errRRFlowHeld)
	}
	// The rollout gate holds for queued work too: a job from migration 026 or an older release never
	// runs while services.rrflow is off or the tenant is not in its rollout (GC-20261019-PB ruling e).
	if !a.cfg.Services.RRFlow.Allows(j.TenantID) {
		return nil, jobs.Permanent(errRRFlowOff)
	}
	inst, err := a.svcs.Get(ctx, j.TenantID, j.TargetID)
	if err != nil {
		return nil, jobs.Permanent(err)
	}
	if inst.Kind != managed.KindRRFlow {
		return nil, jobs.Permanent(errors.New("unknown service kind"))
	}
	if inst.Scope != "vm" {
		return nil, jobs.Permanent(errors.New("per-project rrflow instances are retired (D36); the shared instance's job migrates them"))
	}
	tn, err := a.db.TenantByID(ctx, j.TenantID)
	if err != nil || tn == nil {
		return nil, errors.New("tenant lookup failed")
	}
	ns, err := a.svcs.Namespaces(ctx, inst.ID)
	if err != nil || len(ns) == 0 {
		return nil, jobs.Permanent(errors.New("shared rrflow instance has no project namespace"))
	}
	step("Starting rrflow on your private server")
	_ = a.svcs.SetState(ctx, inst.ID, "provisioning", nil, "")
	ep, err := a.host.EnsureRRFlow(ctx, hostclient.Svc{Tenant: j.TenantID, Project: ns[0], ID: inst.ID, Shared: true}, tn.Plan)
	if err != nil {
		if j.Attempts >= j.MaxAttempts {
			_ = a.svcs.SetState(context.WithoutCancel(ctx), inst.ID, "error", nil, "rrflow could not be started.")
		}
		return nil, err
	}
	if err := a.svcs.SetState(ctx, inst.ID, "ready", ep, ""); err != nil {
		return nil, err
	}
	moved, err := a.migrateRRFlow(ctx, inst, step)
	if err != nil {
		return nil, err
	}
	bound := 0
	for _, b := range inst.Bindings {
		if b.State == "bound" {
			continue
		}
		step("Attaching rrflow to your devspace")
		if err := a.host.BindService(ctx, hostclient.Svc{Tenant: j.TenantID, Project: b.ProjectID, ID: inst.ID, Shared: true}, b.ID, b.WorkspaceID); err != nil {
			_ = a.svcs.SetBinding(ctx, b.ID, "error", "The devspace must be running to attach rrflow.")
			return nil, err
		}
		_ = a.svcs.SetBinding(ctx, b.ID, "bound", "")
		bound++
	}
	return map[string]any{"instance": inst.ID, "bound": bound, "migrated": moved}, nil
}

// migrateRRFlow copies each pre-D36 instance marked for vm (026_rrflow_per_vm.sql) into its project
// namespace (hostd snapshots it first) and marks it migrated; done before binding so a namespace is never served half-filled.
func (a *API) migrateRRFlow(ctx context.Context, vm *managed.Instance, step func(string)) (int, error) {
	old, err := a.svcs.MigratingInto(ctx, vm.TenantID, vm.ID)
	if err != nil {
		return 0, err
	}
	for _, o := range old {
		step("Moving rrflow state into the shared instance")
		if err := a.host.MigrateService(ctx, hostclient.Svc{Tenant: o.TenantID, Project: o.ProjectID, ID: o.ID}, vm.ID); err != nil {
			log.Printf("rrflow migrate %s -> %s: %v", o.ID, vm.ID, err)
			return 0, err
		}
		if err := a.svcs.SetState(ctx, o.ID, "migrated", nil, ""); err != nil { // retired later by hostd retire-into
			return 0, err
		}
	}
	return len(old), nil
}

// attachRRFlow (prompt builds, chat) provisions and binds inline, so the agent starts with rrflow ready.
func (a *API) attachRRFlow(ctx context.Context, tenant, project, ws, by string) error {
	if err := a.rrflowVMAllowed(ctx, tenant); err != nil {
		return err
	}
	inst, err := a.svcs.EnsureVM(ctx, tenant, project, managed.KindRRFlow, by)
	if err != nil {
		return err
	}
	if _, err := a.svcs.Bind(ctx, tenant, inst.ID, ws, project); err != nil {
		return err
	}
	j := jobs.Job{TenantID: tenant, TargetID: inst.ID, Attempts: 1, MaxAttempts: 1}
	_, err = a.runServiceJob(ctx, &j, func(string) {})
	return err
}
