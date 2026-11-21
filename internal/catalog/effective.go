package catalog

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/jackc/pgx/v5"
)

// EffectiveConfig is the merged view for one tenant.
type EffectiveConfig struct {
	TenantID  string      `json:"tenant_id"`
	PlanID    string      `json:"plan_id"`
	Workspace string      `json:"workspace_id,omitempty"`
	Layers    []string    `json:"layers"`
	Settings  []Effective `json:"settings"`
	Services  []Effective `json:"services"`
	Enabled   []string    `json:"enabled_services"`
}

// Querier is what the effective config is read through: a pool, or an action's own transaction
// (mcpl.DB satisfies it).
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
	QueryRow(ctx context.Context, sql string, args ...any) pgx.Row
}

// TenantService is a tenant's override of one service (nil Enabled = inherit the plan). The customer's
// service actions (internal/tenancy/customer/service) own the rows; the merge reads them.
type TenantService struct {
	TenantID  string          `json:"tenant_id"`
	ServiceID string          `json:"service_id"`
	Enabled   *bool           `json:"enabled"`
	Overrides json.RawMessage `json:"overrides"`
}

// EffectiveFor loads every layer for a customer (system, its plan, itself and optionally one workspace)
// and merges them. It answers ErrNotFound when no customer has this id. The customer actions
// (internal/tenancy/customer/effective_config and provision_intent) read through it.
func EffectiveFor(ctx context.Context, q Querier, tenant, workspace string) (*EffectiveConfig, error) {
	var plan string
	err := q.QueryRow(ctx, `SELECT plan FROM tenants WHERE kind='customer' AND id::text=$1`, tenant).Scan(&plan)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	items, err := layerItems(ctx, q, plan, tenant, workspace)
	if err != nil {
		return nil, err
	}
	// Only what ServiceItems reads; the rest of a catalog service is not part of the merge.
	var svcs []Service
	if err := decodeRows(ctx, q, `SELECT id, label, default_config, required, active FROM catalog_services`, &svcs); err != nil {
		return nil, err
	}
	var cells []PlanService
	if err := decodeRows(ctx, q, `SELECT plan_id, service_id, enabled, lock, limits FROM plan_services WHERE plan_id=$1`, &cells, plan); err != nil {
		return nil, err
	}
	var ts []TenantService
	if err := decodeRows(ctx, q, `SELECT tenant_id::text AS tenant_id, service_id, enabled, overrides FROM tenant_services WHERE tenant_id::text=$1`, &ts, tenant); err != nil {
		return nil, err
	}
	out := &EffectiveConfig{TenantID: tenant, PlanID: plan, Workspace: workspace, Layers: LayerOrder,
		Settings: []Effective{}, Services: []Effective{}, Enabled: []string{}}
	for _, e := range Merge(append(items, ServiceItems(plan, svcs, cells, ts)...)) {
		if !strings.HasPrefix(e.Key, "svc.") {
			out.Settings = append(out.Settings, e)
			continue
		}
		out.Services = append(out.Services, e)
		if strings.HasSuffix(e.Key, ".enabled") && string(e.Value) == "true" {
			out.Enabled = append(out.Enabled, strings.TrimSuffix(strings.TrimPrefix(e.Key, "svc."), ".enabled"))
		}
	}
	return out, nil
}

// layerItems loads the setting rows that apply to a tenant on a plan (workspace optional).
func layerItems(ctx context.Context, q Querier, plan, tenant, workspace string) ([]Item, error) {
	rows, err := q.Query(ctx, `SELECT layer, ref, key, value, lock, bounds, note FROM tenant_defaults
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

// decodeRows runs a list query and unmarshals its rows, as one JSON array, into dst.
func decodeRows(ctx context.Context, q Querier, sql string, dst any, args ...any) error {
	var raw json.RawMessage
	if err := q.QueryRow(ctx, `SELECT coalesce(json_agg(t),'[]') FROM (`+sql+`) t`, args...).Scan(&raw); err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}
