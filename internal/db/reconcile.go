package db

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

// OrphanWorkspace is a workspace left provisioning with no live job, plus how its last job ended.
type OrphanWorkspace struct {
	ID, TenantID, CreatedBy string
	LastStatus, LastError   string // "" when it never had a job (in-process path, or enqueue failed)
}

// OrphanWorkspaces lists workspaces provisioning for longer than age without a queued/running job.
func (d *DB) OrphanWorkspaces(ctx context.Context, age time.Duration) ([]OrphanWorkspace, error) {
	rows, err := d.Pool.Query(ctx, `SELECT w.id::text, w.tenant_id::text, w.created_by,
	    coalesce(j.status,''), coalesce(j.error,'')
	  FROM workspaces w
	  LEFT JOIN LATERAL (SELECT status, error FROM jobs WHERE kind='workspace.provision' AND target_type='workspace'
	    AND target_id=w.id::text ORDER BY created_at DESC LIMIT 1) j ON true
	  WHERE w.state='provisioning' AND w.updated_at < now()-make_interval(secs => $1)
	    AND coalesce(j.status,'') NOT IN ('queued','running','canceling')`, age.Seconds())
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (OrphanWorkspace, error) {
		var o OrphanWorkspace
		return o, r.Scan(&o.ID, &o.TenantID, &o.CreatedBy, &o.LastStatus, &o.LastError)
	})
}

// StuckTenants lists tenants provisioning for longer than age (their provisioning call died).
func (d *DB) StuckTenants(ctx context.Context, age time.Duration) ([]Tenant, error) {
	rows, err := d.Pool.Query(ctx, `SELECT `+tenantCols+` FROM tenants WHERE status='provisioning'
	  AND updated_at < now()-make_interval(secs => $1)`, age.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanTenants(rows)
}

// TryLeader takes a session advisory lock for one reconcile pass; release with the returned func.
// Only one console replica reconciles at a time.
func (d *DB) TryLeader(ctx context.Context, key int64) (bool, func(), error) {
	conn, err := d.Pool.Acquire(ctx)
	if err != nil {
		return false, nil, err
	}
	var ok bool
	if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1)`, key).Scan(&ok); err != nil || !ok {
		conn.Release()
		return false, nil, err
	}
	return true, func() {
		_, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, key)
		conn.Release()
	}, nil
}

// ReconcileWorkspaceActions exposes abandoned/terminal operations as needing review.
// Never replay a mutation when its outcome is uncertain.
func (d *DB) ReconcileWorkspaceActions(ctx context.Context, age time.Duration) error {
	_, err := d.Pool.Exec(ctx, `UPDATE workspaces w SET state='failed',
 error='Operation interrupted or exhausted retries. Verify the provider state before retrying.',updated_at=now()
 WHERE w.state IN ('starting','stopping','deleting') AND w.updated_at < now()-make_interval(secs => $1)
 AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.target_type='workspace' AND j.target_id=w.id::text
 AND j.status IN ('queued','running','canceling'))`, age.Seconds())
	return err
}
