CREATE TABLE IF NOT EXISTS tenants (
  id          uuid PRIMARY KEY,
  idp_org_id  text NOT NULL UNIQUE,
  name        text NOT NULL,
  plan        text NOT NULL,
  status      text NOT NULL DEFAULT 'provisioning',
  ssh_ip      text,
  error       text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS workspaces (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  name        text NOT NULL,
  state       text NOT NULL DEFAULT 'provisioning',
  ssh_port    int  NOT NULL UNIQUE,
  ipv4        text,
  ssh_keys    text[] NOT NULL DEFAULT '{}',
  created_by  text NOT NULL,
  error       text,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS workspaces_tenant ON workspaces(tenant_id);
CREATE TABLE IF NOT EXISTS sessions (
  id          text PRIMARY KEY,
  data        bytea NOT NULL,
  expires_at  timestamptz NOT NULL
);
CREATE INDEX IF NOT EXISTS sessions_expiry ON sessions(expires_at);
CREATE TABLE IF NOT EXISTS schema_migrations (version text PRIMARY KEY, applied_at timestamptz NOT NULL DEFAULT now());
