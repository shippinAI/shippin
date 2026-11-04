package catalog

import (
	"context"
	"encoding/json"
	"strings"
)

// Service is a catalog row: something Shippin can turn on for a tenant.
type Service struct {
	ID               string            `json:"id"`
	Label            string            `json:"label"`
	Description      string            `json:"description"`
	Kind             string            `json:"kind"`
	DocsURL          string            `json:"docs_url"`
	ConfigSchema     json.RawMessage   `json:"config_schema"`
	DefaultConfig    json.RawMessage   `json:"default_config"`
	DeploymentSource *DeploymentSource `json:"deployment_source"`
	Required         bool              `json:"required"`
	Active           bool              `json:"active"`
}

const serviceCols = `id, label, description, kind, docs_url, config_schema, default_config, deployment_source, required, active, updated_at`

func (s *Store) Services(ctx context.Context) (json.RawMessage, error) {
	return s.listJSON(ctx, `SELECT `+serviceCols+` FROM catalog_services ORDER BY required DESC, id`)
}

// DeployableService is the tenant-safe view used to configure a new deployment. Source
// credentials and secret-reference defaults are intentionally omitted.
type DeployableService struct {
	ID               string                  `json:"id"`
	Label            string                  `json:"label"`
	Description      string                  `json:"description"`
	Kind             string                  `json:"kind"`
	DocsURL          string                  `json:"docs_url"`
	ConfigSchema     json.RawMessage         `json:"config_schema"`
	DefaultConfig    json.RawMessage         `json:"default_config"`
	DeploymentSource *PublicDeploymentSource `json:"deployment_source"`
}

type PublicDeploymentSource struct {
	Kind        string `json:"kind"`
	Reference   string `json:"reference"`
	Commit      string `json:"commit,omitempty"`
	ContextPath string `json:"context_path,omitempty"`
}

func (s *Store) DeployableServices(ctx context.Context) ([]DeployableService, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id,label,description,kind,docs_url,config_schema::text,default_config::text,deployment_source::text
		FROM catalog_services WHERE active=true AND deployment_source IS NOT NULL ORDER BY kind,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := make([]DeployableService, 0)
	for rows.Next() {
		var item DeployableService
		var schema, defaults, source []byte
		if err := rows.Scan(&item.ID, &item.Label, &item.Description, &item.Kind, &item.DocsURL, &schema, &defaults, &source); err != nil {
			return nil, err
		}
		item.ConfigSchema = RedactSecretReferences(schema)
		item.DefaultConfig = RedactSecretReferences(defaults)
		var pinned DeploymentSource
		if err := json.Unmarshal(source, &pinned); err != nil {
			return nil, err
		}
		item.DeploymentSource = &PublicDeploymentSource{Kind: pinned.Kind, Reference: pinned.Reference,
			Commit: pinned.Commit, ContextPath: pinned.ContextPath}
		items = append(items, item)
	}
	return items, rows.Err()
}

func RedactSecretReferences(raw []byte) json.RawMessage {
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return json.RawMessage(`{}`)
	}
	value = scrubSecretReference(value)
	result, err := json.Marshal(value)
	if err != nil {
		return json.RawMessage(`{}`)
	}
	return result
}

func scrubSecretReference(value any) any {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if text, ok := child.(string); ok && (strings.HasPrefix(text, "vaultix://") || strings.HasPrefix(text, "sops://")) {
				delete(typed, key)
				continue
			}
			typed[key] = scrubSecretReference(child)
		}
		return typed
	case []any:
		out := make([]any, len(typed))
		for i, child := range typed {
			if text, ok := child.(string); ok && (strings.HasPrefix(text, "vaultix://") || strings.HasPrefix(text, "sops://")) {
				out[i] = ""
				continue
			}
			out[i] = scrubSecretReference(child)
		}
		return out
	default:
		return value
	}
}

func (s *Store) Service(ctx context.Context, id string) (json.RawMessage, error) {
	return s.oneJSON(ctx, `SELECT `+serviceCols+` FROM catalog_services WHERE id=$1`, id)
}

func (s *Store) CreateService(ctx context.Context, v Service) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO catalog_services(id,label,description,kind,docs_url,config_schema,default_config,deployment_source,required,active)
		VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`, v.ID, v.Label, v.Description, v.Kind, v.DocsURL, v.ConfigSchema, v.DefaultConfig, deploymentSourceJSON(v.DeploymentSource), v.Required, v.Active)
	return err
}

func (s *Store) UpdateService(ctx context.Context, v Service) error {
	return s.exec(ctx, `UPDATE catalog_services SET label=$2, description=$3, kind=$4, docs_url=$5, config_schema=$6,
		default_config=$7, deployment_source=$8, required=$9, active=$10, updated_at=now() WHERE id=$1`,
		v.ID, v.Label, v.Description, v.Kind, v.DocsURL, v.ConfigSchema, v.DefaultConfig, deploymentSourceJSON(v.DeploymentSource), v.Required, v.Active)
}

func deploymentSourceJSON(source *DeploymentSource) json.RawMessage {
	if source == nil {
		return nil
	}
	b, _ := json.Marshal(source)
	return json.RawMessage(b)
}

func (s *Store) DeleteService(ctx context.Context, id string) error {
	return s.exec(ctx, `DELETE FROM catalog_services WHERE id=$1`, id)
}

// PlanService is one cell of the plan x service matrix.
type PlanService struct {
	PlanID    string          `json:"plan_id"`
	ServiceID string          `json:"service_id"`
	Enabled   bool            `json:"enabled"`
	Lock      string          `json:"lock"`
	Limits    json.RawMessage `json:"limits"`
}

func (s *Store) PlanServices(ctx context.Context) (json.RawMessage, error) {
	return s.listJSON(ctx, `SELECT plan_id, service_id, enabled, lock, limits, updated_at FROM plan_services ORDER BY plan_id, service_id`)
}

// PutPlanService upserts a matrix cell.
func (s *Store) PutPlanService(ctx context.Context, p PlanService) error {
	_, err := s.Pool.Exec(ctx, `INSERT INTO plan_services(plan_id,service_id,enabled,lock,limits) VALUES($1,$2,$3,$4,$5)
		ON CONFLICT (plan_id,service_id) DO UPDATE SET enabled=$3, lock=$4, limits=$5, updated_at=now()`,
		p.PlanID, p.ServiceID, p.Enabled, p.Lock, p.Limits)
	return err
}

func (s *Store) DeletePlanService(ctx context.Context, plan, svc string) error {
	return s.exec(ctx, `DELETE FROM plan_services WHERE plan_id=$1 AND service_id=$2`, plan, svc)
}
