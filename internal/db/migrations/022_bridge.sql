-- N4 devspace access through the Shippin MCP bridge. A device sign-in (RFC 8628 style) binds a
-- bridge session to one account (IdP sub + email). Only hashes of the device code and the bearer
-- token are stored. revoked_at takes effect on the next request and closes open relays.
CREATE TABLE IF NOT EXISTS bridge_sessions (
  id               uuid PRIMARY KEY,
  device_hash      text NOT NULL UNIQUE,
  user_code        text NOT NULL UNIQUE,
  client_name      text NOT NULL CHECK (length(client_name) BETWEEN 1 AND 80),
  status           text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','approved','denied','issued')),
  sub              text,
  email            text,
  identity         jsonb,
  token_hash       text UNIQUE,
  created_at       timestamptz NOT NULL DEFAULT now(),
  device_expires   timestamptz NOT NULL,
  approved_at      timestamptz,
  expires_at       timestamptz,
  revoked_at       timestamptz,
  last_used_at     timestamptz
);
CREATE INDEX IF NOT EXISTS bridge_sessions_sub ON bridge_sessions(sub, created_at DESC);
