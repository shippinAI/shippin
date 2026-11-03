CREATE TABLE IF NOT EXISTS identity_users (
  id uuid PRIMARY KEY,
  email text NOT NULL UNIQUE,
  email_verified boolean NOT NULL DEFAULT false,
  name text NOT NULL DEFAULT '',
  role text NOT NULL DEFAULT 'member',
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS identity_orgs (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS identity_members (
  org_id uuid NOT NULL REFERENCES identity_orgs(id),
  user_id uuid NOT NULL REFERENCES identity_users(id),
  role text NOT NULL,
  PRIMARY KEY (org_id, user_id)
);
CREATE TABLE IF NOT EXISTS identity_passkeys (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES identity_users(id),
  credential_id bytea NOT NULL UNIQUE,
  public_key bytea NOT NULL,
  sign_count bigint NOT NULL DEFAULT 0,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS identity_recovery (
  id uuid PRIMARY KEY,
  user_id uuid NOT NULL REFERENCES identity_users(id),
  code_hash bytea NOT NULL,
  used_at timestamptz
);
CREATE TABLE IF NOT EXISTS identity_tokens (
  id uuid PRIMARY KEY,
  kind text NOT NULL,
  hash bytea NOT NULL UNIQUE,
  email text NOT NULL DEFAULT '',
  user_id uuid,
  meta jsonb NOT NULL DEFAULT '{}',
  expires_at timestamptz NOT NULL,
  used_at timestamptz
);
CREATE TABLE IF NOT EXISTS identity_links (
  issuer text NOT NULL,
  sub text NOT NULL,
  user_id uuid NOT NULL REFERENCES identity_users(id),
  PRIMARY KEY (issuer, sub)
);
CREATE UNIQUE INDEX IF NOT EXISTS identity_links_user_issuer ON identity_links (user_id, issuer);
CREATE TABLE IF NOT EXISTS identity_invites (
  id uuid PRIMARY KEY,
  hash bytea NOT NULL UNIQUE,
  email text NOT NULL DEFAULT '',
  role text NOT NULL DEFAULT 'member',
  org_id uuid,
  max_uses int NOT NULL DEFAULT 1,
  uses int NOT NULL DEFAULT 0,
  expires_at timestamptz NOT NULL,
  created_by uuid
);
CREATE TABLE IF NOT EXISTS identity_waitlist (
  id uuid PRIMARY KEY,
  email text NOT NULL UNIQUE,
  name text NOT NULL DEFAULT '',
  company text NOT NULL DEFAULT '',
  notes text NOT NULL DEFAULT '',
  status text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS identity_api_keys (
  id uuid PRIMARY KEY,
  name text NOT NULL,
  hash bytea NOT NULL UNIQUE,
  prefix text NOT NULL,
  scopes text[] NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS identity_webhooks (
  id uuid PRIMARY KEY,
  url text NOT NULL,
  secret bytea NOT NULL,
  events text[] NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS identity_oauth_codes (
  hash bytea PRIMARY KEY,
  client_id text NOT NULL,
  user_id uuid NOT NULL,
  challenge text NOT NULL,
  redirect text NOT NULL,
  expires_at timestamptz NOT NULL,
  used_at timestamptz
);
CREATE TABLE IF NOT EXISTS identity_oauth_refresh (
  hash bytea PRIMARY KEY,
  family uuid NOT NULL,
  client_id text NOT NULL,
  user_id uuid NOT NULL,
  replaced bool NOT NULL DEFAULT false,
  revoked bool NOT NULL DEFAULT false,
  expires_at timestamptz NOT NULL
);
CREATE TABLE IF NOT EXISTS identity_audit (
  id bigserial PRIMARY KEY,
  at timestamptz NOT NULL DEFAULT now(),
  actor text NOT NULL,
  action text NOT NULL,
  detail jsonb NOT NULL DEFAULT '{}'
);
CREATE TABLE IF NOT EXISTS identity_settings (
  key text PRIMARY KEY,
  value jsonb NOT NULL
);
