package db

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// StaffSession is one staff-mode session (023_staff_sessions.sql).
type StaffSession struct {
	ID         string     `json:"id"`
	Actor      string     `json:"actor"`
	Reason     string     `json:"reason"`
	FromTenant string     `json:"from_tenant,omitempty"`
	AuthAt     time.Time  `json:"auth_at"`
	StartedAt  time.Time  `json:"started_at"`
	ExpiresAt  time.Time  `json:"expires_at"`
	EndedAt    *time.Time `json:"ended_at,omitempty"`
}

const staffSessCols = `id::text, actor, reason, coalesce(from_tenant::text,''), auth_at, started_at, expires_at, ended_at`

// StartStaffSession closes any open session of actor and opens a new one, writing e (the audit
// entry) in the same transaction so a session never exists unaudited.
func (d *DB) StartStaffSession(ctx context.Context, s StaffSession, name string, ttl time.Duration, e AuditEntry) (*StaffSession, error) {
	id, _ := uuid.NewV7()
	var from any
	if s.FromTenant != "" {
		from = s.FromTenant
	}
	var out *StaffSession
	err := pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, `UPDATE staff_sessions SET ended_at=now() WHERE actor=$1 AND ended_at IS NULL`, s.Actor); err != nil {
			return err
		}
		row := tx.QueryRow(ctx, `INSERT INTO staff_sessions(id,actor,actor_name,reason,from_tenant,auth_at,expires_at)
		  VALUES ($1,$2,$3,$4,$5,$6,now()+$7::interval) RETURNING `+staffSessCols,
			id, s.Actor, name, s.Reason, from, s.AuthAt, ttl.String())
		var err error
		if out, err = scanStaffSession(row); err != nil {
			return err
		}
		return addAuditTx(ctx, tx, e)
	})
	return out, err
}

// ActiveStaffSession returns actor's open, unexpired session, or nil.
func (d *DB) ActiveStaffSession(ctx context.Context, actor string) (*StaffSession, error) {
	s, err := scanStaffSession(d.Pool.QueryRow(ctx, `SELECT `+staffSessCols+` FROM staff_sessions
	  WHERE actor=$1 AND ended_at IS NULL AND expires_at > now()`, actor))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return s, err
}

// EndStaffSession closes actor's open session (audited in the same transaction); false if none.
func (d *DB) EndStaffSession(ctx context.Context, actor string, e AuditEntry) (bool, error) {
	ended := false
	err := pgx.BeginFunc(ctx, d.Pool, func(tx pgx.Tx) error {
		tag, err := tx.Exec(ctx, `UPDATE staff_sessions SET ended_at=now() WHERE actor=$1 AND ended_at IS NULL`, actor)
		if err != nil || tag.RowsAffected() == 0 {
			return err
		}
		ended = true
		return addAuditTx(ctx, tx, e)
	})
	return ended, err
}

func scanStaffSession(row pgx.Row) (*StaffSession, error) {
	var s StaffSession
	if err := row.Scan(&s.ID, &s.Actor, &s.Reason, &s.FromTenant, &s.AuthAt, &s.StartedAt, &s.ExpiresAt, &s.EndedAt); err != nil {
		return nil, err
	}
	return &s, nil
}
