-- Notion D36: the rrflow instance lives at the tenant VM (boundary) and is shared by its
-- containers and projects; each project keeps its own state namespace inside it.
-- SCHEMA ONLY (audit pa2 decision 4): no row of any tenant changes here. Pre-D36 per-project rows
-- keep scope 'project' and keep serving as they are. Each tenant is moved on its own, later, by the
-- operator setup route (POST /v1/operator/tenants/{id}/setup/rrflow -> managed.Store.MigrateTenant):
-- shared ('vm') instance per boundary, projects become namespaces, bindings re-created against it,
-- old instances 'migrating' until the service.provision job copied their state (hostd MigrateRRFlow:
-- snapshot first, never deletes), then 'migrated'; retire-into deletes them later, separately.
ALTER TABLE service_instances ALTER COLUMN project_id DROP NOT NULL;
ALTER TABLE service_instances ADD COLUMN IF NOT EXISTS scope text NOT NULL DEFAULT 'project' CHECK (scope IN ('project','vm'));
ALTER TABLE service_instances ADD COLUMN IF NOT EXISTS migrate_to uuid REFERENCES service_instances(id) ON DELETE SET NULL;
ALTER TABLE service_instances DROP CONSTRAINT IF EXISTS service_instances_project_id_kind_key;
ALTER TABLE service_instances DROP CONSTRAINT IF EXISTS service_instances_state_check;
ALTER TABLE service_instances ADD CONSTRAINT service_instances_state_check
  CHECK (state IN ('pending','provisioning','ready','error','deleting','migrating','migrated'));
ALTER TABLE service_instances DROP CONSTRAINT IF EXISTS service_instances_scope_shape;
ALTER TABLE service_instances ADD CONSTRAINT service_instances_scope_shape CHECK (
  (scope = 'project' AND project_id IS NOT NULL) OR (scope = 'vm' AND project_id IS NULL AND boundary_id IS NOT NULL));
CREATE UNIQUE INDEX IF NOT EXISTS service_instances_project_kind ON service_instances(project_id, kind) WHERE scope = 'project';
CREATE UNIQUE INDEX IF NOT EXISTS service_instances_vm_kind ON service_instances(boundary_id, kind) WHERE scope = 'vm';

CREATE TABLE IF NOT EXISTS service_namespaces (
  instance_id uuid NOT NULL REFERENCES service_instances(id) ON DELETE CASCADE,
  project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (instance_id, project_id)
);
ALTER TABLE service_bindings ADD COLUMN IF NOT EXISTS project_id uuid REFERENCES projects(id) ON DELETE CASCADE;
