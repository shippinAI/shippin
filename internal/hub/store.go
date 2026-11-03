package hub

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// Store reads and writes hub_tasks and hub_events.
type Store struct{ Pool *pgxpool.Pool }

const cols = `id, org_id, title, body, status, progress, coalesce(lease_owner,''), lease_until,
  approval_note, coalesce(decided_by,''), decided_at, created_by, created_at, updated_at`

func scan(row pgx.Row) (*Task, error) {
	t := &Task{}
	err := row.Scan(&t.ID, &t.OrgID, &t.Title, &t.Body, &t.Status, &t.Progress, &t.LeaseOwner, &t.LeaseUntil,
		&t.ApprovalNote, &t.DecidedBy, &t.DecidedAt, &t.CreatedBy, &t.CreatedAt, &t.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return t, err
}

// UpsertApproval posts a factory-board card already in needs_approval, keyed by externalKey
// (org + key is unique). A card that was already decided is left as it is.
func (s *Store) UpsertApproval(ctx context.Context, org, externalKey, title, body, by string) (*Task, error) {
	if strings.TrimSpace(org) == "" || strings.TrimSpace(externalKey) == "" {
		return nil, errors.New("approval org and key are required")
	}
	if title == "" {
		title = externalKey
	}
	id, _ := uuid.NewV7()
	t, err := scan(s.Pool.QueryRow(ctx, `INSERT INTO hub_tasks(id,org_id,title,body,status,created_by,external_key)
	  VALUES ($1,$2,$3,$4,'needs_approval',$5,$6)
	  ON CONFLICT (org_id, external_key) WHERE external_key <> ''
	  DO UPDATE SET title=EXCLUDED.title, body=EXCLUDED.body, updated_at=now()
	  WHERE hub_tasks.status='needs_approval'
	  RETURNING `+cols, id.String(), org, title, body, by, externalKey))
	if errors.Is(err, ErrNotFound) {
		return s.approvalByKey(ctx, org, externalKey)
	}
	return t, err
}

// ApprovalByKey returns the card posted under externalKey.
func (s *Store) ApprovalByKey(ctx context.Context, org, externalKey string) (*Task, error) {
	return s.approvalByKey(ctx, org, externalKey)
}

func (s *Store) approvalByKey(ctx context.Context, org, externalKey string) (*Task, error) {
	return scan(s.Pool.QueryRow(ctx, `SELECT `+cols+` FROM hub_tasks WHERE org_id=$1 AND external_key=$2`, org, externalKey))
}

// Create opens a new task.
func (s *Store) Create(ctx context.Context, org, title, body, by string) (*Task, error) {
	id, _ := uuid.NewV7()
	t, err := scan(s.Pool.QueryRow(ctx, `INSERT INTO hub_tasks(id,org_id,title,body,created_by)
	  VALUES ($1,$2,$3,$4,$5) RETURNING `+cols, id.String(), org, title, body, by))
	if err == nil {
		s.event(ctx, t.ID, by, "created", title)
	}
	return t, err
}

// List returns the org's tasks, oldest first.
func (s *Store) List(ctx context.Context, org string) ([]Task, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+cols+` FROM hub_tasks WHERE org_id=$1 ORDER BY created_at`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Task{}
	for rows.Next() {
		t, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// Claim checks a task out to agent for lease: open tasks, or claimed/in_progress ones whose
// lease expired (the agent died). The same agent may re-claim to extend its lease.
func (s *Store) Claim(ctx context.Context, org, id, agent string, lease time.Duration) (*Task, error) {
	return s.move(ctx, org, id, agent, "claimed", `status='claimed', lease_owner=$3,
	  lease_until=now()+make_interval(secs => $4)`, `(status='open' OR (status IN ('claimed','in_progress')
	  AND (lease_until < now() OR lease_owner=$3)))`, agent, lease.Seconds())
}

// Progress records a note from the lease holder and extends the lease.
func (s *Store) Progress(ctx context.Context, org, id, agent, note string, lease time.Duration) (*Task, error) {
	return s.move(ctx, org, id, agent, "progress:"+note, `status='in_progress', progress=$5,
	  lease_until=now()+make_interval(secs => $4)`, `status IN ('claimed','in_progress') AND lease_owner=$3
	  AND lease_until >= now()`, agent, lease.Seconds(), note)
}

// RequestApproval parks the task for the owner's sign-off and releases the lease.
func (s *Store) RequestApproval(ctx context.Context, org, id, agent, note string) (*Task, error) {
	return s.move(ctx, org, id, agent, "approval_requested:"+note, `status='needs_approval', approval_note=$4,
	  lease_until=NULL`, `status IN ('claimed','in_progress') AND lease_owner=$3`, agent, note)
}

// Decide approves or rejects a needs_approval task. Owner checks happen in the HTTP layer.
func (s *Store) Decide(ctx context.Context, org, id, owner string, approve bool, note string) (*Task, error) {
	st := Rejected
	if approve {
		st = Approved
	}
	return s.move(ctx, org, id, owner, st+":"+note, `status=$4, decided_by=$3, decided_at=now()`,
		`status='needs_approval'`, owner, st)
}

// Finish marks work done: by the lease holder (no sign-off needed) or after approval.
func (s *Store) Finish(ctx context.Context, org, id, agent string) (*Task, error) {
	return s.move(ctx, org, id, agent, "done", `status='done', lease_until=NULL`,
		`(status='approved' OR (status IN ('claimed','in_progress') AND lease_owner=$3))`, agent)
}

// move applies set where cond holds; $1=id, $2=org, then args. kind may carry ":note". A miss is NotFound or Conflict.
func (s *Store) move(ctx context.Context, org, id, actor, kind, set, cond string, args ...any) (*Task, error) {
	kind, note, _ := strings.Cut(kind, ":")
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrNotFound
	}
	t, err := scan(s.Pool.QueryRow(ctx, `UPDATE hub_tasks SET `+set+`, updated_at=now()
	  WHERE id=$1 AND org_id=$2 AND `+cond+` RETURNING `+cols, append([]any{id, org}, args...)...))
	if errors.Is(err, ErrNotFound) {
		var n int
		if s.Pool.QueryRow(ctx, `SELECT count(*) FROM hub_tasks WHERE id=$1 AND org_id=$2`, id, org).Scan(&n) == nil && n > 0 {
			return nil, ErrConflict
		}
		return nil, ErrNotFound
	}
	if err == nil {
		s.event(ctx, id, actor, kind, note)
	}
	return t, err
}

func (s *Store) event(ctx context.Context, id, actor, kind, note string) {
	_, _ = s.Pool.Exec(ctx, `INSERT INTO hub_events(task_id,actor,kind,note) VALUES ($1,$2,$3,$4)`, id, actor, kind, note)
}
