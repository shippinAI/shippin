-- Vaultix hosted (multi-tenant): one Vaultix organization + workspace (project) per signed-in
-- Shippin person (Nomen subject). The console identity's client secret is sealed (AES-GCM, key
-- from the secrets store); the keyring identity's secrets are shown once and never stored.
CREATE TABLE IF NOT EXISTS vaultix_accounts (
  sub           text PRIMARY KEY,
  email         text NOT NULL DEFAULT '',
  nomen_org     text NOT NULL DEFAULT '',
  vx_org        text NOT NULL,
  project_id    text NOT NULL,
  console_id    text NOT NULL,
  console_cid   text NOT NULL,
  console_seal  bytea NOT NULL,
  keyring_id    text NOT NULL DEFAULT '',
  keyring_cid   text NOT NULL DEFAULT '',
  created_at    timestamptz NOT NULL DEFAULT now()
);
