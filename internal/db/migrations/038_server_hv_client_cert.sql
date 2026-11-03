-- Client certificate for a hypervisor API (Incus trusts remotes by certificate). These are PEM
-- paths on the console host, like hv_ca_file. The key itself is not a column.
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_cert_file text;
ALTER TABLE servers ADD COLUMN IF NOT EXISTS hv_key_file text;
