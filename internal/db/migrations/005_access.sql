-- Customer-granted platform access (and break-glass), plus an append-only audit log.
CREATE TABLE IF NOT EXISTS access_grants (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL REFERENCES tenants(id),
  workspace_id   uuid REFERENCES workspaces(id),
  kind           text NOT NULL CHECK (kind IN ('request', 'breakglass')),
  requester      text NOT NULL,
  requester_name text NOT NULL,
  reason         text NOT NULL,
  category       text NOT NULL DEFAULT '',
  duration_s     int  NOT NULL CHECK (duration_s > 0),
  status         text NOT NULL CHECK (status IN ('pending', 'approved', 'denied', 'revoked', 'expired', 'ended', 'cancelled')),
  decided_by     text NOT NULL DEFAULT '',
  decided_name   text NOT NULL DEFAULT '',
  created_at     timestamptz NOT NULL DEFAULT now(),
  decided_at     timestamptz,
  expires_at     timestamptz,
  ended_at       timestamptz,
  -- When the customer may see this grant and its audit entries (later than created_at only for a legal-hold break-glass).
  notice_at      timestamptz,
  CHECK (status <> 'approved' OR expires_at IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS access_grants_tenant ON access_grants(tenant_id, status);
CREATE INDEX IF NOT EXISTS access_grants_requester ON access_grants(requester, status);

CREATE TABLE IF NOT EXISTS audit_log (
  id           bigserial PRIMARY KEY,
  at           timestamptz NOT NULL DEFAULT now(),
  tenant_id    uuid,
  workspace_id uuid,
  grant_id     uuid,
  actor        text NOT NULL,
  actor_name   text NOT NULL DEFAULT '',
  action       text NOT NULL,
  detail       text NOT NULL DEFAULT '',
  ip           text NOT NULL DEFAULT '',
  -- Customers see their tenant's entries; entries of a grant follow its notice_at.
  breakglass   boolean NOT NULL DEFAULT false
);
CREATE INDEX IF NOT EXISTS audit_log_tenant ON audit_log(tenant_id, at DESC);

CREATE OR REPLACE FUNCTION shippin_append_only() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END $$;
DROP TRIGGER IF EXISTS audit_log_append_only ON audit_log;
CREATE TRIGGER audit_log_append_only BEFORE UPDATE OR DELETE ON audit_log
  FOR EACH ROW EXECUTE FUNCTION shippin_append_only();
DROP TRIGGER IF EXISTS audit_log_no_truncate ON audit_log;
CREATE TRIGGER audit_log_no_truncate BEFORE TRUNCATE ON audit_log
  FOR EACH STATEMENT EXECUTE FUNCTION shippin_append_only();
-- Grants change status but are never removed.
DROP TRIGGER IF EXISTS access_grants_no_delete ON access_grants;
CREATE TRIGGER access_grants_no_delete BEFORE DELETE ON access_grants
  FOR EACH ROW EXECUTE FUNCTION shippin_append_only();
DROP TRIGGER IF EXISTS access_grants_no_truncate ON access_grants;
CREATE TRIGGER access_grants_no_truncate BEFORE TRUNCATE ON access_grants
  FOR EACH STATEMENT EXECUTE FUNCTION shippin_append_only();
