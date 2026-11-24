// Package projects stores the M1 hierarchy below a tenant: hidden boundary VMs, projects placed
// on them, and environments (SHIPPIN-CLOUD/07-multi-tenancy.md). Every object is a UUID row first.
package projects

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var ErrNotFound = errors.New("not found")

type Store struct{ Pool *pgxpool.Pool }

func (s *Store) list(ctx context.Context, q string, args ...any) (json.RawMessage, error) {
	var out json.RawMessage
	err := s.Pool.QueryRow(ctx, `SELECT coalesce(json_agg(t),'[]') FROM (`+q+`) t`, args...).Scan(&out)
	return out, err
}

func (s *Store) one(ctx context.Context, q string, args ...any) (json.RawMessage, error) {
	var out json.RawMessage
	err := s.Pool.QueryRow(ctx, `SELECT row_to_json(t) FROM (`+q+`) t`, args...).Scan(&out)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	return out, err
}

// projectCols is the user-facing project view: the boundary is reported only as placed or not.
const projectCols = `SELECT p.id, p.workspace_id, p.name, p.state, p.created_at, (p.boundary_id IS NOT NULL) AS placed,
  (SELECT coalesce(json_agg(json_build_object('id',e.id,'name',e.name) ORDER BY e.created_at),'[]')
     FROM environments e WHERE e.project_id=p.id) AS environments,
  (SELECT count(*) FROM workspaces w WHERE w.project_id=p.id AND w.state<>'deleted') AS workspaces
  FROM projects p`

func (s *Store) Projects(ctx context.Context, tenant string) (json.RawMessage, error) {
	return s.list(ctx, projectCols+` WHERE p.tenant_id=$1 AND p.state<>'deleting' ORDER BY p.created_at`, tenant)
}

func (s *Store) Project(ctx context.Context, tenant, id string) (json.RawMessage, error) {
	if uuid.Validate(id) != nil {
		return nil, ErrNotFound
	}
	return s.one(ctx, projectCols+` WHERE p.tenant_id=$1 AND p.id=$2`, tenant, id)
}

// CreateProject inserts the project with its first environment, auto-placed on the tenant's
// boundary with the fewest projects (the operator 2026-10-18: "auto-placed where there's room"). It is
// ready only when that boundary is ready; on a planned/provisioning boundary, or with none yet,
// it stays pending until MarkBoundaryReady/AdoptVM places it. A duplicate name is ErrNameTaken.
func (s *Store) CreateProject(ctx context.Context, tenant, name, env, actor string) (string, error) {
	return s.CreateProjectInWorkspace(ctx, tenant, "", name, env, actor)
}

func (s *Store) CreateProjectInWorkspace(ctx context.Context, tenant, workspace, name, env, actor string) (string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if workspace != "" && uuid.Validate(workspace) != nil {
		return "", ErrNotFound
	}
	var group string
	err = tx.QueryRow(ctx, `SELECT id::text FROM shippin_workspaces WHERE tenant_id=$1 AND (id=nullif($2,'')::uuid OR ($2='' AND is_default)) FOR KEY SHARE`, tenant, workspace).Scan(&group)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", ErrNotFound
	}
	if err != nil {
		return "", err
	}
	var b, bstate *string
	err = tx.QueryRow(ctx, `SELECT b.id::text, b.state FROM boundaries b WHERE b.tenant_id=$1 AND b.state IN ('planned','provisioning','ready')
	  ORDER BY (b.state='ready') DESC, (SELECT count(*) FROM projects p WHERE p.boundary_id=b.id), b.created_at LIMIT 1`, tenant).Scan(&b, &bstate)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	id, _ := uuid.NewV7()
	eid, _ := uuid.NewV7()
	state := "pending"
	if bstate != nil && *bstate == "ready" {
		state = "ready"
	}
	if _, err := tx.Exec(ctx, `INSERT INTO projects(id,tenant_id,boundary_id,name,state,created_by,workspace_id) VALUES($1,$2,$3,$4,$5,$6,$7)`,
		id, tenant, b, name, state, actor, group); err != nil {
		return "", uniqueViolation(err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO environments(id,project_id,tenant_id,name) VALUES($1,$2,$3,$4)`, eid, id, tenant, env); err != nil {
		return "", uniqueViolation(err)
	}
	return id.String(), tx.Commit(ctx)
}

// DeleteProject marks it deleting; refused while it still has live workspaces.
func (s *Store) DeleteProject(ctx context.Context, tenant, id string) error {
	if uuid.Validate(id) != nil {
		return ErrNotFound
	}
	tag, err := s.Pool.Exec(ctx, `UPDATE projects SET state='deleting', updated_at=now() WHERE tenant_id=$1 AND id=$2
	  AND NOT EXISTS (SELECT 1 FROM workspaces w WHERE w.project_id=$2 AND w.state<>'deleted')`, tenant, id)
	if err == nil && tag.RowsAffected() == 0 {
		return ErrNotFound
	}
	return err
}
