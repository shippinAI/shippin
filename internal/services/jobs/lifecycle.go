package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

var ErrWorkspaceBusy = errors.New("workspace has an operation in progress")

// Tx is what EnqueueActionOn writes through: the caller's transaction.
type Tx interface {
	Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// EnqueueAction commits the workspace claim and durable intent together.
func (s *Store) EnqueueAction(ctx context.Context, j Job, state string) (*Job, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	got, err := EnqueueActionOn(ctx, tx, j, state)
	if err != nil {
		return nil, err
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return got, nil
}

// EnqueueActionOn claims the workspace and records the durable intent on the caller's transaction,
// so they are kept or lost with whatever else it holds: the audit entry and the replay receipt.
// ErrWorkspaceBusy when the workspace has work in progress.
func EnqueueActionOn(ctx context.Context, tx Tx, j Job, state string) (*Job, error) {
	tag, err := tx.Exec(ctx, `UPDATE workspaces SET state=$3,error=NULL,updated_at=now()
 WHERE id::text=$1 AND tenant_id::text=$2 AND state NOT IN ('provisioning','deleting','deleted','starting','stopping')
 AND NOT EXISTS (SELECT 1 FROM jobs WHERE target_type='workspace' AND target_id=$1 AND status IN ('queued','running','canceling'))`, j.TargetID, j.TenantID, state)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrWorkspaceBusy
	}
	return scan(tx.QueryRow(ctx, `INSERT INTO jobs(id,tenant_id,kind,target_type,target_id,max_attempts,payload,created_by)
 VALUES($1,$2,$3,'workspace',$4,$5,$6,$7) RETURNING `+cols, uuid.NewString(), j.TenantID, KindWorkspaceAction, j.TargetID, j.MaxAttempts, []byte(j.Payload), j.CreatedBy))
}

// ActionPhase fences writes by the current lease. Dispatch is persisted before hostd is called.
func (s *Store) ActionPhase(ctx context.Context, j *Job, phase string) error {
	return s.owned(ctx, `UPDATE jobs SET payload=jsonb_set(payload,'{phase}',to_jsonb($3::text)),updated_at=now()
 WHERE id=$1 AND lease_owner=$2 AND status='running' AND lease_until>now()`, j.ID, j.LeaseOwner, phase)
}

// FinishAction prevents an old worker from changing workspace state after losing its lease.
func (s *Store) FinishAction(ctx context.Context, j *Job, state, msg string) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var id string
	err = tx.QueryRow(ctx, `SELECT id::text FROM jobs WHERE id=$1 AND lease_owner=$2 AND status='running' AND lease_until>now() FOR UPDATE`, j.ID, j.LeaseOwner).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrLostLease
	}
	if err != nil {
		return err
	}
	tag, err := tx.Exec(ctx, `UPDATE workspaces SET state=$2,error=nullif($3,''),updated_at=now() WHERE id::text=$1 AND tenant_id::text=$4`, j.TargetID, state, msg, j.TenantID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return errors.New("workspace completion target missing")
	}
	phase, _ := json.Marshal(map[string]string{"state": state, "message": msg})
	_, err = tx.Exec(ctx, `UPDATE jobs SET payload=jsonb_set(payload,'{completion}',$2::jsonb) WHERE id=$1`, j.ID, phase)
	if err != nil {
		return err
	}
	return tx.Commit(ctx)
}
