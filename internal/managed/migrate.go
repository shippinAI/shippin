package managed

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

// LegacyPending reports whether tenant still has pre-D36 per-project instances that were not moved
// (scope 'project', no migrate_to). While it does, the tenant keeps using them as they are and no
// shared ('vm') instance is created for it: the move happens only through MigrateTenant.
func (s *Store) LegacyPending(ctx context.Context, tenant string) (bool, error) {
	var ok bool
	err := s.Pool.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM service_instances WHERE tenant_id::text=$1 AND scope='project'
	  AND migrate_to IS NULL AND state NOT IN ('deleting','migrated'))`, tenant).Scan(&ok)
	return ok, err
}

// MigrateTenant is the per-tenant data step of D36 (formerly the body of 026_rrflow_per_vm.sql, which
// is schema only now), for ONE tenant, in one transaction, idempotent: each boundary of tenant gets
// one shared ('vm') instance, every pre-D36 project becomes a namespace in it, bound devspaces get a
// binding against it, the old instances are marked 'migrating' (migrate_to set), the tenant's queued
// per-project provision jobs are retired and one service.provision job per shared instance is queued.
// Nothing is deleted. Returns the shared instance ids that got a job. No other tenant's row is read
// for writing.
func (s *Store) MigrateTenant(ctx context.Context, tenant, by string) ([]string, error) {
	tx, err := s.Pool.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	steps := []string{
		`UPDATE service_bindings b SET project_id = i.project_id FROM service_instances i
		  WHERE b.instance_id = i.id AND b.project_id IS NULL AND i.scope = 'project' AND i.tenant_id::text = $1`,
		`INSERT INTO service_instances(id, tenant_id, project_id, boundary_id, kind, scope, created_by)
		 SELECT gen_random_uuid(), tenant_id, NULL, boundary_id, kind, 'vm', 'setup-migrate:' || $2
		  FROM service_instances WHERE tenant_id::text = $1 AND scope = 'project' AND boundary_id IS NOT NULL AND state <> 'deleting'
		  GROUP BY tenant_id, boundary_id, kind
		 ON CONFLICT (boundary_id, kind) WHERE scope = 'vm' DO NOTHING`,
		`INSERT INTO service_namespaces(instance_id, project_id, tenant_id)
		 SELECT v.id, o.project_id, o.tenant_id FROM service_instances o
		  JOIN service_instances v ON v.scope = 'vm' AND v.boundary_id = o.boundary_id AND v.kind = o.kind
		  WHERE o.tenant_id::text = $1 AND o.scope = 'project' AND o.state <> 'deleting'
		 ON CONFLICT DO NOTHING`,
		`INSERT INTO service_bindings(id, tenant_id, instance_id, workspace_id, project_id)
		 SELECT gen_random_uuid(), b.tenant_id, v.id, b.workspace_id, o.project_id FROM service_bindings b
		  JOIN service_instances o ON o.id = b.instance_id AND o.scope = 'project' AND o.state <> 'deleting'
		  JOIN service_instances v ON v.scope = 'vm' AND v.boundary_id = o.boundary_id AND v.kind = o.kind
		  WHERE o.tenant_id::text = $1 AND b.state <> 'removing'
		 ON CONFLICT (instance_id, workspace_id) DO NOTHING`,
		`UPDATE service_instances o SET state = 'migrating', migrate_to = v.id, updated_at = now()
		  FROM service_instances v
		  WHERE o.tenant_id::text = $1 AND o.scope = 'project' AND o.state NOT IN ('deleting','migrated') AND o.migrate_to IS NULL
		    AND v.scope = 'vm' AND v.boundary_id = o.boundary_id AND v.kind = o.kind`,
		`UPDATE jobs SET status = 'canceled', error = 'retired: per-project rrflow (Notion D36)', finished_at = now(), updated_at = now()
		  WHERE tenant_id::text = $1 AND kind = 'service.provision' AND status = 'queued'
		    AND target_id IN (SELECT id::text FROM service_instances WHERE tenant_id::text = $1 AND scope = 'project')`,
	}
	for _, q := range steps {
		args := []any{tenant}
		if strings.Contains(q, "$2") {
			args = append(args, by)
		}
		if _, err := tx.Exec(ctx, q, args...); err != nil {
			return nil, err
		}
	}
	rows, err := tx.Query(ctx, `INSERT INTO jobs(id, tenant_id, kind, target_type, target_id, created_by)
	  SELECT gen_random_uuid(), v.tenant_id, 'service.provision', 'service_instance', v.id::text, 'setup-migrate:' || $2
	   FROM service_instances v
	   WHERE v.tenant_id::text = $1 AND v.scope = 'vm' AND EXISTS (SELECT 1 FROM service_instances o WHERE o.migrate_to = v.id AND o.state = 'migrating')
	     AND NOT EXISTS (SELECT 1 FROM jobs j WHERE j.target_id = v.id::text AND j.status IN ('queued','running'))
	  RETURNING target_id`, tenant, by)
	if err != nil {
		return nil, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		return nil, err
	}
	return ids, tx.Commit(ctx)
}
