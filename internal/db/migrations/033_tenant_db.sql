-- Tenant database console: SQL audit, saved snippets, and one-click backup requests.
-- Backup rows record intent only. Dumps are a hostd job that is not wired yet.
-- Rollback: internal/db/rollback/033_tenant_db.down.sql
-- Same schema as cursor/tenant-database-ui-aba9's 028_tenant_db.sql. Renumbered to 033
-- because 028_hub.sql and 029_hub_live.sql already occupied 028/029 on wi-integration.
-- Do not re-add 028_tenant_db.sql.
CREATE TABLE IF NOT EXISTS tenant_sql_audit (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  actor       text NOT NULL,
  actor_name  text NOT NULL DEFAULT '',
  action      text NOT NULL,
  statement   text NOT NULL,
  kind        text NOT NULL DEFAULT '',
  read_only   boolean NOT NULL DEFAULT false,
  confirmed   boolean NOT NULL DEFAULT false,
  row_count   int,
  error       text NOT NULL DEFAULT '',
  status      text NOT NULL DEFAULT 'started',
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS tenant_sql_audit_tenant ON tenant_sql_audit (tenant_id, created_at DESC);

CREATE TABLE IF NOT EXISTS tenant_sql_snippets (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  owner_sub   text NOT NULL,
  name        text NOT NULL,
  sql         text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, owner_sub, name)
);

CREATE TABLE IF NOT EXISTS tenant_db_backups (
  id          uuid PRIMARY KEY,
  tenant_id   uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  actor       text NOT NULL,
  note        text NOT NULL DEFAULT '',
  status      text NOT NULL DEFAULT 'pending_backend',
  created_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS tenant_db_backups_tenant ON tenant_db_backups (tenant_id, created_at DESC);
