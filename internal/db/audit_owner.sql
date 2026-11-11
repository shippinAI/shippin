-- Hands the audit log and access grants to a separate NOLOGIN owner so the app role can only
-- read and append: no UPDATE/DELETE/TRUNCATE on audit_log, only status columns of grants, and
-- no ALTER/DROP/DISABLE TRIGGER (those need ownership). Idempotent. Must run as a superuser,
-- AFTER the app has applied migration 005, with:  SET shippin.app_role = '<app role>';
-- Not an app migration on purpose: the app role can't (and mustn't) do any of this.
DO $$
DECLARE app text := current_setting('shippin.app_role');
BEGIN
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = app) THEN
    RAISE EXCEPTION 'app role % does not exist', app;
  END IF;
  IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'shippin_audit_owner') THEN
    CREATE ROLE shippin_audit_owner NOLOGIN NOINHERIT;
  END IF;
  ALTER ROLE shippin_audit_owner NOLOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION NOBYPASSRLS;
  IF pg_has_role(app, 'shippin_audit_owner', 'MEMBER') THEN
    EXECUTE format('REVOKE shippin_audit_owner FROM %I', app);
  END IF;
  ALTER TABLE audit_log OWNER TO shippin_audit_owner;      -- its sequence follows
  ALTER TABLE access_grants OWNER TO shippin_audit_owner;
  ALTER FUNCTION shippin_append_only() OWNER TO shippin_audit_owner;
  EXECUTE format('REVOKE ALL ON audit_log, access_grants FROM PUBLIC, %I', app);
  EXECUTE format('REVOKE ALL ON SEQUENCE audit_log_id_seq FROM PUBLIC, %I', app);
  EXECUTE format('GRANT SELECT, INSERT ON audit_log, access_grants TO %I', app);
  EXECUTE format('GRANT USAGE ON SEQUENCE audit_log_id_seq TO %I', app);
  EXECUTE format('GRANT UPDATE (status, decided_by, decided_name, decided_at, expires_at, ended_at) ON access_grants TO %I', app);
  -- Whoever owns a schema can drop anything in it, whoever owns the table, and a database owner owns
  -- public through pg_database_owner. The superuser keeps both; the app keeps what migrations need.
  EXECUTE format('ALTER DATABASE %I OWNER TO CURRENT_USER', current_database());
  ALTER SCHEMA public OWNER TO CURRENT_USER;
  EXECUTE format('GRANT CONNECT, CREATE, TEMPORARY ON DATABASE %I TO %I', current_database(), app);
  EXECUTE format('GRANT USAGE, CREATE ON SCHEMA public TO %I', app);
END $$;
