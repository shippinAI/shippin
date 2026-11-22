-- Every setting in the panel (CFG-01, docs/settings.md).
--
-- shippin_secret_store is the installation's Vaultix account, connected with a machine identity. Its
-- client secret is the one key that can't be kept in Vaultix, so it is sealed here with the panel's
-- at-rest key; it is never returned.
CREATE TABLE IF NOT EXISTS shippin_secret_store (
  id            boolean PRIMARY KEY DEFAULT true CHECK (id),
  api_url       text NOT NULL CHECK (api_url ~ '^https?://'),
  project_id    text NOT NULL CHECK (length(project_id) BETWEEN 1 AND 100),
  environment   text NOT NULL CHECK (environment ~ '^[a-z0-9][a-z0-9_-]{0,63}$'),
  path          text NOT NULL CHECK (path ~ '^(/[A-Za-z0-9_-]{1,64}){1,8}$'),
  client_id     text NOT NULL CHECK (length(client_id) BETWEEN 1 AND 200),
  client_secret bytea NOT NULL,
  updated_by    text NOT NULL,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  checked_at    timestamptz,
  check_error   text NOT NULL DEFAULT ''
);

-- One row per section saved in the panel. A section with no row still comes from the bootstrap file.
-- vals holds no key: keys maps a key field to where it is in the secret store (ports.SecretRef).
CREATE TABLE IF NOT EXISTS shippin_settings (
  section    text PRIMARY KEY CHECK (section ~ '^[a-z][a-z0-9_]{1,40}$'),
  vals       jsonb NOT NULL CHECK (jsonb_typeof(vals) = 'object'),
  keys       jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(keys) = 'object'),
  version    integer NOT NULL CHECK (version > 0),
  updated_by text NOT NULL,
  updated_at timestamptz NOT NULL DEFAULT now()
);

-- Each saved version of a section, with who, why and which keys changed (their names, never values).
CREATE TABLE IF NOT EXISTS shippin_setting_versions (
  section      text NOT NULL,
  version      integer NOT NULL CHECK (version > 0),
  vals         jsonb NOT NULL,
  keys_changed text[] NOT NULL DEFAULT '{}',
  changed_by   text NOT NULL,
  reason       text NOT NULL,
  changed_at   timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (section, version)
);

-- Keys to delete from the secret store: ones a save replaced or removed (due at once), and ones written
-- for a save that has not committed yet (due later; the save deletes its row when it commits). A row
-- names where the key is, never its value.
CREATE TABLE IF NOT EXISTS shippin_setting_retired_secrets (
  id         bigserial PRIMARY KEY,
  ref        jsonb NOT NULL,
  due_at     timestamptz NOT NULL DEFAULT now(),
  attempts   integer NOT NULL DEFAULT 0,
  last_error text NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS shippin_setting_retired_secrets_due ON shippin_setting_retired_secrets (due_at);
