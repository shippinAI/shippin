-- CP-P1 server registry (CONTROL-PLANE-DEVSPACE-PLAN.md §4). org = tenant for now. tenant_id NULL
-- marks an estate row the platform seeds (remote sites); tenants see it read-only. No secret is
-- stored in clear: enrollment tokens and host credentials are kept as sha256 hex only.
CREATE TABLE IF NOT EXISTS servers (
  id              uuid PRIMARY KEY,
  tenant_id       uuid REFERENCES tenants(id),
  slug            text NOT NULL CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,39}$'),
  label           text NOT NULL CHECK (length(label) BETWEEN 1 AND 120),
  site            text NOT NULL CHECK (site ~ '^[a-z0-9-]{1,40}$'),
  state           text NOT NULL CHECK (state IN ('planned','pending','active','revoked')),
  hostname        text NOT NULL DEFAULT '',
  agent_version   text NOT NULL DEFAULT '',
  cred_sha256     text,
  cred_expires_at timestamptz,
  last_seen_at    timestamptz,
  enrolled_at     timestamptz,
  revoked_at      timestamptz,
  created_by      text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS servers_tenant_slug ON servers (coalesce(tenant_id::text, 'estate'), slug);
CREATE UNIQUE INDEX IF NOT EXISTS servers_cred ON servers (cred_sha256) WHERE cred_sha256 IS NOT NULL;

-- Single-use enrollment tokens: shown once, stored hashed, short TTL. Issuing a new one voids the old.
CREATE TABLE IF NOT EXISTS server_enroll_tokens (
  id          uuid PRIMARY KEY,
  server_id   uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  token_sha256 text NOT NULL UNIQUE,
  expires_at  timestamptz NOT NULL,
  used_at     timestamptz,
  voided_at   timestamptz,
  created_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS server_enroll_tokens_server ON server_enroll_tokens(server_id, created_at DESC);

-- Which tenants (orgs) may place work on a server. The creating tenant is assigned 'owner'.
CREATE TABLE IF NOT EXISTS server_assignments (
  server_id  uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  tenant_id  uuid NOT NULL REFERENCES tenants(id),
  role       text NOT NULL CHECK (role IN ('owner','placement')),
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (server_id, tenant_id)
);
