ALTER TABLE identity_users ADD COLUMN IF NOT EXISTS username text;
ALTER TABLE identity_users ADD COLUMN IF NOT EXISTS password_hash text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS identity_users_username_lower ON identity_users (lower(username)) WHERE username IS NOT NULL;
