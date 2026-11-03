-- N2 agent identities (02a-product-rbac, decision 14): each agent is its own IdP service user in
-- the tenant's org, delegated by one user. No credential is stored here; tokens come later through
-- Nomen token exchange (`act`). tool_grants can only ever narrow what the delegating user holds.
CREATE TABLE IF NOT EXISTS agent_identities (
  id                 uuid PRIMARY KEY,
  tenant_id          uuid NOT NULL REFERENCES tenants(id),
  workspace_id       uuid REFERENCES workspaces(id),
  name               text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
  idp_user_id        text NOT NULL UNIQUE,
  delegating_user_id text NOT NULL,
  tool_grants        text[] NOT NULL DEFAULT '{}',
  created_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);
CREATE INDEX IF NOT EXISTS agent_identities_tenant ON agent_identities(tenant_id, created_at DESC);
