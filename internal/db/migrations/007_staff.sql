-- Staff roles are data: an Owner creates, renames, edits and deletes them in the Team page.
-- Keys mirror Nomen project role keys (synced by the console). Presets are seeded once.
CREATE TABLE IF NOT EXISTS staff_roles (
  key          text PRIMARY KEY CHECK (key ~ '^staff\.[a-z][a-z0-9_]{1,30}$'),
  name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 60),
  description  text NOT NULL DEFAULT '' CHECK (length(description) <= 300),
  permissions  text[] NOT NULL DEFAULT '{}',
  preset       boolean NOT NULL DEFAULT false,
  created_at   timestamptz NOT NULL DEFAULT now(),
  updated_at   timestamptz NOT NULL DEFAULT now()
);
-- One row of team settings (two-person rule, idle days, break-glass approvals, mode).
CREATE TABLE IF NOT EXISTS staff_settings (
  id          int PRIMARY KEY CHECK (id = 1),
  data        jsonb NOT NULL,
  updated_at  timestamptz NOT NULL DEFAULT now(),
  updated_by  text NOT NULL DEFAULT ''
);
-- Last time each staff user used the console (shown as last activity, drives the idle flag).
CREATE TABLE IF NOT EXISTS staff_seen (
  user_id  text PRIMARY KEY,
  at       timestamptz NOT NULL
);
-- Two-person approvals for high-risk team changes.
CREATE TABLE IF NOT EXISTS staff_requests (
  id              uuid PRIMARY KEY,
  kind            text NOT NULL CHECK (kind IN ('roles', 'invite', 'role')),
  target          text NOT NULL,
  target_name     text NOT NULL DEFAULT '',
  payload         jsonb NOT NULL,
  reason          text NOT NULL,
  requested_by    text NOT NULL,
  requested_name  text NOT NULL,
  status          text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'rejected', 'cancelled')),
  created_at      timestamptz NOT NULL DEFAULT now(),
  decided_by      text NOT NULL DEFAULT '',
  decided_at      timestamptz
);
CREATE INDEX IF NOT EXISTS staff_requests_open ON staff_requests(status) WHERE status = 'pending';
