package servers

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// Enrolled is what hostd gets back once: its credential and the beat contract.
type Enrolled struct {
	ServerID         string    `json:"server_id"`
	Credential       string    `json:"credential"`
	ExpiresAt        time.Time `json:"expires_at"`
	HeartbeatSeconds int       `json:"heartbeat_seconds"`
}

// Enroll redeems a token exactly once (the UPDATE is the claim) and mints the host credential.
func (s *Store) Enroll(ctx context.Context, token, hostname, version string, credTTL time.Duration) (*Enrolled, error) {
	if !wellFormed(token, TokenPrefix) {
		return nil, ErrBadToken
	}
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	now := s.now()
	var id, st string
	err = tx.QueryRow(ctx, `UPDATE server_enroll_tokens t SET used_at=$2 FROM servers s
		WHERE t.token_sha256=$1 AND t.used_at IS NULL AND t.voided_at IS NULL AND t.expires_at>$2 AND s.id=t.server_id
		RETURNING s.id::text, s.state`, Hash(token), now).Scan(&id, &st)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrBadToken
	} else if err != nil {
		return nil, err
	}
	if CanEnroll(State(st)) != nil {
		return nil, ErrBadToken
	}
	plain, hash, err := newSecret(CredPrefix)
	if err != nil {
		return nil, err
	}
	exp := now.Add(credTTL)
	if _, err := tx.Exec(ctx, `UPDATE servers SET state='active', cred_sha256=$2, cred_expires_at=$3, last_seen_at=$4,
		enrolled_at=$4, hostname=$5, agent_version=$6 WHERE id=$1`, id, hash, exp, now, hostname, version); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return &Enrolled{ServerID: id, Credential: plain, ExpiresAt: exp, HeartbeatSeconds: int(s.Clock.Interval / time.Second)}, nil
}

// Beat records a heartbeat for a live credential and slides its expiry to now+credTTL.
func (s *Store) Beat(ctx context.Context, cred, version string, credTTL time.Duration) (string, time.Time, error) {
	if !wellFormed(cred, CredPrefix) {
		return "", time.Time{}, ErrBadCred
	}
	now := s.now()
	exp := now.Add(credTTL)
	var id string
	err := s.Pool.QueryRow(ctx, `UPDATE servers SET last_seen_at=$2, cred_expires_at=$3, agent_version=$4
		WHERE cred_sha256=$1 AND cred_expires_at>$2 AND state='active' RETURNING id::text`, Hash(cred), now, exp, version).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, ErrBadCred
	}
	return id, exp, err
}
