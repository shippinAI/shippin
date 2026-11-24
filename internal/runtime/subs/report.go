package subs

import (
	"context"
	"sort"

	"github.com/google/uuid"
)

// SetReport applies one complete report atomically. It cannot undo a concurrent
// user revocation: disconnected remains denied until explicit activation.
func (s *Store) SetReport(ctx context.Context, h *Home, statuses map[string]string) error {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(context.Background())
	ids := make([]string, 0, len(statuses))
	for id := range statuses {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, provider := range ids {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		_, err = tx.Exec(ctx, `INSERT INTO subscription_connections(id,tenant_id,account_id,provider_id,host_id,status,checked_at)
		 VALUES ($1,$2,$3,$4,$5,$6,now()) ON CONFLICT (account_id,provider_id,host_id) DO UPDATE
		 SET status=CASE WHEN subscription_connections.status='disconnected' THEN 'disconnected' ELSE EXCLUDED.status END,
		 checked_at=now(),updated_at=now()`, id.String(), h.TenantID, h.AccountID, provider, h.ID, statuses[provider])
		if err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
