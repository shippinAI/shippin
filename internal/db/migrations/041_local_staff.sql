-- Staff roles kept by the panel itself, for an installation whose identity provider does not hold them.
CREATE TABLE IF NOT EXISTS local_staff_roles (
  user_id    uuid PRIMARY KEY REFERENCES identity_users(id),
  roles      text[] NOT NULL DEFAULT '{}',
  updated_at timestamptz NOT NULL DEFAULT now()
);
-- A person can be switched off without losing what they did.
ALTER TABLE identity_users ADD COLUMN IF NOT EXISTS active boolean NOT NULL DEFAULT true;
