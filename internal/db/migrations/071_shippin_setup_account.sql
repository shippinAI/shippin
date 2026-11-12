-- The built-in setup account (REL-03): the one account an installer, a person or an AI assistant, uses
-- to finish setting up. /setup creates it with the one-time setup code. A person then takes it over by
-- setting its password, which ends every other session of it, and creates their own owner account.
ALTER TABLE identity_users ADD COLUMN IF NOT EXISTS builtin text CHECK (builtin IN ('setup'));
CREATE UNIQUE INDEX IF NOT EXISTS identity_users_builtin ON identity_users (builtin) WHERE builtin IS NOT NULL;
-- When a person took the setup account over; null while only whoever ran setup knows its password.
ALTER TABLE identity_users ADD COLUMN IF NOT EXISTS taken_over_at timestamptz;

-- Sessions record their local user, so every session of one account can be ended at once.
ALTER TABLE sessions ADD COLUMN IF NOT EXISTS user_id text;
CREATE INDEX IF NOT EXISTS sessions_user ON sessions (user_id) WHERE user_id IS NOT NULL;
