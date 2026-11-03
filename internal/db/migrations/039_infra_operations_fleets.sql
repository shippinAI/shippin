-- Reach a hypervisor from the panel alone. hv_trusted_cert and hv_client_cert are public. hv_secret
-- and hv_client_key are sealed with a key derived from SHIPPIN_SESSION_KEY and the server id; only
-- ciphertext is stored, and no API returns them.
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_trusted_cert text;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_client_cert text;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_client_key bytea;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_secret bytea;

-- A fleet groups hosts in the panel. Nothing changes on the hosts, and each keeps its own provider
-- and native cluster. A host is in at most one fleet.
CREATE TABLE IF NOT EXISTS infra_fleets (
  id          uuid PRIMARY KEY,
  label       text NOT NULL CHECK (length(label) BETWEEN 1 AND 120),
  description text NOT NULL DEFAULT '' CHECK (length(description) <= 400),
  created_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now()
);
ALTER TABLE servers ADD COLUMN IF NOT EXISTS fleet_id uuid REFERENCES infra_fleets(id) ON DELETE SET NULL;

-- Every change to a guest: who asked, what they asked for, and how it ended. It outlives the request
-- and the process. queued and running only ever end as a terminal state, and a panel restart turns
-- what it left behind into unknown, because the hypervisor may still have done the work.
CREATE TABLE IF NOT EXISTS infra_operations (
  id           uuid PRIMARY KEY,
  server_id    uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  batch_id     uuid,
  guest        text NOT NULL,
  verb         text NOT NULL,
  status       text NOT NULL CHECK (status IN ('queued','running','succeeded','failed','unknown','skipped')),
  detail       text NOT NULL DEFAULT '',
  guest_status text NOT NULL DEFAULT '',
  actor        text NOT NULL,
  actor_name   text NOT NULL DEFAULT '',
  started_at   timestamptz NOT NULL DEFAULT now(),
  finished_at  timestamptz
);
CREATE INDEX IF NOT EXISTS infra_operations_server ON infra_operations (server_id, started_at DESC);
CREATE INDEX IF NOT EXISTS infra_operations_recent ON infra_operations (started_at DESC);
CREATE INDEX IF NOT EXISTS infra_operations_open ON infra_operations (status) WHERE status IN ('queued','running');
