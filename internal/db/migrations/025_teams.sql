-- Teams inside a tenant (Nomen has orgs and org roles but no teams). A team groups org members;
-- membership is by IdP user id and must be an org member (checked by the API against Nomen).
CREATE TABLE IF NOT EXISTS tenant_teams (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants(id),
  name        text NOT NULL CHECK (name ~ '^[a-z][a-z0-9-]{0,38}[a-z0-9]$'),
  description text NOT NULL DEFAULT '' CHECK (length(description) <= 300),
  created_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, name)
);
CREATE TABLE IF NOT EXISTS team_members (
  team_id   uuid NOT NULL REFERENCES tenant_teams(id) ON DELETE CASCADE,
  user_id   text NOT NULL CHECK (length(user_id) BETWEEN 1 AND 200),
  role      text NOT NULL DEFAULT 'member' CHECK (role IN ('maintainer','member')),
  added_by  text NOT NULL,
  added_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (team_id, user_id)
);
CREATE INDEX IF NOT EXISTS team_members_user ON team_members(user_id);

-- Ops-editable overrides of selected config keys (provider admin, Platform settings page). A
-- missing row means the config file value applies; every write is audited in the same tx.
CREATE TABLE IF NOT EXISTS platform_settings (
  key        text PRIMARY KEY CHECK (key ~ '^[a-z][a-z0-9_.]{2,80}$'),
  value      jsonb NOT NULL,
  updated_by text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);
-- The Owner preset row gains the platform settings permissions (a fresh database gets them from
-- the preset seed). No other role row changes; others get them only by a direct grant.
UPDATE staff_roles SET permissions = (
    SELECT array_agg(DISTINCT p ORDER BY p) FROM unnest(permissions || ARRAY['platform.settings.read','platform.settings.write']) p),
  updated_at = now()
WHERE key = 'staff.owner';
