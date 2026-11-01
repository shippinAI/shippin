package catalog

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

// RecordIntent snapshots the tenant's effective config as the desired state. It stays
// "pending" until a Gold ticket applies it; nothing is provisioned here.
func (s *Store) RecordIntent(ctx context.Context, tenant, actor string, note string) (json.RawMessage, error) {
	eff, err := s.Effective(ctx, tenant, "")
	if err != nil {
		return nil, err
	}
	desired, _ := json.Marshal(map[string]any{"plan_id": eff.PlanID, "enabled_services": eff.Enabled,
		"services": eff.Services, "settings": eff.Settings, "note": note})
	id, _ := uuid.NewV7()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO provision_intents(id,tenant_id,desired,created_by) VALUES($1,$2::uuid,$3,$4)`,
		id.String(), tenant, desired, actor); err != nil {
		return nil, err
	}
	return s.oneJSON(ctx, `SELECT id, tenant_id, status, ticket_ref, created_by, created_at, desired->'enabled_services' AS enabled_services
		FROM provision_intents WHERE id=$1`, id.String())
}

func (s *Store) Intents(ctx context.Context, tenant string) (json.RawMessage, error) {
	return s.listJSON(ctx, `SELECT id, tenant_id, status, ticket_ref, created_by, created_at, desired->'enabled_services' AS enabled_services
		FROM provision_intents WHERE tenant_id::text=$1 ORDER BY created_at DESC LIMIT 50`, tenant)
}
