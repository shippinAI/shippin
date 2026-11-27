-- Which customer a guest on a server belongs to. A guest with no row is unassigned: one of the host's
-- own, or one that was running before the panel managed the host and that nobody has placed yet. The
-- guest is named as the hypervisor names it (an Incus instance name, a Proxmox VM id).
CREATE TABLE IF NOT EXISTS shippin_guest_owners (
  server_id   uuid NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
  guest       text NOT NULL,
  tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  assigned_by text NOT NULL,
  assigned_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (server_id, guest)
);
CREATE INDEX IF NOT EXISTS shippin_guest_owners_tenant ON shippin_guest_owners (tenant_id);
