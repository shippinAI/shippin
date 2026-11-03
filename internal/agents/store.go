// Package agents stores agent identities (021_agents.sql): IdP service users delegated by a user.
package agents

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// ErrNameTaken: the tenant already has an agent with that name.
var ErrNameTaken = errors.New("agent name taken")

// Agent is one agent identity. IdPUserID is the service user in the tenant's IdP org.
type Agent struct {
	ID               string    `json:"id"`
	TenantID         string    `json:"-"`
	Name             string    `json:"name"`
	WorkspaceID      string    `json:"workspace_id,omitempty"`
	DelegatingUserID string    `json:"delegating_user_id"`
	IdPUserID        string    `json:"idp_user_id"`
	ToolGrants       []string  `json:"tool_grants"`
	CreatedAt        time.Time `json:"created_at"`
}

type Store struct{ Pool *pgxpool.Pool }

// List returns the tenant's agents, newest first.
func (s *Store) List(ctx context.Context, tenant string) ([]Agent, error) {
	rows, err := s.Pool.Query(ctx, `SELECT id, tenant_id, name, coalesce(workspace_id::text,''), delegating_user_id,
		idp_user_id, tool_grants, created_at FROM agent_identities WHERE tenant_id=$1 ORDER BY created_at DESC, id`, tenant)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Agent{}
	for rows.Next() {
		var a Agent
		if err := rows.Scan(&a.ID, &a.TenantID, &a.Name, &a.WorkspaceID, &a.DelegatingUserID, &a.IdPUserID, &a.ToolGrants, &a.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// Count is the tenant's number of agents (for the per-tenant cap).
func (s *Store) Count(ctx context.Context, tenant string) (int, error) {
	var n int
	err := s.Pool.QueryRow(ctx, `SELECT count(*) FROM agent_identities WHERE tenant_id=$1`, tenant).Scan(&n)
	return n, err
}

// NameFree reports whether name is unused in the tenant (checked before the IdP call).
func (s *Store) NameFree(ctx context.Context, tenant, name string) (bool, error) {
	var taken bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM agent_identities WHERE tenant_id=$1 AND name=$2)`, tenant, name).Scan(&taken)
	return !taken, err
}

// Create inserts a; a.ID, a.IdPUserID and a.DelegatingUserID must be set.
func (s *Store) Create(ctx context.Context, a *Agent) error {
	if a.ToolGrants == nil {
		a.ToolGrants = []string{}
	}
	err := s.Pool.QueryRow(ctx, `INSERT INTO agent_identities(id, tenant_id, workspace_id, name, idp_user_id, delegating_user_id, tool_grants)
		VALUES($1,$2,nullif($3,'')::uuid,$4,$5,$6,$7) RETURNING created_at`,
		a.ID, a.TenantID, a.WorkspaceID, a.Name, a.IdPUserID, a.DelegatingUserID, a.ToolGrants).Scan(&a.CreatedAt)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" && pg.ConstraintName == "agent_identities_tenant_id_name_key" {
		return ErrNameTaken
	}
	return err
}
