-- Rollback for migrations/027_project_org.sql. Run by hand, in one transaction, only after the
-- console release that writes these columns has been rolled back. Data in the new columns is lost
-- (owner and org are re-derivable from tenants.idp_org_id and the OpenFGA tuples).
BEGIN;
DROP INDEX IF EXISTS projects_source_path_uq;
DROP INDEX IF EXISTS projects_org_slug_uq;
DROP INDEX IF EXISTS projects_public_id_uq;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_cost_center_chk;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_limits_chk;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_public_id_chk;
ALTER TABLE projects DROP CONSTRAINT IF EXISTS projects_placement_chk;
ALTER TABLE projects DROP COLUMN IF EXISTS source_workspace_id;
ALTER TABLE projects DROP COLUMN IF EXISTS source_remote;
ALTER TABLE projects DROP COLUMN IF EXISTS source_path;
ALTER TABLE projects DROP COLUMN IF EXISTS cost_center;
ALTER TABLE projects DROP COLUMN IF EXISTS limits;
ALTER TABLE projects DROP COLUMN IF EXISTS placement;
ALTER TABLE projects DROP COLUMN IF EXISTS owner_sub;
ALTER TABLE projects DROP COLUMN IF EXISTS slug;
ALTER TABLE projects DROP COLUMN IF EXISTS org_id;
ALTER TABLE projects DROP COLUMN IF EXISTS public_id;
DELETE FROM schema_migrations WHERE version = 'migrations/027_project_org.sql';
COMMIT;
