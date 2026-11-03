package jobs

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
)

// FinishProvision fences the workspace outcome by this claim and the provisioning state.
// Locking the job prevents lease reclamation between authorization and the workspace write.
func (s *Store) FinishProvision(ctx context.Context, j *Job, state, ip, message string) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	var id string
	e = tx.QueryRow(ctx, `SELECT id::text FROM jobs WHERE id=$1 AND lease_owner=$2 AND status='running' AND lease_until>now() AND kind=$3 FOR UPDATE`, j.ID, j.LeaseOwner, KindWorkspaceProvision).Scan(&id)
	if errors.Is(e, pgx.ErrNoRows) {
		return ErrLostLease
	}
	if e != nil {
		return e
	}
	tag, e := tx.Exec(ctx, `UPDATE workspaces SET state=$3,ipv4=coalesce(nullif($4,''),ipv4),error=nullif($5,''),updated_at=now() WHERE id::text=$1 AND tenant_id::text=$2 AND state='provisioning'`, j.TargetID, j.TenantID, state, ip, message)
	if e != nil {
		return e
	}
	if tag.RowsAffected() != 1 {
		return ErrLostLease
	}
	return tx.Commit(ctx)
}
