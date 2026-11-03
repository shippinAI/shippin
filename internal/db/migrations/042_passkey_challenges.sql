-- Each passkey ceremony signs a challenge this panel issued, and each challenge works once. Without this a
-- captured response could be replayed.
CREATE TABLE IF NOT EXISTS identity_passkey_challenges (
  hash       bytea PRIMARY KEY,
  expires_at timestamptz NOT NULL
);
