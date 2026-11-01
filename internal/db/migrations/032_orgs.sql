-- CP-P0b org split (CONTROL-PLANE-DEVSPACE-PLAN.md §3a). One Operations org (the Shippin.cloud control
-- plane: servers, tenants, staff) and customer orgs (one per Nomen org; the operator's Builder org is an
-- ordinary customer). Staff roles live only on the Operations org and never imply tenant data access;
-- support access to a customer org is an explicit, time-limited, audited grant.
-- Rollback (not embedded, run by hand): internal/db/rollback/029_orgs.down.sql
CREATE TABLE IF NOT EXISTS orgs (
  id           text PRIMARY KEY CHECK (id ~ '^org_[0-9A-HJKMNP-TV-Z]{26}$'),
  slug         text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9][a-z0-9-]{0,39}$'),
  type         text NOT NULL CHECK (type IN ('operations','customer')),
  name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 120),
  nomen_org_id text UNIQUE,
  created_at   timestamptz NOT NULL DEFAULT now()
);
-- Exactly one Operations org can exist.
CREATE UNIQUE INDEX IF NOT EXISTS orgs_single_operations ON orgs ((true)) WHERE type = 'operations';

CREATE TABLE IF NOT EXISTS folders (
  id         text PRIMARY KEY CHECK (id ~ '^fld_[0-9A-HJKMNP-TV-Z]{26}$'),
  org_id     text NOT NULL REFERENCES orgs(id) ON DELETE CASCADE,
  parent_id  text REFERENCES folders(id) ON DELETE CASCADE,
  name       text NOT NULL CHECK (length(name) BETWEEN 1 AND 80),
  created_at timestamptz NOT NULL DEFAULT now(),
  CHECK (parent_id IS NULL OR parent_id <> id)
);
CREATE UNIQUE INDEX IF NOT EXISTS folders_org_parent_name ON folders (org_id, coalesce(parent_id, ''), name);

-- staff_roles (007) is the existing staff role catalog (staff.* keys, Zitadel-held); P0b ops roles are
-- assigned here so the two never collide. Reconciling both is an open decision (plan §7).
CREATE TABLE IF NOT EXISTS staff_assignments (
  user_sub   text NOT NULL CHECK (length(user_sub) BETWEEN 1 AND 200),
  role       text NOT NULL CHECK (role IN ('ops_admin','ops_oncall','ops_support','ops_auditor')),
  granted_by text NOT NULL,
  granted_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz,
  revoked_at timestamptz,
  PRIMARY KEY (user_sub, role)
);

CREATE TABLE IF NOT EXISTS support_grants (
  id         uuid PRIMARY KEY,
  staff_sub  text NOT NULL,
  org_id     text NOT NULL REFERENCES orgs(id),
  project_id text,
  reason     text NOT NULL CHECK (length(reason) BETWEEN 10 AND 500),
  granted_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now(),
  expires_at timestamptz NOT NULL,
  revoked_at timestamptz,
  CHECK (expires_at > created_at)
);
CREATE INDEX IF NOT EXISTS support_grants_active ON support_grants (staff_sub, org_id) WHERE revoked_at IS NULL;
