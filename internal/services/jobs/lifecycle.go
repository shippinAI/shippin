package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

var ErrWorkspaceBusy = errors.New("workspace has an operation in progress")

// ActionAcceptance binds the HTTP replay receipt to the durable operation transaction.
type ActionAcceptance struct {
	Scope, Key string
	Response   func(*Job) ([]byte, error)
}

// EnqueueAction commits the workspace claim and durable intent together.
func (s *Store) EnqueueAction(ctx context.Context, j Job, state string, receipts ...ActionAcceptance) (*Job, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE workspaces SET state=$3,error=NULL,updated_at=now()
 WHERE id::text=$1 AND tenant_id::text=$2 AND state NOT IN ('provisioning','deleting','deleted','starting','stopping')
 AND NOT EXISTS (SELECT 1 FROM jobs WHERE target_type='workspace' AND target_id=$1 AND status IN ('queued','running','canceling'))`, j.TargetID, j.TenantID, state)
	if err != nil {
		return nil, err
	}
	if tag.RowsAffected() != 1 {
		return nil, ErrWorkspaceBusy
	}
	id := uuid.NewString()
	got, err := scan(tx.QueryRow(ctx, `INSERT INTO jobs(id,tenant_id,kind,target_type,target_id,max_attempts,payload,created_by)
 VALUES($1,$2,$3,'workspace',$4,$5,$6,$7) RETURNING `+cols, id, j.TenantID, KindWorkspaceAction, j.TargetID, j.MaxAttempts, []byte(j.Payload), j.CreatedBy))
	if err != nil {
		return nil, err
	}
	if len(receipts) > 0 && receipts[0].Key != "" {
		receipt := receipts[0]
		body, e := receipt.Response(got)
		if e != nil {
			return nil, e
		}
		headers, _ := json.Marshal(map[string]string{"Content-Type": "application/json", "Location": "/v1/jobs/" + got.ID})
		tag, e := tx.Exec(ctx, `UPDATE idempotency_keys SET status=202,body=$3,headers=$4 WHERE scope=$1 AND key=$2 AND status=0`, receipt.Scope, receipt.Key, body, headers)
		if e != nil {
			return nil, e
		}
		if tag.RowsAffected() != 1 {
			return nil, errors.New("idempotency reservation lost")
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return nil, err
	}
	return got, nil
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
