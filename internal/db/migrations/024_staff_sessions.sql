-- Staff mode: a server-side session that a staff member opens (fresh IdP sign-in + reason) before
-- acting in the provider panel. One open session per staff member; ended_at closes it early.
CREATE TABLE IF NOT EXISTS staff_sessions (
  id          uuid PRIMARY KEY,
  actor       text NOT NULL CHECK (length(actor) BETWEEN 1 AND 200),
  actor_name  text NOT NULL DEFAULT '',
  reason      text NOT NULL CHECK (length(reason) BETWEEN 8 AND 1200),
  from_tenant uuid REFERENCES tenants(id),
  auth_at     timestamptz NOT NULL,
  started_at  timestamptz NOT NULL DEFAULT now(),
  expires_at  timestamptz NOT NULL,
  ended_at    timestamptz,
  CHECK (expires_at > started_at)
);
CREATE UNIQUE INDEX IF NOT EXISTS staff_sessions_one_open ON staff_sessions(actor) WHERE ended_at IS NULL;
