-- Retired (deleted) workspaces keep their row for audit but must not hold their SSH port or name.
ALTER TABLE workspaces DROP CONSTRAINT IF EXISTS workspaces_ssh_port_key;
ALTER TABLE workspaces DROP CONSTRAINT IF EXISTS workspaces_tenant_id_name_key;
ALTER TABLE workspaces DROP CONSTRAINT IF EXISTS workspaces_host_key;
CREATE UNIQUE INDEX IF NOT EXISTS workspaces_live_ssh_port ON workspaces(ssh_port) WHERE state <> 'deleted';
CREATE UNIQUE INDEX IF NOT EXISTS workspaces_live_name ON workspaces(tenant_id, name) WHERE state <> 'deleted';
CREATE UNIQUE INDEX IF NOT EXISTS workspaces_live_host ON workspaces(host) WHERE state <> 'deleted';
