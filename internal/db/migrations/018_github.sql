-- M3 import from GitHub through the Shippin GitHub App. No user token is ever stored: the OAuth
-- token from the install flow only proves the installation is the user's, then it is dropped.
-- Installation tokens (1 h) live in memory and in the workspace's tmpfs only.
CREATE TABLE IF NOT EXISTS github_installations (
  id              bigint PRIMARY KEY CHECK (id > 0), -- GitHub installation id
  tenant_id       uuid NOT NULL REFERENCES tenants(id), -- one installation maps to one tenant
  account_login   text NOT NULL DEFAULT '' CHECK (account_login ~ '^[A-Za-z0-9-]{0,39}$'),
  account_type    text NOT NULL DEFAULT '' CHECK (account_type IN ('','User','Organization','Enterprise')),
  repo_selection  text NOT NULL DEFAULT '' CHECK (repo_selection IN ('','all','selected')),
  suspended_at    timestamptz,
  deleted_at      timestamptz,
  created_by      uuid REFERENCES accounts(id),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS github_installations_tenant ON github_installations(tenant_id);

CREATE TABLE IF NOT EXISTS project_sources (
  project_id      uuid PRIMARY KEY REFERENCES projects(id) ON DELETE CASCADE,
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  kind            text NOT NULL CHECK (kind IN ('template','prompt','github','public_url')),
  installation_id bigint REFERENCES github_installations(id),
  repo_id         bigint,
  repo_full_name  text CHECK (repo_full_name ~ '^[A-Za-z0-9-]{1,39}/[A-Za-z0-9._-]{1,100}$'),
  default_branch  text NOT NULL DEFAULT '',
  branch          text NOT NULL DEFAULT '' CHECK (branch ~ '^[A-Za-z0-9._/-]{0,200}$' AND branch NOT LIKE '%..%'),
  status          text NOT NULL DEFAULT 'ok' CHECK (status IN ('ok','revoked','missing')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  CONSTRAINT github_source_complete CHECK (kind <> 'github' OR
    (installation_id IS NOT NULL AND repo_id IS NOT NULL AND repo_full_name IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS project_sources_installation ON project_sources(installation_id);

-- Webhook replay dedupe by X-GitHub-Delivery (pruned after 7 days by the reconciler).
CREATE TABLE IF NOT EXISTS github_deliveries (
  id          text PRIMARY KEY CHECK (id ~ '^[A-Za-z0-9-]{1,64}$'),
  received_at timestamptz NOT NULL DEFAULT now()
);
