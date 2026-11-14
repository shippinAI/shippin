-- Access keys (MCPL-01): a person lets an AI client or a script act as them, through MCP or the API,
-- with the same permissions, audit and events as their own session. Only a hash of the key is kept;
-- the key itself is shown once. Each key expires, records when it was last used, and is revoked by
-- deleting it. A key carries the sign-in method of the session that made it, so an organization's
-- sign-in rule applies to it as it did to that session.
CREATE TABLE IF NOT EXISTS access_keys (
  id           uuid PRIMARY KEY,
  user_id      uuid NOT NULL REFERENCES identity_users(id) ON DELETE CASCADE,
  name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 64),
  prefix       text NOT NULL,
  hash         bytea NOT NULL UNIQUE,
  method       text NOT NULL,
  created_at   timestamptz NOT NULL DEFAULT now(),
  expires_at   timestamptz NOT NULL,
  last_used_at timestamptz
);
CREATE INDEX IF NOT EXISTS access_keys_user ON access_keys (user_id);
