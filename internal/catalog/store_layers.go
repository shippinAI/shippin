package catalog

import (
	"context"
	"encoding/json"
)

// Defaults lists layered settings, optionally for one layer/ref.
func (s *Store) Defaults(ctx context.Context, layer, ref string) (json.RawMessage, error) {
	return s.listJSON(ctx, `SELECT id, layer, ref, key, value, lock, bounds, note, updated_at FROM tenant_defaults
		WHERE ($1='' OR layer=$1) AND ($2='' OR ref=$2) ORDER BY key, layer`, layer, ref)
}

// PutDefault upserts one layered setting.
func (s *Store) PutDefault(ctx context.Context, it Item) error {
	var bounds any
	if it.Bounds != nil {
		bounds = it.Bounds
	}
	_, err := s.Pool.Exec(ctx, `INSERT INTO tenant_defaults(layer,ref,key,value,lock,bounds,note) VALUES($1,$2,$3,$4,$5,$6,$7)
		ON CONFLICT (layer,ref,key) DO UPDATE SET value=$4, lock=$5, bounds=$6, note=$7, updated_at=now()`,
		it.Layer, it.Ref, it.Key, it.Value, lockOr(it.Lock), bounds, it.Note)
	return err
}

func (s *Store) DeleteDefault(ctx context.Context, layer, ref, key string) error {
	return s.exec(ctx, `DELETE FROM tenant_defaults WHERE layer=$1 AND ref=$2 AND key=$3`, layer, ref, key)
}

// layerItems loads the setting rows that apply to a tenant on a plan (workspace optional).
func (s *Store) layerItems(ctx context.Context, plan, tenant, workspace string) ([]Item, error) {
	rows, err := s.Pool.Query(ctx, `SELECT layer, ref, key, value, lock, bounds, note FROM tenant_defaults
		WHERE layer='system' OR (layer='plan' AND ref=$1) OR (layer='tenant' AND ref=$2) OR (layer='workspace' AND ref=$3 AND $3<>'')`,
		plan, tenant, workspace)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Item{}
	for rows.Next() {
		var it Item
		var b []byte
		if err := rows.Scan(&it.Layer, &it.Ref, &it.Key, &it.Value, &it.Lock, &b, &it.Note); err != nil {
			return nil, err
		}
		if len(b) > 0 && string(b) != "null" {
			it.Bounds = &Bounds{}
			_ = json.Unmarshal(b, it.Bounds)
		}
		out = append(out, it)
	}
	return out, rows.Err()
}

// TenantService is a tenant's override of one service (nil Enabled = inherit the plan).
type TenantService struct {
	TenantID  string          `json:"tenant_id"`
	ServiceID string          `json:"service_id"`
	Enabled   *bool           `json:"enabled"`
	Overrides json.RawMessage `json:"overrides"`
}

func (s *Store) TenantServices(ctx context.Context, tenant string) ([]TenantService, error) {
	rows, err := s.Pool.Query(ctx, `SELECT tenant_id::text, service_id, enabled, overrides FROM tenant_services WHERE tenant_id::text=$1`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []TenantService{}
	for rows.Next() {
		var t TenantService
		if err := rows.Scan(&t.TenantID, &t.ServiceID, &t.Enabled, &t.Overrides); err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) PutTenantService(ctx context.Context, t TenantService) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO tenant_services(tenant_id,service_id,enabled,overrides) VALUES($1::uuid,$2,$3,$4)
		ON CONFLICT (tenant_id,service_id) DO UPDATE SET enabled=$3, overrides=$4, updated_at=now()`,
		t.TenantID, t.ServiceID, t.Enabled, t.Overrides)
	return err
}

func (s *Store) DeleteTenantService(ctx context.Context, tenant, svc string) error {
	return s.exec(ctx, `DELETE FROM tenant_services WHERE tenant_id::text=$1 AND service_id=$2`, tenant, svc)
}
