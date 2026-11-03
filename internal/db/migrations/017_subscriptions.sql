-- M2 activate subscriptions. Each user signs in to their OWN AI subscription with the vendor's
-- CLI inside their own container; the login lives only on that user's AI home volume (ai-<account>).
-- Shippin stores status, never credentials: token_ref must stay NULL for the hosted runner.
CREATE TABLE IF NOT EXISTS ai_homes (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenants(id),
  account_id     uuid NOT NULL REFERENCES accounts(id),
  boundary_id    uuid REFERENCES boundaries(id), -- NULL: legacy tenant (no boundary VM)
  volume         text NOT NULL CHECK (volume ~ '^ai-[0-9a-f-]{36}$'),
  state          text NOT NULL DEFAULT 'creating' CHECK (state IN ('creating','ready','error')),
  error          text,
  utility_host   text UNIQUE,
  utility_state  text NOT NULL DEFAULT 'stopped' CHECK (utility_state IN ('stopped','starting','running','error')),
  utility_seen   timestamptz,
  created_at     timestamptz NOT NULL DEFAULT now(),
  updated_at     timestamptz NOT NULL DEFAULT now()
);
-- One AI home per user per placement (tenant + boundary; legacy = no boundary).
CREATE UNIQUE INDEX IF NOT EXISTS ai_homes_owner ON ai_homes(account_id, tenant_id,
  coalesce(boundary_id, '00000000-0000-0000-0000-000000000000'::uuid));

CREATE TABLE IF NOT EXISTS subscription_connections (
  id           uuid PRIMARY KEY,
  tenant_id    uuid NOT NULL REFERENCES tenants(id),
  account_id   uuid NOT NULL REFERENCES accounts(id),
  provider_id  text NOT NULL CHECK (provider_id ~ '^[a-z][a-z0-9-]{1,31}$'),
  runner       text NOT NULL DEFAULT 'hosted_workspace' CHECK (runner IN ('hosted_workspace')),
  host_id      uuid NOT NULL REFERENCES ai_homes(id),
  method       text NOT NULL DEFAULT 'client_login' CHECK (method IN ('client_login')),
  token_ref    text,
  status       text NOT NULL DEFAULT 'not_connected'
               CHECK (status IN ('not_connected','activating','connected','needs_reauth','limited','disconnected')),
  plan_label   text NOT NULL DEFAULT '' CHECK (length(plan_label) <= 64),
  checked_at   timestamptz,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now(),
  UNIQUE (account_id, provider_id, host_id),
  -- "No tokens in Shippin" as a database rule.
  CONSTRAINT subscription_no_token CHECK (runner <> 'hosted_workspace' OR token_ref IS NULL)
);
CREATE INDEX IF NOT EXISTS subscription_connections_account ON subscription_connections(account_id);
