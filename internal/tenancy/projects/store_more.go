package projects

import (
	"context"
	"encoding/json"

	"github.com/google/uuid"
)

func (s *Store) AddEnvironment(ctx context.Context, tenant, project, name string) (string, error) {
	if uuid.Validate(project) != nil {
		return "", ErrNotFound
	}
	id, _ := uuid.NewV7()
	tag, err := s.Pool.Exec(ctx, `INSERT INTO environments(id,project_id,tenant_id,name)
	  SELECT $1,p.id,p.tenant_id,$3 FROM projects p WHERE p.tenant_id=$4 AND p.id=$2 AND p.state<>'deleting'`, id, project, name, tenant)
	if err != nil {
		return "", uniqueViolation(err)
	}
	if tag.RowsAffected() == 0 {
		return "", ErrNotFound
	}
	return id.String(), nil
}

func (s *Store) Workspaces(ctx context.Context, tenant, project string) (json.RawMessage, error) {
	if uuid.Validate(project) != nil {
		return nil, ErrNotFound
	}
	return s.list(ctx, `SELECT w.id, w.name, w.state, w.environment_id, w.created_at FROM workspaces w
	  WHERE w.tenant_id=$1 AND w.project_id=$2 AND w.state<>'deleted' ORDER BY w.created_at`, tenant, project)
}

// Placement is what a new workspace in a project needs: its boundary and environment rows.
type Placement struct {
	Boundary *string
	EnvID    string
}

// Place resolves project + environment name for a new workspace; only ready (placed) projects.
func (s *Store) Place(ctx context.Context, tenant, project, env string) (Placement, error) {
	var p Placement
	if uuid.Validate(project) != nil {
		return p, ErrNotFound
	}
	err := s.Pool.QueryRow(ctx, `SELECT p.boundary_id::text, e.id::text FROM projects p JOIN environments e ON e.project_id=p.id
	  WHERE p.tenant_id=$1 AND p.id=$2 AND e.name=$3 AND p.state='ready'`, tenant, project, env).Scan(&p.Boundary, &p.EnvID)
	if err != nil {
		return p, ErrNotFound
	}
	return p, nil
}

// Boundaries is the operator view (users never see boundaries).
func (s *Store) Boundaries(ctx context.Context) (json.RawMessage, error) {
	return s.list(ctx, `SELECT b.id, b.tenant_id, t.name AS tenant, b.state, b.cpu, b.memory_mib, b.root_gib, b.data_gib,
	  b.legacy_vm, b.created_at, (SELECT count(*) FROM projects p WHERE p.boundary_id=b.id AND p.state<>'deleting') AS projects
	  FROM boundaries b JOIN tenants t ON t.id=b.tenant_id ORDER BY b.created_at`)
}

// PlanBoundary records a boundary for a tenant from its plan size (state planned; hostd provisions).
func (s *Store) PlanBoundary(ctx context.Context, tenant string, cpu, memMiB, rootGiB, dataGiB int) (string, error) {
	id, _ := uuid.NewV7()
	_, err := s.Pool.Exec(ctx, `INSERT INTO boundaries(id,tenant_id,cpu,memory_mib,root_gib,data_gib) VALUES($1,$2,$3,$4,$5,$6)`,
		id, tenant, cpu, memMiB, rootGiB, dataGiB)
	return id.String(), err
}
