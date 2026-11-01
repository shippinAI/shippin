package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store reads and writes the jobs table.
type Store struct{ pool *pgxpool.Pool }

// NewStore wraps a pool.
func NewStore(p *pgxpool.Pool) *Store { return &Store{pool: p} }

const cols = `id, tenant_id, kind, target_type, target_id, status, step, attempts, max_attempts,
  payload, result, coalesce(error,''), created_by, created_at, updated_at, finished_at, coalesce(lease_owner,'')`

func scan(row pgx.Row) (*Job, error) {
	j := &Job{}
	var payload, result []byte
	err := row.Scan(&j.ID, &j.TenantID, &j.Kind, &j.TargetType, &j.TargetID, &j.Status, &j.Step,
		&j.Attempts, &j.MaxAttempts, &payload, &result, &j.Error, &j.CreatedBy,
		&j.CreatedAt, &j.UpdatedAt, &j.FinishedAt, &j.LeaseOwner)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	j.Payload, j.Result = payload, result
	return j, nil
}

// Enqueue inserts a queued job, or returns the live job for the same kind and target (created=false).
func (s *Store) Enqueue(ctx context.Context, j Job) (job *Job, created bool, err error) {
	if len(j.Payload) == 0 {
		j.Payload = json.RawMessage(`{}`)
	}
	id, _ := uuid.NewV7()
	for range 3 {
		job, err = scan(s.pool.QueryRow(ctx, `INSERT INTO jobs(id, tenant_id, kind, target_type, target_id,
		  max_attempts, payload, created_by) VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
		  ON CONFLICT (kind, target_type, target_id) WHERE status IN ('queued','running','canceling')
		  DO NOTHING RETURNING `+cols,
			id.String(), j.TenantID, j.Kind, j.TargetType, j.TargetID, j.MaxAttempts, []byte(j.Payload), j.CreatedBy))
		if err == nil {
			return job, true, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, false, err
		}
		job, err = s.Live(ctx, j.Kind, j.TargetType, j.TargetID)
		if err == nil {
			return job, false, nil
		}
		if !errors.Is(err, ErrNotFound) {
			return nil, false, err
		}
		// The live job finished between the insert and the read: insert again.
	}
	return nil, false, errors.New("enqueue kept racing a finishing job")
}

// Live returns the queued/running job for a kind and target.
func (s *Store) Live(ctx context.Context, kind, targetType, targetID string) (*Job, error) {
	return scan(s.pool.QueryRow(ctx, `SELECT `+cols+` FROM jobs WHERE kind=$1 AND target_type=$2
	  AND target_id=$3 AND status IN ('queued','running','canceling')`, kind, targetType, targetID))
}

// Get returns one job.
func (s *Store) Get(ctx context.Context, id string) (*Job, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrNotFound
	}
	return scan(s.pool.QueryRow(ctx, `SELECT `+cols+` FROM jobs WHERE id=$1`, id))
}

// Claim takes the next due job of the given kinds: queued, or running with an expired lease
// (its worker died). Concurrent claimers never get the same row (SKIP LOCKED).
func (s *Store) Claim(ctx context.Context, owner string, kinds []string, lease time.Duration) (*Job, error) {
	j, err := scan(s.pool.QueryRow(ctx, `UPDATE jobs SET status='running', attempts=attempts+1,
	  lease_owner=$1, lease_until=now()+make_interval(secs => $2), updated_at=now()
	  WHERE id = (SELECT id FROM jobs WHERE kind = ANY($3) AND run_after <= now()
	    AND (status='queued' OR (status='running' AND lease_until < now()))
	    ORDER BY run_after, created_at FOR UPDATE SKIP LOCKED LIMIT 1)
	  RETURNING `+cols, owner, lease.Seconds(), kinds))
	if errors.Is(err, ErrNotFound) {
		return nil, nil
	}
	return j, err
}
