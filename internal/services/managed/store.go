// Package managed stores managed service instances and their bindings (020_services.sql;
// 07-multi-tenancy "service instance si-<uuid>", "binding sb-<uuid>"). rrflow/state is the first kind.
package managed

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

// KindRRFlow is rrflow/state (02c-rrflow.md).
const KindRRFlow = "rrflow-state"

var ErrNotFound = errors.New("service instance not found")

// Instance is one service instance. Its endpoint (an inner address on the boundary VM) is internal.
type Instance struct {
	ID        string          `json:"id"`
	TenantID  string          `json:"tenant_id"`
	ProjectID string          `json:"project_id,omitempty"` // scope "project" only (pre-D36)
	Scope     string          `json:"scope"`                // "vm": one per tenant VM (Notion D36)
	MigrateTo string          `json:"migrate_to,omitempty"` // pre-D36 instance moving into this vm instance
	Kind      string          `json:"kind"`
	State     string          `json:"state"`
	Error     string          `json:"error,omitempty"`
	Endpoint  json.RawMessage `json:"-"`
	CreatedAt time.Time       `json:"created_at"`
	Bindings  []Binding       `json:"bindings"`
}

// Binding attaches an instance to one workspace (a devspace).
type Binding struct {
	ID          string `json:"id"`
	WorkspaceID string `json:"workspace_id"`
	ProjectID   string `json:"project_id,omitempty"` // the namespace the workspace uses
	State       string `json:"state"`
	Error       string `json:"error,omitempty"`
}

type Store struct{ Pool *pgxpool.Pool }

const cols = `i.id::text, i.tenant_id::text, coalesce(i.project_id::text,''), i.scope, coalesce(i.migrate_to::text,''), i.kind, i.state, coalesce(i.error,''), i.endpoint, i.created_at,
  (SELECT coalesce(json_agg(json_build_object('id',b.id,'workspace_id',b.workspace_id,'project_id',coalesce(b.project_id::text,''),'state',b.state,'error',coalesce(b.error,''))
     ORDER BY b.created_at),'[]') FROM service_bindings b WHERE b.instance_id=i.id AND b.state<>'removing')`

func scan(row pgx.Row) (*Instance, error) {
	var x Instance
	var b []byte
	err := row.Scan(&x.ID, &x.TenantID, &x.ProjectID, &x.Scope, &x.MigrateTo, &x.Kind, &x.State, &x.Error, &x.Endpoint, &x.CreatedAt, &b)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err == nil {
		err = json.Unmarshal(b, &x.Bindings)
	}
	return &x, err
}

// Get returns one instance of tenant.
func (s *Store) Get(ctx context.Context, tenant, id string) (*Instance, error) {
	return scan(s.Pool.QueryRow(ctx, `SELECT `+cols+` FROM service_instances i WHERE i.tenant_id=$1 AND i.id::text=$2`, tenant, id))
}

// SetState records provisioning progress (endpoint nil keeps the old one).
func (s *Store) SetState(ctx context.Context, id, state string, endpoint any, msg string) error {
	var ep []byte
	if endpoint != nil {
		ep, _ = json.Marshal(endpoint)
	}
	_, err := s.Pool.Exec(ctx, `UPDATE service_instances SET state=$2, error=NULLIF($3,''), updated_at=now(),
	  endpoint=coalesce($4::jsonb, endpoint) WHERE id::text=$1`, id, state, msg, ep)
	return err
}

// SetBinding records a binding's state.
func (s *Store) SetBinding(ctx context.Context, id, state, msg string) error {
	_, err := s.Pool.Exec(ctx, `UPDATE service_bindings SET state=$2, error=NULLIF($3,''), updated_at=now() WHERE id::text=$1`, id, state, msg)
	return err
}
