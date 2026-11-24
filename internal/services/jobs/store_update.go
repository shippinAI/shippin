package jobs

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// owned runs an update that only applies while owner still holds the job's lease.
func (s *Store) owned(ctx context.Context, sql string, args ...any) error {
	tag, err := s.pool.Exec(ctx, sql, args...)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrLostLease
	}
	return nil
}

// Heartbeat renews the lease.
func (s *Store) Heartbeat(ctx context.Context, id, owner string, lease time.Duration) error {
	return s.owned(ctx, `UPDATE jobs SET lease_until=now()+make_interval(secs => $3), updated_at=now()
	  WHERE id=$1 AND lease_owner=$2 AND status='running'`, id, owner, lease.Seconds())
}

// Step records the human-readable progress line (Job.message in the API).
func (s *Store) Step(ctx context.Context, id, owner, step string) error {
	return s.owned(ctx, `UPDATE jobs SET step=$3, updated_at=now()
	  WHERE id=$1 AND lease_owner=$2 AND status='running'`, id, owner, step)
}

// Succeed finishes the job with an optional JSON result.
func (s *Store) Succeed(ctx context.Context, id, owner string, result any) error {
	var raw []byte
	if result != nil {
		var err error
		if raw, err = json.Marshal(result); err != nil {
			return err
		}
	}
	return s.owned(ctx, `UPDATE jobs SET status='succeeded', result=$3, error=NULL, lease_owner=NULL,
	  lease_until=NULL, updated_at=now(), finished_at=now()
	  WHERE id=$1 AND lease_owner=$2 AND status='running'`, id, owner, raw)
}

// Fail records msg. With retry and attempts left the job is requeued after backoff; otherwise it fails.
// It reports whether the job is now terminally failed.
func (s *Store) Fail(ctx context.Context, id, owner, msg string, retry bool, backoff time.Duration) (bool, error) {
	var final bool
	err := s.pool.QueryRow(ctx, `UPDATE jobs SET error=$3, lease_owner=NULL, lease_until=NULL, updated_at=now(),
	  status = CASE WHEN $4 AND attempts < max_attempts THEN 'queued' ELSE 'failed' END,
	  run_after = CASE WHEN $4 AND attempts < max_attempts THEN now()+make_interval(secs => $5) ELSE run_after END,
	  finished_at = CASE WHEN $4 AND attempts < max_attempts THEN NULL ELSE now() END
	  WHERE id=$1 AND lease_owner=$2 AND status='running' RETURNING status='failed'`,
		id, owner, msg, retry, backoff.Seconds()).Scan(&final)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, ErrLostLease
	}
	return final, err
}

// Release hands a job back unfinished (shutdown): queued again, and the attempt isn't counted.
func (s *Store) Release(ctx context.Context, id, owner string) error {
	return s.owned(ctx, `UPDATE jobs SET status='queued', attempts=greatest(attempts-1,0), lease_owner=NULL,
	  lease_until=NULL, updated_at=now() WHERE id=$1 AND lease_owner=$2 AND status='running'`, id, owner)
}

// Prune deletes finished jobs older than keep.
func (s *Store) Prune(ctx context.Context, keep time.Duration) (int64, error) {
	tag, err := s.pool.Exec(ctx, `DELETE FROM jobs WHERE finished_at IS NOT NULL
	  AND finished_at < now()-make_interval(secs => $1)`, keep.Seconds())
	return tag.RowsAffected(), err
}
