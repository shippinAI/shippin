-- N2 audit hash chain. Every audit_log row carries chain_seq (gapless), prev_hash and
-- hash = sha256(prev_hash || canonical row). A BEFORE INSERT trigger fills them under an advisory
-- lock, so the app role can't skip or forge links, and shippin_audit_verify() re-walks the chain.
-- Superuser path only (like audit_owner.sql, which must run first): the app role doesn't own
-- audit_log. Idempotent; rows written before the chain existed are linked once, in id order.
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS chain_seq bigint;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS prev_hash bytea;
ALTER TABLE audit_log ADD COLUMN IF NOT EXISTS hash bytea;
CREATE UNIQUE INDEX IF NOT EXISTS audit_log_chain_seq ON audit_log(chain_seq);

-- Canonical form: a JSON array of the row's facts (jsonb text output is deterministic); the
-- time is in microseconds since the epoch so the session time zone can't change it.
CREATE OR REPLACE FUNCTION shippin_audit_row_hash(prev bytea, r audit_log) RETURNS bytea
LANGUAGE sql IMMUTABLE SET search_path = pg_catalog AS $$
  SELECT sha256(coalesce(prev, ''::bytea) || convert_to(jsonb_build_array(
    r.chain_seq, (extract(epoch FROM r.at) * 1000000)::bigint, r.tenant_id, r.workspace_id, r.grant_id,
    r.actor, r.actor_name, r.action, r.detail, r.ip, r.breakglass)::text, 'UTF8'))
$$;

CREATE OR REPLACE FUNCTION shippin_audit_chain() RETURNS trigger
LANGUAGE plpgsql SET search_path = public, pg_temp AS $$
DECLARE last_seq bigint; last_hash bytea;
BEGIN
  PERFORM pg_advisory_xact_lock(hashtext('shippin.audit_chain'));
  SELECT chain_seq, hash INTO last_seq, last_hash FROM audit_log
    WHERE chain_seq IS NOT NULL ORDER BY chain_seq DESC LIMIT 1;
  NEW.chain_seq := coalesce(last_seq, 0) + 1;
  NEW.prev_hash := last_hash;
  NEW.hash := shippin_audit_row_hash(last_hash, NEW);
  RETURN NEW;
END $$;

-- Walks the whole chain: rows, ok, the first broken chain_seq (or NULL) and the head hash (hex),
-- which can be anchored outside the database.
CREATE OR REPLACE FUNCTION shippin_audit_verify(OUT n_rows bigint, OUT ok boolean, OUT first_bad bigint, OUT head text)
LANGUAGE plpgsql STABLE SET search_path = public, pg_temp AS $$
DECLARE r audit_log; prev bytea;
BEGIN
  n_rows := 0; ok := true;
  FOR r IN SELECT * FROM audit_log ORDER BY chain_seq NULLS LAST, id LOOP
    n_rows := n_rows + 1;
    IF ok AND (r.chain_seq IS DISTINCT FROM n_rows OR r.prev_hash IS DISTINCT FROM prev
               OR r.hash IS DISTINCT FROM shippin_audit_row_hash(prev, r)) THEN
      ok := false; first_bad := coalesce(r.chain_seq, -r.id);
    END IF;
    prev := r.hash;
  END LOOP;
  head := encode(prev, 'hex');
END $$;

DO $$
DECLARE r record; prev bytea; seq bigint;
BEGIN
  LOCK TABLE audit_log IN SHARE ROW EXCLUSIVE MODE;
  IF EXISTS (SELECT 1 FROM audit_log WHERE hash IS NULL) THEN
    SELECT chain_seq, hash INTO seq, prev FROM audit_log WHERE hash IS NOT NULL ORDER BY chain_seq DESC LIMIT 1;
    seq := coalesce(seq, 0);
    ALTER TABLE audit_log DISABLE TRIGGER audit_log_append_only;
    FOR r IN SELECT id FROM audit_log WHERE hash IS NULL ORDER BY id LOOP
      seq := seq + 1;
      UPDATE audit_log SET chain_seq = seq, prev_hash = prev WHERE id = r.id;
      UPDATE audit_log a SET hash = shippin_audit_row_hash(prev, a) WHERE id = r.id RETURNING hash INTO prev;
    END LOOP;
    ALTER TABLE audit_log ENABLE TRIGGER audit_log_append_only;
  END IF;
  DROP TRIGGER IF EXISTS audit_log_chain ON audit_log;
  CREATE TRIGGER audit_log_chain BEFORE INSERT ON audit_log FOR EACH ROW EXECUTE FUNCTION shippin_audit_chain();
  ALTER FUNCTION shippin_audit_row_hash(bytea, audit_log) OWNER TO shippin_audit_owner;
  ALTER FUNCTION shippin_audit_chain() OWNER TO shippin_audit_owner;
  ALTER FUNCTION shippin_audit_verify() OWNER TO shippin_audit_owner;
  REVOKE ALL ON FUNCTION shippin_audit_verify() FROM PUBLIC;
  EXECUTE format('GRANT EXECUTE ON FUNCTION shippin_audit_verify() TO %I', current_setting('shippin.app_role'));
END $$;
