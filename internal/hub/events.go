package hub

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// Event is one line of a task's history (created, claimed, progress, approval_requested,
// approved, rejected, done), written in the same request as the transition.
type Event struct {
	ID    int64     `json:"id"`
	Actor string    `json:"actor"`
	Kind  string    `json:"kind"`
	Note  string    `json:"note"`
	At    time.Time `json:"at"`
}

// Events returns a task's history, oldest first, only if the task belongs to org.
func (s *Store) Events(ctx context.Context, org, id string) ([]Event, error) {
	if _, err := uuid.Parse(id); err != nil {
		return nil, ErrNotFound
	}
	var n int
	if err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM hub_tasks WHERE id=$1 AND org_id=$2`, id, org).Scan(&n); err != nil {
		return nil, err
	}
	if n == 0 {
		return nil, ErrNotFound
	}
	rows, err := s.Pool.Query(ctx, `SELECT id, actor, kind, note, at FROM hub_events WHERE task_id=$1 ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Event{}
	for rows.Next() {
		var e Event
		if err := rows.Scan(&e.ID, &e.Actor, &e.Kind, &e.Note, &e.At); err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}
