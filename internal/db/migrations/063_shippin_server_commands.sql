-- Durable, target-bound host commands. Command payloads are bounded protocol
-- data; they never contain credentials or arbitrary executable/URL fields.
CREATE TABLE shippin_server_commands (
  id               uuid PRIMARY KEY,
  server_id        uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  tenant_id        uuid NOT NULL,
  deployment_id    uuid NOT NULL,
  generation       bigint NOT NULL CHECK (generation > 0),
  plan_sha256      text NOT NULL CHECK (plan_sha256 ~ '^[a-f0-9]{64}$'),
  idempotency_key  text NOT NULL CHECK (idempotency_key ~ '^[a-f0-9]{64}$'),
  command_type     text NOT NULL CHECK (command_type = 'probe_incus_application'),
  payload          jsonb NOT NULL CHECK (jsonb_typeof(payload) = 'object' AND octet_length(payload::text) <= 8192),
  state            text NOT NULL DEFAULT 'queued' CHECK (state IN ('queued','completed','expired')),
  result           jsonb CHECK (result IS NULL OR (jsonb_typeof(result) = 'object' AND octet_length(result::text) <= 2048)),
  created_at       timestamptz NOT NULL DEFAULT now(),
  expires_at       timestamptz NOT NULL,
  completed_at     timestamptz,
  UNIQUE (tenant_id, idempotency_key),
  FOREIGN KEY (deployment_id, tenant_id, generation)
    REFERENCES shippin_deployment_generations(deployment_id, tenant_id, generation) ON DELETE RESTRICT,
  CHECK ((state = 'completed') = (completed_at IS NOT NULL)),
  CHECK ((state = 'completed') = (result IS NOT NULL))
);
CREATE INDEX shippin_server_commands_pending
  ON shippin_server_commands(server_id, created_at, id)
  WHERE state = 'queued';
CREATE INDEX shippin_server_commands_expiry
  ON shippin_server_commands(expires_at)
  WHERE state = 'queued';
