-- P0-6 RLS groundwork: environments was the only tenant-scoped table without tenant_id.
-- Backfilled from its project; a composite FK keeps it equal to the project's tenant.
-- No policies yet (RLS proper comes after M2).
ALTER TABLE environments ADD COLUMN IF NOT EXISTS tenant_id uuid REFERENCES tenants(id);
UPDATE environments e SET tenant_id = p.tenant_id FROM projects p WHERE p.id = e.project_id AND e.tenant_id IS NULL;
ALTER TABLE environments ALTER COLUMN tenant_id SET NOT NULL;
ALTER TABLE projects ADD CONSTRAINT projects_id_tenant UNIQUE (id, tenant_id);
ALTER TABLE environments ADD CONSTRAINT environments_project_tenant
  FOREIGN KEY (project_id, tenant_id) REFERENCES projects(id, tenant_id);
CREATE INDEX IF NOT EXISTS environments_tenant ON environments(tenant_id);
