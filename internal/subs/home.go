// Package subs records each user's AI home (the volume their vendor CLI logins live on) and the
// status of their own AI subscriptions. It never stores credentials: the database refuses a
// token_ref for the hosted runner (017_subscriptions.sql).
package subs

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

// Store reads and writes ai_homes and subscription_connections.
type Store struct{ Pool *pgxpool.Pool }

// Home is one user's AI home in one placement (tenant, boundary; legacy tenants have none).
type Home struct {
	ID           string     `json:"id"`
	TenantID     string     `json:"tenant_id"`
	AccountID    string     `json:"account_id"`
	BoundaryID   string     `json:"boundary_id,omitempty"`
	Volume       string     `json:"volume"`
	State        string     `json:"state"`
	Error        string     `json:"error,omitempty"`
	UtilityHost  string     `json:"utility_host,omitempty"`
	UtilityState string     `json:"utility_state"`
	UtilitySeen  *time.Time `json:"utility_seen,omitempty"`
}

// Volume is the Incus volume name of an account's AI home.
func Volume(account string) string { return "ai-" + account }

// UtilityName is the activation container of an account.
func UtilityName(account string) string { return "u-" + account }

// UtilityLabel is the DNS label of an account's activation terminal host: u- and the last 12 hex
// digits (the random part of a UUIDv7; the leading digits are a timestamp and would collide).
func UtilityLabel(account string) string {
	h := strings.ReplaceAll(account, "-", "")
	return "u-" + h[len(h)-12:]
}

// TerminalLabel binds the public host to both tenant placement and account (128 bits).
func TerminalLabel(tenant, account string) string {
	digest := sha256.Sum256([]byte(tenant + ":" + account))
	return "u-" + hex.EncodeToString(digest[:16])
}

const homeCols = `id::text, tenant_id::text, account_id::text, coalesce(boundary_id::text,''), volume, state,
  coalesce(error,''), coalesce(utility_host,''), utility_state, utility_seen`

func scanHome(row pgx.Row) (*Home, error) {
	h := &Home{}
	err := row.Scan(&h.ID, &h.TenantID, &h.AccountID, &h.BoundaryID, &h.Volume, &h.State, &h.Error,
		&h.UtilityHost, &h.UtilityState, &h.UtilitySeen)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return h, err
}

// EnsureHome returns the account's AI home in tenant/boundary, creating the row (state creating)
// on first use. host is the activation terminal hostname to record.
func (s *Store) EnsureHome(ctx context.Context, tenant, account, boundary, host string) (*Home, error) {
	if h, err := s.HomeFor(ctx, tenant, account, boundary); !errors.Is(err, ErrNotFound) {
		return h, err
	}
	id, _ := uuid.NewV7()
	var b any
	if boundary != "" {
		b = boundary
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO ai_homes(id, tenant_id, account_id, boundary_id, volume, utility_host)
	  VALUES ($1,$2,$3,$4,$5,$6) ON CONFLICT DO NOTHING`, id.String(), tenant, account, b, Volume(account), host)
	if err != nil {
		return nil, err
	}
	return s.HomeFor(ctx, tenant, account, boundary)
}

// HomeFor finds the account's AI home in tenant/boundary.
func (s *Store) HomeFor(ctx context.Context, tenant, account, boundary string) (*Home, error) {
	return scanHome(s.Pool.QueryRow(ctx, `SELECT `+homeCols+` FROM ai_homes WHERE tenant_id::text=$1
	  AND account_id::text=$2 AND coalesce(boundary_id::text,'')=$3`, tenant, account, boundary))
}

// HomeByHost finds the AI home whose activation terminal is served on host.
func (s *Store) HomeByHost(ctx context.Context, host string) (*Home, error) {
	return scanHome(s.Pool.QueryRow(ctx, `SELECT `+homeCols+` FROM ai_homes WHERE utility_host=$1`, host))
}

// SetHomeState records the volume state (ready/error).
func (s *Store) SetHomeState(ctx context.Context, id, state, msg string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE ai_homes SET state=$2, error=nullif($3,''), updated_at=now() WHERE id=$1`, id, state, msg)
	return err
}

// SetUtility records the activation container state; touch=true also marks it as in use now.
func (s *Store) SetUtility(ctx context.Context, id, state string, touch bool) error {
	_, err := s.Pool.Exec(ctx, `UPDATE ai_homes SET utility_state=$2, updated_at=now(),
	  utility_seen = CASE WHEN $3 THEN now() ELSE utility_seen END WHERE id=$1`, id, state, touch)
	return err
}

// IdleUtilities also retries failed activation/cleanup, whose terminal state is uncertain.
func (s *Store) IdleUtilities(ctx context.Context, idle time.Duration) ([]Home, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+homeCols+` FROM ai_homes WHERE utility_state='error'
	  OR (utility_state IN ('running','starting')
	  AND (utility_seen IS NULL OR utility_seen < now()-make_interval(secs => $1)))`, idle.Seconds())
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Home, error) {
		h, err := scanHome(r)
		if err != nil {
			return Home{}, err
		}
		return *h, nil
	})
}
