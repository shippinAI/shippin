package subs

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Connection is the status of one user's own subscription with one provider, on one AI home.
// There is deliberately no token field anywhere in this type.
type Connection struct {
	ProviderID string     `json:"provider"`
	HomeID     string     `json:"home_id"`
	TenantID   string     `json:"tenant_id"`
	Status     string     `json:"status"`
	PlanLabel  string     `json:"plan,omitempty"`
	CheckedAt  *time.Time `json:"checked_at,omitempty"`
}

// Statuses a connection can be in (017_subscriptions.sql).
const (
	NotConnected = "not_connected"
	Activating   = "activating"
	Connected    = "connected"
	NeedsReauth  = "needs_reauth"
	Disconnected = "disconnected"
	Limited      = "limited"
)

// SetStatus upserts the account's connection status for provider on home.
func (s *Store) SetStatus(ctx context.Context, h *Home, provider, status, plan string, checked bool) error {
	id, _ := uuid.NewV7()
	_, err := s.Pool.Exec(ctx, `INSERT INTO subscription_connections(id, tenant_id, account_id, provider_id, host_id,
	    status, plan_label, checked_at) VALUES ($1,$2,$3,$4,$5,$6,$7, CASE WHEN $8 THEN now() END)
	  ON CONFLICT (account_id, provider_id, host_id) DO UPDATE SET status=EXCLUDED.status,
	    plan_label=EXCLUDED.plan_label, updated_at=now(),
	    checked_at=CASE WHEN $8 THEN now() ELSE subscription_connections.checked_at END`,
		id.String(), h.TenantID, h.AccountID, provider, h.ID, status, plan, checked)
	return err
}

// Connections lists an account's connections across its AI homes.
func (s *Store) Connections(ctx context.Context, account string) ([]Connection, error) {
	rows, err := s.Pool.Query(ctx, `SELECT provider_id, host_id::text, tenant_id::text, status, plan_label, checked_at
	  FROM subscription_connections WHERE account_id::text=$1 ORDER BY provider_id`, account)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Connection, error) {
		var c Connection
		return c, r.Scan(&c.ProviderID, &c.HomeID, &c.TenantID, &c.Status, &c.PlanLabel, &c.CheckedAt)
	})
}

// Homes lists an account's AI homes.
func (s *Store) Homes(ctx context.Context, account string) ([]Home, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+homeCols+` FROM ai_homes WHERE account_id::text=$1 ORDER BY created_at`, account)
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

// AccountBySub returns the account id of an IdP subject ("" when it has none yet).
func (s *Store) AccountBySub(ctx context.Context, sub string) (string, error) {
	var id string
	err := s.Pool.QueryRow(ctx, `SELECT id::text FROM accounts WHERE idp_sub=$1`, sub).Scan(&id)
	if err == pgx.ErrNoRows {
		return "", nil
	}
	return id, err
}

// HomeByID loads one AI home.
func (s *Store) HomeByID(ctx context.Context, id string) (*Home, error) {
	return scanHome(s.Pool.QueryRow(ctx, `SELECT `+homeCols+` FROM ai_homes WHERE id::text=$1`, id))
}

// SubOf returns the IdP subject of an account.
func (s *Store) SubOf(ctx context.Context, account string) (string, error) {
	var sub string
	return sub, s.Pool.QueryRow(ctx, `SELECT idp_sub FROM accounts WHERE id::text=$1`, account).Scan(&sub)
}
