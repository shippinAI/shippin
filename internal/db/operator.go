package db

import "context"

// AllWorkspaces lists every live workspace across tenants (provider panel).
func (d *DB) AllWorkspaces(ctx context.Context) ([]Workspace, error) {
	return d.queryWS(ctx, `WHERE state<>'deleted' ORDER BY created_at`)
}

// WorkspaceByID returns a live workspace by id.
func (d *DB) WorkspaceByID(ctx context.Context, id string) (*Workspace, error) {
	ws, err := d.queryWS(ctx, `WHERE id::text=$1 AND state<>'deleted'`, id)
	if err != nil || len(ws) == 0 {
		return nil, err
	}
	return &ws[0], nil
}

// TenantByID returns one tenant (any status).
func (d *DB) TenantByID(ctx context.Context, id string) (*Tenant, error) {
	rows, err := d.Pool.Query(ctx, `SELECT `+tenantCols+` FROM tenants WHERE id::text=$1`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ts, err := scanTenants(rows)
	if err != nil || len(ts) == 0 {
		return nil, err
	}
	return &ts[0], nil
}

// MarkTenantDeleted keeps the rows (audit) but retires the tenant and its workspaces atomically.
func (d *DB) MarkTenantDeleted(ctx context.Context, id string) error {
	tx, err := d.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, `UPDATE workspaces SET state='deleted', updated_at=now() WHERE tenant_id::text=$1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE tenants SET status='deleted', error=NULL, updated_at=now() WHERE id::text=$1`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// BeginTenantOp moves a tenant from one of from to state atomically; false if it wasn't in from.
func (d *DB) BeginTenantOp(ctx context.Context, id, state string, from ...string) (bool, error) {
	tag, err := d.Pool.Exec(ctx, `UPDATE tenants SET status=$2, error=NULL, updated_at=now() WHERE id::text=$1 AND status = ANY($3)`, id, state, from)
	return err == nil && tag.RowsAffected() == 1, err
}

// WorkspaceActionTarget retains tombstones solely for authorized operation receipt
// replay. Inventory/detail readers continue to exclude deleted workspaces.
func (d *DB) WorkspaceActionTarget(ctx context.Context, id string) (*Workspace, error) {
	rows, err := d.queryWS(ctx, `WHERE id::text=$1`, id)
	if err != nil || len(rows) == 0 {
		return nil, err
	}
	return &rows[0], nil
}
