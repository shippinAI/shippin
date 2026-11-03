-- Managed services attachable to a devspace (07-multi-tenancy: service instance si-<uuid> in a project on
-- its boundary VM; binding sb-<uuid> to a workspace). rrflow/state (02c-rrflow.md) is the first kind.
-- Endpoints hold no secrets; binding tokens live only in the bound workspace (their hash in the instance).
CREATE TABLE IF NOT EXISTS service_instances (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  boundary_id uuid REFERENCES boundaries(id),
  kind        text NOT NULL CHECK (kind IN ('rrflow-state')),
  state       text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','provisioning','ready','error','deleting')),
  endpoint    jsonb NOT NULL DEFAULT '{}',
  error       text,
  created_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, kind)
);
CREATE TABLE IF NOT EXISTS service_bindings (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenants(id),
  instance_id  uuid NOT NULL REFERENCES service_instances(id) ON DELETE CASCADE,
  workspace_id uuid NOT NULL REFERENCES workspaces(id),
  state        text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','bound','error','removing')),
  error        text,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (instance_id, workspace_id)
);
INSERT INTO catalog_services(id,label,description,kind,docs_url,required,config_schema,default_config) VALUES
 ('rrflow','rrflow','Reasoning engine: agent sessions, memory and a hash-chained decision log, on your own private server.',
  'ai','gold-homebase/SHIPPIN-CLOUD/02c-rrflow.md',false,
  '{"type":"object","properties":{"memory_scope":{"type":"string","enum":["workspace"],"title":"Memory scope"}}}',
  '{"memory_scope":"workspace"}')
ON CONFLICT (id) DO NOTHING;
