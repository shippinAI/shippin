package managed

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// ErrNoBoundary: the project has no tenant VM yet, so it has nowhere to run managed services.
var ErrNoBoundary = errors.New("project has no tenant VM")

// EnsureVM returns the tenant VM's shared instance of kind (Notion D36), creating it (pending) on
// first use, and adds project as a namespace inside it.
func (s *Store) EnsureVM(ctx context.Context, tenant, project, kind, by string) (*Instance, error) {
	var boundary *string
	err := s.Pool.QueryRow(ctx, `SELECT boundary_id::text FROM projects WHERE tenant_id=$1 AND id::text=$2`, tenant, project).Scan(&boundary)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	if boundary == nil {
		return nil, ErrNoBoundary
	}
	id, _ := uuid.NewV7()
	if _, err := s.Pool.Exec(ctx, `INSERT INTO service_instances(id,tenant_id,boundary_id,kind,scope,created_by)
	  VALUES($1,$2,$3,$4,'vm',$5) ON CONFLICT (boundary_id, kind) WHERE scope='vm' DO NOTHING`, id, tenant, *boundary, kind, by); err != nil {
		return nil, err
	}
	inst, err := scan(s.Pool.QueryRow(ctx, `SELECT `+cols+` FROM service_instances i WHERE i.tenant_id=$1 AND i.boundary_id::text=$2
	  AND i.kind=$3 AND i.scope='vm'`, tenant, *boundary, kind))
	if err != nil {
		return nil, err
	}
	_, err = s.Pool.Exec(ctx, `INSERT INTO service_namespaces(instance_id,project_id,tenant_id) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`,
		inst.ID, project, tenant)
	return inst, err
}

// Bind returns the binding of workspace (using namespace project) to the instance, creating it
// (pending) on first use.
func (s *Store) Bind(ctx context.Context, tenant, instance, workspace, project string) (*Binding, error) {
	id, _ := uuid.NewV7()
	var b Binding
	err := s.Pool.QueryRow(ctx, `INSERT INTO service_bindings(id,tenant_id,instance_id,workspace_id,project_id)
	  VALUES($1,$2,$3,$4,NULLIF($5,'')::uuid)
	  ON CONFLICT (instance_id, workspace_id) DO UPDATE SET state=CASE WHEN service_bindings.state='removing' THEN 'pending'
	    ELSE service_bindings.state END, project_id=coalesce(EXCLUDED.project_id, service_bindings.project_id), updated_at=now()
	  RETURNING id::text, workspace_id::text, coalesce(project_id::text,''), state, coalesce(error,'')`, id, tenant, instance, workspace, project).
		Scan(&b.ID, &b.WorkspaceID, &b.ProjectID, &b.State, &b.Error)
	return &b, err
}

// ByProject lists the instances a project uses: its pre-D36 instance (if any) and the shared
// instance it is a namespace of, with only this project's bindings.
func (s *Store) ByProject(ctx context.Context, tenant, project string) ([]Instance, error) {
	out, err := s.list(ctx, `i.tenant_id=$1 AND i.state NOT IN ('deleting','migrated') AND ((i.scope='project' AND i.project_id::text=$2)
	  OR i.id IN (SELECT n.instance_id FROM service_namespaces n WHERE n.project_id::text=$2))`, tenant, project)
	for k := range out {
		if out[k].Scope == "vm" {
			mine := []Binding{}
			for _, b := range out[k].Bindings {
				if b.ProjectID == project {
					mine = append(mine, b)
				}
			}
			out[k].Bindings = mine
		}
	}
	return out, err
}

// VMInstances lists the tenant's shared (per tenant VM) instances.
func (s *Store) VMInstances(ctx context.Context, tenant string) ([]Instance, error) {
	return s.list(ctx, `i.tenant_id=$1 AND i.scope='vm' AND i.state<>'deleting'`, tenant)
}

// MigratingInto lists pre-D36 instances whose state is moving into shared instance vm.
func (s *Store) MigratingInto(ctx context.Context, tenant, vm string) ([]Instance, error) {
	return s.list(ctx, `i.tenant_id=$1 AND i.migrate_to::text=$2 AND i.state='migrating'`, tenant, vm)
}

// HasNamespace reports whether project is a namespace of instance.
func (s *Store) HasNamespace(ctx context.Context, instance, project string) bool {
	var ok bool
	_ = s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM service_namespaces WHERE instance_id::text=$1 AND project_id::text=$2)`,
		instance, project).Scan(&ok)
	return ok
}

// Namespaces lists the projects inside instance, oldest first.
func (s *Store) Namespaces(ctx context.Context, instance string) ([]string, error) {
	rows, err := s.Pool.Query(ctx, `SELECT project_id::text FROM service_namespaces WHERE instance_id::text=$1 ORDER BY created_at, project_id`, instance)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[string])
}

// Delete removes an instance row (its bindings and namespaces cascade).
func (s *Store) Delete(ctx context.Context, id string) error {
	_, err := s.Pool.Exec(ctx, `DELETE FROM service_instances WHERE id::text=$1`, id)
	return err
}

func (s *Store) list(ctx context.Context, where string, args ...any) ([]Instance, error) {
	rows, err := s.Pool.Query(ctx, `SELECT `+cols+` FROM service_instances i WHERE `+where+` ORDER BY i.created_at`, args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, func(r pgx.CollectableRow) (Instance, error) {
		x, err := scan(r)
		if err != nil {
			return Instance{}, err
		}
		return *x, nil
	})
}
