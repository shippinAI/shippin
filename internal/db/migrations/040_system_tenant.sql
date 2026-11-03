-- The control plane's own tenant. It owns what runs the platform: the hypervisors added from the panel,
-- the guest the panel runs on, and later the platform's own workspaces and databases. It is marked so
-- that the customer paths never return it. Belonging to it grants nothing: access comes from staff roles.
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS kind text NOT NULL DEFAULT 'customer' CHECK (kind IN ('customer', 'system'));
CREATE UNIQUE INDEX IF NOT EXISTS tenants_one_system ON tenants (kind) WHERE kind = 'system';
INSERT INTO tenants(id, idp_org_id, name, plan, status, kind)
  VALUES ('019a0000-0000-7000-8000-00000000c0de', 'system:control-plane', 'Control plane', 'system', 'active', 'system')
  ON CONFLICT DO NOTHING;

-- One installation's site names were seeded here on every install. A row a site attached a hypervisor to,
-- or assigned to a tenant, is that site's own and stays.
DELETE FROM servers WHERE created_by = 'seed:028' AND state = 'planned' AND hv_kind IS NULL
  AND NOT EXISTS (SELECT 1 FROM server_assignments a WHERE a.server_id = servers.id);

-- A server with no tenant was how "the platform's own" used to be spelled. Now it has an owner.
UPDATE servers SET tenant_id = '019a0000-0000-7000-8000-00000000c0de' WHERE tenant_id IS NULL;
ALTER TABLE servers ALTER COLUMN tenant_id SET NOT NULL;

-- Why each change to a guest was made.
ALTER TABLE infra_operations ADD COLUMN IF NOT EXISTS note text NOT NULL DEFAULT '';

-- Roles already on this database gain the infrastructure permissions. A fresh database gets them from
-- the preset seed, which also adds the new role.
UPDATE staff_roles SET permissions = (
    SELECT array_agg(DISTINCT p ORDER BY p) FROM unnest(permissions || ARRAY['infra.read','infra.hosts.write','infra.fleets.write','infra.guests.power','infra.guests.snapshot','infra.guests.restore','infra.guests.backup','infra.guests.manage','infra.storage.write','infra.self']) AS p
  ), updated_at = now() WHERE key = 'staff.owner';
UPDATE staff_roles SET permissions = (
    SELECT array_agg(DISTINCT p ORDER BY p) FROM unnest(permissions || ARRAY['infra.read','infra.guests.power','infra.guests.snapshot']) AS p
  ), updated_at = now() WHERE key = 'staff.ops';
UPDATE staff_roles SET permissions = (
    SELECT array_agg(DISTINCT p ORDER BY p) FROM unnest(permissions || ARRAY['infra.read']) AS p
  ), updated_at = now() WHERE key = 'staff.security';
UPDATE staff_roles SET permissions = (
    SELECT array_agg(DISTINCT p ORDER BY p) FROM unnest(permissions || ARRAY['infra.read']) AS p
  ), updated_at = now() WHERE key = 'staff.auditor';
INSERT INTO staff_roles(key, name, description, permissions, preset)
  SELECT 'staff.infra', 'Infrastructure', 'Adds hosts and changes guests. Does not touch the guest the panel runs on.',
    ARRAY['infra.read','infra.hosts.write','infra.fleets.write','infra.guests.power','infra.guests.snapshot','infra.guests.restore','infra.guests.backup','infra.guests.manage','infra.storage.write']::text[], true
  WHERE EXISTS (SELECT 1 FROM staff_settings)
  ON CONFLICT DO NOTHING;
