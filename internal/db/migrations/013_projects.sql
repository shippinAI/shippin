-- M1 hierarchy (SHIPPIN-CLOUD/07-multi-tenancy.md): account a- -> tenant t- -> boundary b- (hidden VM)
-- -> project pj- -> environment e- -> workspace w-. Additive only: existing rows and routes keep
-- working; legacy workspaces keep NULL project/boundary ("legacy direct", vm-first §11).
CREATE TABLE IF NOT EXISTS accounts (
  id          uuid PRIMARY KEY,
  idp_sub     text NOT NULL UNIQUE CHECK (length(idp_sub) BETWEEN 1 AND 200),
  email       text NOT NULL DEFAULT '',
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS tenant_owners (
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  account_id  uuid NOT NULL REFERENCES accounts(id),
  created_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, account_id)
);
-- A boundary is a tenant VM with its own inner Incus. Users never see it; projects are placed on it.
CREATE TABLE IF NOT EXISTS boundaries (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  state       text NOT NULL DEFAULT 'planned' CHECK (state IN ('planned','provisioning','ready','error','deleting')),
  cpu         int  NOT NULL CHECK (cpu > 0),
  memory_mib  int  NOT NULL CHECK (memory_mib > 0),
  root_gib    int  NOT NULL CHECK (root_gib > 0),
  data_gib    int  NOT NULL CHECK (data_gib > 0),
  legacy_vm   text,
  error       text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS boundaries_tenant ON boundaries(tenant_id);
CREATE TABLE IF NOT EXISTS projects (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  boundary_id uuid REFERENCES boundaries(id),
  name        text NOT NULL CHECK (name ~ '^[a-z][a-z0-9-]{0,38}[a-z0-9]$'),
  state       text NOT NULL DEFAULT 'pending' CHECK (state IN ('pending','ready','error','deleting')),
  created_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);
CREATE TABLE IF NOT EXISTS environments (
  id          uuid PRIMARY KEY,
  project_id  uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  name        text NOT NULL CHECK (name ~ '^[a-z][a-z0-9-]{0,22}[a-z0-9]$'),
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (project_id, name)
);
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS project_id uuid REFERENCES projects(id);
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS environment_id uuid REFERENCES environments(id);
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS boundary_id uuid REFERENCES boundaries(id);
-- Plan key: how many boundary VMs a tier gets (founder = 1 for M1, the operator 2026-10-18 15:57 PDT).
ALTER TABLE plans ADD COLUMN IF NOT EXISTS boundaries int NOT NULL DEFAULT 1 CHECK (boundaries >= 0);
