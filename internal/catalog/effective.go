package catalog

import (
	"context"
	"encoding/json"
	"strings"
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

// Effective loads every layer for the tenant and merges them.
func (s *Store) Effective(ctx context.Context, tenant, workspace string) (*EffectiveConfig, error) {
	plan, err := s.CustomerPlan(ctx, tenant)
	if err != nil {
		return nil, err
	}
	items, err := s.layerItems(ctx, plan, tenant, workspace)
	if err != nil {
		return nil, err
	}
	svcs, err := s.serviceRows(ctx)
	if err != nil {
		return nil, err
	}
	var cells []PlanService
	if err := s.decode(ctx, `SELECT plan_id, service_id, enabled, lock, limits FROM plan_services WHERE plan_id=$1`, &cells, plan); err != nil {
		return nil, err
	}
	ts, err := s.TenantServices(ctx, tenant)
	if err != nil {
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

func (s *Store) serviceRows(ctx context.Context) ([]Service, error) {
	var out []Service
	err := s.decode(ctx, `SELECT `+serviceCols+` FROM catalog_services`, &out)
	return out, err
}

// decode runs a list query and unmarshals the JSON array into dst.
func (s *Store) decode(ctx context.Context, q string, dst any, args ...any) error {
	raw, err := s.listJSON(ctx, q, args...)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, dst)
}
