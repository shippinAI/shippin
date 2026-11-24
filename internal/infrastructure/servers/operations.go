package servers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Operation states. Queued and running are open; the rest are final.
const (
	OpQueued    = "queued"
	OpRunning   = "running"
	OpSucceeded = "succeeded"
	OpFailed    = "failed"
	OpUnknown   = "unknown"
	OpSkipped   = "skipped"
)

// Operation is one change to a guest and how it ended.
type Operation struct {
	ID          string     `json:"id"`
	ServerID    string     `json:"server_id"`
	ServerLabel string     `json:"server_label,omitempty"`
	BatchID     string     `json:"batch_id,omitempty"`
	Guest       string     `json:"guest"`
	Verb        string     `json:"verb"`
	Status      string     `json:"status"`
	Detail      string     `json:"detail,omitempty"`
	GuestStatus string     `json:"guest_status,omitempty"`
	Actor       string     `json:"-"`
	ActorName   string     `json:"actor_name,omitempty"`
	Note        string     `json:"note,omitempty"`
	StartedAt   time.Time  `json:"started_at"`
	FinishedAt  *time.Time `json:"finished_at,omitempty"`
}

// StartOperation records that a change is about to happen, and returns its id.
// RecentOutcomes is the status each guest ended in after a change finished in the last while. A
// hypervisor's inventory can lag behind a change it has already made.
func (s *Store) RecentOutcomes(ctx context.Context, serverID string, within time.Duration) (map[string]string, error) {
	if uuid.Validate(serverID) != nil {
		return nil, ErrNotFound
	}
	rows, err := s.Pool.Query(ctx, `SELECT DISTINCT ON (guest) guest, guest_status FROM infra_operations
		WHERE server_id=$1 AND status=$2 AND guest_status<>'' AND finished_at > now() - make_interval(secs => $3)
		ORDER BY guest, finished_at DESC`, serverID, OpSucceeded, within.Seconds())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]string{}
	for rows.Next() {
		var guest, status string
		if err := rows.Scan(&guest, &status); err != nil {
			return nil, err
		}
		out[guest] = status
	}
	return out, rows.Err()
}

func (s *Store) StartOperation(ctx context.Context, op Operation) (string, error) {
	id, err := uuid.NewV7()
	if err != nil {
		return "", err
	}
	status := op.Status
	if status == "" {
		status = OpRunning
	}
	var batch any
	if op.BatchID != "" {
		batch = op.BatchID
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO infra_operations(id, server_id, batch_id, guest, verb, status, actor, actor_name, note) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)`,
		id, op.ServerID, batch, op.Guest, op.Verb, status, op.Actor, op.ActorName, op.Note)
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// RunOperation moves a queued operation to running.
func (s *Store) RunOperation(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE infra_operations SET status='running', started_at=now() WHERE id=$1 AND status='queued'`, id)
	return err
}

// FinishOperation records how a change ended. A final state is never overwritten.
func (s *Store) FinishOperation(ctx context.Context, id, status, detail, guestStatus string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE infra_operations SET status=$2, detail=$3, guest_status=$4, finished_at=now() WHERE id=$1 AND status IN ('queued','running')`,
		id, status, detail, guestStatus)
	return err
}

// Operations lists recent changes, newest first, for one server or for all of them.
func (s *Store) Operations(ctx context.Context, serverID string, limit int) ([]Operation, error) {
	if serverID != "" && uuid.Validate(serverID) != nil {
		return nil, ErrNotFound
	}
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	const cols = `o.id, o.server_id, coalesce(nullif(s.label,''), s.slug), coalesce(o.batch_id::text,''), o.guest, o.verb, o.status, o.detail, o.guest_status, o.note, o.actor_name, o.started_at, o.finished_at`
	var (
		rows pgx.Rows
		err  error
	)
	if serverID == "" {
		rows, err = s.Pool.Query(ctx, `SELECT `+cols+` FROM infra_operations o JOIN servers s ON s.id=o.server_id ORDER BY o.started_at DESC, o.id DESC LIMIT $1`, limit)
	} else {
		rows, err = s.Pool.Query(ctx, `SELECT `+cols+` FROM infra_operations o JOIN servers s ON s.id=o.server_id WHERE o.server_id=$1 ORDER BY o.started_at DESC, o.id DESC LIMIT $2`, serverID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Operation{}
	for rows.Next() {
		var o Operation
		if err := rows.Scan(&o.ID, &o.ServerID, &o.ServerLabel, &o.BatchID, &o.Guest, &o.Verb, &o.Status, &o.Detail, &o.GuestStatus, &o.Note, &o.ActorName, &o.StartedAt, &o.FinishedAt); err != nil {
			return nil, err
		}
		out = append(out, o)
	}
	return out, rows.Err()
}

// AbandonOperations settles what a previous process left open. The hypervisor may have finished the
// work, so the honest answer is unknown, not failed.
func (s *Store) AbandonOperations(ctx context.Context) (int64, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE infra_operations SET status='unknown', finished_at=now(),
		detail='The panel restarted before this finished. Check the guest''s state; it may have completed.' WHERE status IN ('queued','running')`)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

// Fleet is a group of hosts in the panel. It changes nothing on them.
type Fleet struct {
	ID          string `json:"id"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
}

// Fleets lists every fleet by name.
func (s *Store) Fleets(ctx context.Context) ([]Fleet, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, label, description FROM infra_fleets ORDER BY label, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Fleet{}
	for rows.Next() {
		var f Fleet
		if err := rows.Scan(&f.ID, &f.Label, &f.Description); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

// SaveFleet creates a fleet, or renames the one with this id.
func (s *Store) SaveFleet(ctx context.Context, f Fleet, by string) (Fleet, error) {
	if f.ID == "" {
		id, err := uuid.NewV7()
		if err != nil {
			return Fleet{}, err
		}
		f.ID = id.String()
		_, err = s.Pool.Exec(ctx, `INSERT INTO infra_fleets(id, label, description, created_by) VALUES($1,$2,$3,$4)`, f.ID, f.Label, f.Description, by)
		return f, err
	}
	if uuid.Validate(f.ID) != nil {
		return Fleet{}, ErrNotFound
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE infra_fleets SET label=$2, description=$3 WHERE id=$1`, f.ID, f.Label, f.Description)
	if err != nil {
		return Fleet{}, err
	}
	if tag.RowsAffected() == 0 {
		return Fleet{}, ErrNotFound
	}
	return f, nil
}

// DeleteFleet removes the group. Its hosts stay, and simply belong to no fleet.
func (s *Store) DeleteFleet(ctx context.Context, id string) error {
	if uuid.Validate(id) != nil {
		return ErrNotFound
	}
	tag, err := s.Pool.Exec(ctx, `DELETE FROM infra_fleets WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

// SetFleet puts a host in a fleet, or in none when fleetID is empty.
func (s *Store) SetFleet(ctx context.Context, serverID, fleetID string) error {
	if uuid.Validate(serverID) != nil || (fleetID != "" && uuid.Validate(fleetID) != nil) {
		return ErrNotFound
	}
	var fleet any
	if fleetID != "" {
		fleet = fleetID
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE servers SET fleet_id=$2 WHERE id=$1`, serverID, fleet)
	if err != nil {
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23503" { // that fleet does not exist
			return ErrNotFound
		}
		return err
	}
	if tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}
