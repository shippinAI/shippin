package servers

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Issued is returned once to the admin who asked; Token is never stored or logged.
type Issued struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
}

// ownRow locks the tenant's own server row and returns its stored state. One merely assigned to it is read-only.
func ownRow(ctx context.Context, tx pgx.Tx, tenant, id string) (State, error) {
	if uuid.Validate(id) != nil {
		return "", ErrNotFound
	}
	var st string
	var owner string
	err := tx.QueryRow(ctx, `SELECT state, tenant_id::text FROM servers WHERE id=$1 AND (tenant_id=$2 OR EXISTS (SELECT 1 FROM server_assignments a WHERE a.server_id=servers.id AND a.tenant_id=$2)) FOR UPDATE`,
		id, tenant).Scan(&st, &owner)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err == nil && owner != tenant {
		return "", ErrReadOnly
	}
	return State(st), err
}

// IssueToken voids any unused token of the server and issues a fresh single-use one.
func (s *Store) IssueToken(ctx context.Context, tenant, id, by string, ttl time.Duration) (*Issued, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	st, err := ownRow(ctx, tx, tenant, id)
	if err != nil {
		return nil, err
	}
	if err := CanIssue(st); err != nil {
		return nil, err
	}
	plain, hash, err := newSecret(TokenPrefix)
	if err != nil {
		return nil, err
	}
	exp := s.now().Add(ttl).UTC().Truncate(time.Second)
	tid, _ := uuid.NewV7()
	if _, err := tx.Exec(ctx, `UPDATE server_enroll_tokens SET voided_at=now() WHERE server_id=$1 AND used_at IS NULL AND voided_at IS NULL`, id); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO server_enroll_tokens(id, server_id, token_sha256, expires_at, created_by) VALUES($1,$2,$3,$4,$5)`,
		tid, id, hash, exp, by); err != nil {
		return nil, err
	}
	return &Issued{Token: plain, ExpiresAt: exp}, tx.Commit(ctx)
}

// Revoke ends the server: credential dropped, unused tokens voided. Terminal.
func (s *Store) Revoke(ctx context.Context, tenant, id string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	st, err := ownRow(ctx, tx, tenant, id)
	if err != nil {
		return err
	}
	if err := CanRevoke(st); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE servers SET state='revoked', revoked_at=now(), cred_sha256=NULL, cred_expires_at=NULL WHERE id=$1`, id); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, `UPDATE server_enroll_tokens SET voided_at=now() WHERE server_id=$1 AND used_at IS NULL AND voided_at IS NULL`, id); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
