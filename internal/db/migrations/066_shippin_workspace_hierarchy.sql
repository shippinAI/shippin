-- Product workspaces group projects. The historical workspaces table continues
-- to hold compute devspaces; its IDs, files, jobs and runtime routes are unchanged.
CREATE TABLE shippin_workspaces (
  id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  tenant_id uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  name text NOT NULL CHECK (length(btrim(name)) BETWEEN 1 AND 80),
  description text NOT NULL DEFAULT '' CHECK (length(description)<=500),
  is_default boolean NOT NULL DEFAULT false,
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE(id,tenant_id)
);
CREATE UNIQUE INDEX shippin_workspaces_name ON shippin_workspaces(tenant_id,lower(name));
CREATE UNIQUE INDEX shippin_workspaces_default ON shippin_workspaces(tenant_id) WHERE is_default;
INSERT INTO shippin_workspaces(tenant_id,name,is_default,created_by)
  SELECT id,'Default workspace',true,'migration:066' FROM tenants;

CREATE FUNCTION shippin_default_workspace() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  INSERT INTO shippin_workspaces(tenant_id,name,is_default,created_by)
    VALUES(NEW.id,'Default workspace',true,'tenant:create');
  RETURN NEW;
END $$;
CREATE TRIGGER shippin_tenant_workspace AFTER INSERT ON tenants
  FOR EACH ROW EXECUTE FUNCTION shippin_default_workspace();

ALTER TABLE projects ADD COLUMN workspace_id uuid;
UPDATE projects p SET workspace_id=w.id FROM shippin_workspaces w
  WHERE p.tenant_id=w.tenant_id AND w.is_default;
ALTER TABLE projects ALTER COLUMN workspace_id SET NOT NULL;
ALTER TABLE projects ADD CONSTRAINT shippin_project_workspace_tenant
  FOREIGN KEY(workspace_id,tenant_id) REFERENCES shippin_workspaces(id,tenant_id);
CREATE INDEX shippin_projects_workspace ON projects(tenant_id,workspace_id);

-- Existing import/adoption call sites keep their contract and enter the default
-- group. New writes pass an explicit workspace. The composite FK enforces ancestry.
CREATE FUNCTION shippin_project_default_workspace() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  IF NEW.workspace_id IS NULL THEN
    SELECT id INTO NEW.workspace_id FROM shippin_workspaces WHERE tenant_id=NEW.tenant_id AND is_default;
  END IF;
  RETURN NEW;
END $$;
CREATE TRIGGER shippin_project_workspace BEFORE INSERT ON projects
  FOR EACH ROW EXECUTE FUNCTION shippin_project_default_workspace();
