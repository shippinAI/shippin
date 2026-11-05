package api

import (
	"context"
	"log"
	"time"

	"shippin.cloud/console/internal/config"
	"shippin.cloud/console/internal/jobs"
	"shippin.cloud/console/internal/projects"
)

// reconcileLock is the advisory lock that elects one reconciling console ("SHIPREC").
const reconcileLock int64 = 0x53_4849_5052_4543

// runReconciler re-enqueues work whose console died, marks dead-ended workspaces failed, prunes
// old jobs and idempotency keys, and (less often) reports inventory drift to the audit log.
func (a *API) runReconciler(ctx context.Context) {
	j := a.cfg.Jobs
	tick := time.NewTicker(config.Sec(j.ReconcileSeconds))
	defer tick.Stop()
	lastDrift := time.Time{}
	for {
		drift := time.Since(lastDrift) >= config.Sec(j.DriftSeconds)
		if a.reconcileOnce(ctx, drift) && drift {
			lastDrift = time.Now()
		}
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// reconcileOnce runs one pass if this console holds the leader lock; it reports whether it ran.
func (a *API) reconcileOnce(ctx context.Context, drift bool) bool {
	ok, release, err := a.db.TryLeader(ctx, reconcileLock)
	if err != nil || !ok {
		return false
	}
	defer release()
	j := a.cfg.Jobs
	stale := config.Sec(2 * j.LeaseSeconds)
	a.relayDeploymentOutbox(ctx)
	a.reconcileWorkspaces(ctx, stale)
	if err := a.db.ReconcileWorkspaceActions(ctx, stale); err != nil {
		log.Printf("reconcile: workspace actions: %v", err)
	}
	a.reconcileTenants(ctx, stale)
	if j.ProvisionPlannedBoundaries {
		a.reconcileBoundaries(ctx)
	}
	a.stopIdleTerminals(ctx)
	a.githubMaintenance(ctx)
	if _, err := a.jobs.Prune(ctx, config.Sec(j.KeepDays*86400)); err != nil {
		log.Printf("reconcile: prune jobs: %v", err)
	}
	if _, err := a.db.PruneIdemKeys(ctx, config.Sec(j.IdempotencyHours*3600)); err != nil {
		log.Printf("reconcile: prune idempotency keys: %v", err)
	}
	if drift {
		a.checkDrift(ctx)
	}
	return true
}

func (a *API) reconcileWorkspaces(ctx context.Context, stale time.Duration) {
	orphans, err := a.db.OrphanWorkspaces(ctx, stale)
	if err != nil {
		log.Printf("reconcile: workspaces: %v", err)
		return
	}
	for _, o := range orphans {
		if o.LastStatus == jobs.Failed || o.LastStatus == jobs.Canceled {
			_ = a.db.SetWorkspaceState(ctx, o.ID, "failed", "", "provisioning failed: "+o.LastError)
			continue
		}
		a.enqueue(ctx, jobs.Job{TenantID: o.TenantID, Kind: jobs.KindWorkspaceProvision, TargetType: "workspace",
			TargetID: o.ID, MaxAttempts: a.cfg.Jobs.MaxAttempts, CreatedBy: o.CreatedBy})
	}
}

func (a *API) reconcileTenants(ctx context.Context, stale time.Duration) {
	ts, err := a.db.StuckTenants(ctx, stale)
	if err != nil {
		log.Printf("reconcile: tenants: %v", err)
		return
	}
	for _, t := range ts {
		a.enqueue(ctx, jobs.Job{TenantID: t.ID, Kind: jobs.KindTenantProvision, TargetType: "tenant",
			TargetID: t.ID, MaxAttempts: a.cfg.Jobs.MaxAttempts, CreatedBy: reconcilerActor})
	}
}

func (a *API) reconcileBoundaries(ctx context.Context) {
	bs, err := (&projects.Store{Pool: a.db.Pool}).PlannedBoundaries(ctx)
	if err != nil {
		log.Printf("reconcile: boundaries: %v", err)
		return
	}
	for _, b := range bs {
		a.enqueue(ctx, jobs.Job{TenantID: b.Tenant, Kind: jobs.KindBoundaryProvision, TargetType: "boundary",
			TargetID: b.ID, MaxAttempts: a.cfg.Jobs.MaxAttempts, CreatedBy: reconcilerActor})
	}
}

// enqueue adds a job (joining a live one for the same target) and wakes a worker.
func (a *API) enqueue(ctx context.Context, j jobs.Job) {
	got, created, err := a.jobs.Enqueue(ctx, j)
	if err != nil {
		log.Printf("reconcile: enqueue %s %s: %v", j.Kind, j.TargetID, err)
		return
	}
	if created {
		log.Printf("reconcile: queued %s for %s %s (job %s)", j.Kind, j.TargetType, j.TargetID, got.ID)
		if a.runner != nil {
			a.runner.Wake()
		}
	}
}
