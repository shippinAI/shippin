ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS host text UNIQUE;
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS domains text[] NOT NULL DEFAULT '{}';
ALTER TABLE workspaces ADD COLUMN IF NOT EXISTS firewall jsonb NOT NULL DEFAULT '[]';
CREATE TABLE IF NOT EXISTS user_ssh_keys (
  id          uuid PRIMARY KEY,
  user_sub    text NOT NULL,
  name        text NOT NULL,
  public_key  text NOT NULL,
  fingerprint text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  UNIQUE (user_sub, fingerprint)
);
CREATE INDEX IF NOT EXISTS user_ssh_keys_user ON user_ssh_keys(user_sub);
