-- R-J1a: projects carry their Nomen org, a stable public id, an org-unique slug, the owner, and
-- placement/limits/cost_center so container <-> dedicated-VM is a data change plus a migration job.
-- Existing rows: org_id is filled from tenants.idp_org_id and slug from name; public_id stays NULL
-- until the next write (the API backfills; NULLs don't collide in the unique index).
-- Rollback (not embedded, run by hand): internal/db/rollback/027_project_org.down.sql
ALTER TABLE projects ADD COLUMN IF NOT EXISTS public_id   text;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS org_id      text;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS slug        text;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS owner_sub   text;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS placement   text NOT NULL DEFAULT 'container-in-tenant-vm';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS limits      jsonb NOT NULL DEFAULT '{}';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS cost_center text NOT NULL DEFAULT '';
-- Where the project came from: a devspace folder (adopt) and/or its git remote (import / link).
ALTER TABLE projects ADD COLUMN IF NOT EXISTS source_path   text;
ALTER TABLE projects ADD COLUMN IF NOT EXISTS source_remote text NOT NULL DEFAULT '';
ALTER TABLE projects ADD COLUMN IF NOT EXISTS source_workspace_id uuid;  -- the devspace holding source_path

UPDATE projects p SET org_id = t.idp_org_id FROM tenants t WHERE p.tenant_id = t.id AND p.org_id IS NULL;
UPDATE projects SET slug = name WHERE slug IS NULL;

DO $$ BEGIN
  ALTER TABLE projects ADD CONSTRAINT projects_placement_chk
    CHECK (placement IN ('container-in-tenant-vm', 'dedicated-vm'));
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
  ALTER TABLE projects ADD CONSTRAINT projects_public_id_chk
    CHECK (public_id IS NULL OR public_id ~ '^prj_[0-9A-HJKMNP-TV-Z]{26}$');
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
  ALTER TABLE projects ADD CONSTRAINT projects_limits_chk
    CHECK (jsonb_typeof(limits) = 'object' AND limits - '{cpu,mem,disk,net}'::text[] = '{}'::jsonb);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;
DO $$ BEGIN
  ALTER TABLE projects ADD CONSTRAINT projects_cost_center_chk CHECK (length(cost_center) <= 64);
EXCEPTION WHEN duplicate_object THEN NULL; END $$;

CREATE UNIQUE INDEX IF NOT EXISTS projects_public_id_uq ON projects(public_id) WHERE public_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS projects_org_slug_uq ON projects(org_id, slug) WHERE org_id IS NOT NULL AND state <> 'deleting';
CREATE UNIQUE INDEX IF NOT EXISTS projects_source_path_uq ON projects(tenant_id, source_path) WHERE source_path IS NOT NULL AND state <> 'deleting';
