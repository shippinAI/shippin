package bridge

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrPending = errors.New("authorization_pending")
	ErrDenied  = errors.New("access_denied")
	ErrExpired = errors.New("expired_token")
)

type Store struct{ Pool *pgxpool.Pool }

// Session is a live bridge session: the account it is bound to and the identity snapshot.
type Session struct {
	ID        string          `json:"id"`
	Client    string          `json:"client_name"`
	Sub       string          `json:"-"`
	Email     string          `json:"email"`
	Identity  json.RawMessage `json:"-"`
	CreatedAt time.Time       `json:"created_at"`
	ExpiresAt *time.Time      `json:"expires_at,omitempty"`
	LastUsed  *time.Time      `json:"last_used_at,omitempty"`
	Status    string          `json:"status"`
}

// Start opens a device sign-in and returns its id, device code and user code.
func (s *Store) Start(ctx context.Context, client string, ttl time.Duration) (string, string, string, error) {
	id, dev, code := uuid.NewString(), secret("dev_"), userCode()
	_, err := s.Pool.Exec(ctx, `INSERT INTO bridge_sessions (id, device_hash, user_code, client_name, device_expires)
		VALUES ($1,$2,$3,$4,now()+$5::interval)`, id, hash(dev), code, client, ttl.String())
	return id, dev, code, err
}

// Pending returns the client name of a live, pending user code ("" if none).
func (s *Store) Pending(ctx context.Context, code string) (string, error) {
	var c string
	err := s.Pool.QueryRow(ctx, `SELECT client_name FROM bridge_sessions WHERE user_code=$1 AND status='pending'
		AND device_expires > now()`, code).Scan(&c)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", nil
	}
	return c, err
}

// Decide approves (binding the account) or denies a pending code. False: no such live code.
func (s *Store) Decide(ctx context.Context, code string, approve bool, sub, email string, ident any, session time.Duration) (bool, error) {
	status, raw := "denied", []byte("null")
	if approve {
		status, raw = "approved", must(json.Marshal(ident))
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE bridge_sessions SET status=$2, sub=$3, email=$4, identity=$5, approved_at=now(),
		expires_at=now()+$6::interval WHERE user_code=$1 AND status='pending' AND device_expires > now()`,
		code, status, sub, email, raw, session.String())
	return tag.RowsAffected() == 1, err
}

// Redeem exchanges an approved device code for a bearer token, once.
func (s *Store) Redeem(ctx context.Context, device, prefix string) (string, *Session, error) {
	var status string
	var exp time.Time
	err := s.Pool.QueryRow(ctx, `SELECT status, device_expires FROM bridge_sessions WHERE device_hash=$1`, hash(device)).Scan(&status, &exp)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return "", nil, ErrExpired
	case err != nil:
		return "", nil, err
	case status == "denied":
		return "", nil, ErrDenied
	case status == "pending" && time.Now().After(exp), status == "issued":
		return "", nil, ErrExpired
	case status == "pending":
		return "", nil, ErrPending
	}
	tok := secret(prefix)
	tag, err := s.Pool.Exec(ctx, `UPDATE bridge_sessions SET status='issued', token_hash=$2 WHERE device_hash=$1 AND status='approved'`, hash(device), hash(tok))
	if err != nil || tag.RowsAffected() != 1 {
		return "", nil, ErrExpired
	}
	ses, err := s.ByToken(ctx, tok)
	return tok, ses, err
}

const cols = `id, client_name, coalesce(sub,''), coalesce(email,''), coalesce(identity,'null'), created_at, expires_at, last_used_at,
	CASE WHEN revoked_at IS NOT NULL THEN 'revoked' WHEN expires_at < now() THEN 'expired' ELSE status END`

func scan(row pgx.Row) (*Session, error) {
	var x Session
	err := row.Scan(&x.ID, &x.Client, &x.Sub, &x.Email, &x.Identity, &x.CreatedAt, &x.ExpiresAt, &x.LastUsed, &x.Status)
	return &x, err
}

// ByToken returns the live session for a bearer token, or nil when it is unknown, expired or revoked.
func (s *Store) ByToken(ctx context.Context, tok string) (*Session, error) {
	x, err := scan(s.Pool.QueryRow(ctx, `UPDATE bridge_sessions SET last_used_at=now() WHERE token_hash=$1 AND status='issued'
		AND revoked_at IS NULL AND expires_at > now() RETURNING `+cols, hash(tok)))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return x, err
}

// List returns an account's bridge sessions, newest first.
func (s *Store) List(ctx context.Context, sub string) ([]Session, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+cols+` FROM bridge_sessions WHERE sub=$1 ORDER BY created_at DESC LIMIT 50`, sub)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Session{}
	for rows.Next() {
		x, err := scan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *x)
	}
	return out, rows.Err()
}

// Revoke ends one of the account's sessions now. False: not found.
func (s *Store) Revoke(ctx context.Context, sub, id string) (bool, error) {
	tag, err := s.Pool.Exec(ctx, `UPDATE bridge_sessions SET revoked_at=now() WHERE id=$1 AND sub=$2 AND revoked_at IS NULL`, id, sub)
	return tag.RowsAffected() == 1, err
}

func must(b []byte, _ error) []byte { return b }
