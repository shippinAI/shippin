package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// StaffRequest is a high-risk team change waiting for a second Owner (two-person rule).
type StaffRequest struct {
	ID            string          `json:"id"`
	Kind          string          `json:"kind"` // roles | invite
	Target        string          `json:"target"`
	TargetName    string          `json:"target_name"`
	Payload       json.RawMessage `json:"payload"`
	Reason        string          `json:"reason"`
	RequestedBy   string          `json:"requested_by"`
	RequestedName string          `json:"requested_name"`
	Status        string          `json:"status"`
	CreatedAt     time.Time       `json:"created_at"`
}

// CreateStaffRequest stores a pending request with its audit entries.
func (d *DB) CreateStaffRequest(ctx context.Context, q StaffRequest, es ...AuditEntry) error {
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `INSERT INTO staff_requests(id, kind, target, target_name, payload, reason, requested_by, requested_name)
			VALUES($1, $2, $3, $4, $5, $6, $7, $8)`, q.ID, q.Kind, q.Target, q.TargetName, []byte(q.Payload), q.Reason, q.RequestedBy, q.RequestedName)
		return tag.RowsAffected(), err
	})
}

// StaffRequests lists pending requests, oldest first.
func (d *DB) StaffRequests(ctx context.Context) ([]StaffRequest, error) {
	rows, err := d.Pool.Query(ctx, `SELECT id::text, kind, target, target_name, payload, reason, requested_by, requested_name, status, created_at
		FROM staff_requests WHERE status='pending' ORDER BY created_at`)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (StaffRequest, error) {
		var q StaffRequest
		var p []byte
		err := r.Scan(&q.ID, &q.Kind, &q.Target, &q.TargetName, &p, &q.Reason, &q.RequestedBy, &q.RequestedName, &q.Status, &q.CreatedAt)
		q.Payload = p
		return q, err
	})
}

// StaffRequestByID returns one request (any status).
func (d *DB) StaffRequestByID(ctx context.Context, id string) (*StaffRequest, error) {
	var q StaffRequest
	var p []byte
	err := d.Pool.QueryRow(ctx, `SELECT id::text, kind, target, target_name, payload, reason, requested_by, requested_name, status, created_at
		FROM staff_requests WHERE id::text=$1`, id).Scan(&q.ID, &q.Kind, &q.Target, &q.TargetName, &p, &q.Reason, &q.RequestedBy, &q.RequestedName, &q.Status, &q.CreatedAt)
	if err == pgx.ErrNoRows {
		return nil, ErrNotFound
	}
	q.Payload = p
	return &q, err
}

// DecideStaffRequest moves a pending request to status (approved, rejected, cancelled), audited.
func (d *DB) DecideStaffRequest(ctx context.Context, id, status, by string, es ...AuditEntry) error {
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `UPDATE staff_requests SET status=$2, decided_by=$3, decided_at=now()
			WHERE id::text=$1 AND status='pending'`, id, status, by)
		return tag.RowsAffected(), err
	})
}
