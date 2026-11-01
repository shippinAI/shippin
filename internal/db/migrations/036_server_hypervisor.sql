-- Hypervisor API on the existing server row. Enrollment, heartbeat, and hostd are unchanged.
-- Seeded estate rows stay null: no address is added for them. The token secret is not a column;
-- hv_token_env is the name of an environment variable. hv_ca_file is a PEM path, not the PEM.
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_kind text
  CHECK (hv_kind IS NULL OR hv_kind IN ('proxmox', 'incus', 'bare'));
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_api_url text;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_node text;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_token_id text;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_token_env text;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_ca_file text;
