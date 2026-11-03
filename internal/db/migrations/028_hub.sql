-- Agent hub (AGENT-TICKETS-PLAN): a checkout board. Agents claim a task under a lease, post
-- progress, and may ask for sign-off; only the configured owner approves or rejects.
CREATE TABLE IF NOT EXISTS hub_tasks (
  id          uuid PRIMARY KEY,
  org_id      text NOT NULL,
  title       text NOT NULL CHECK (length(title) BETWEEN 1 AND 200),
  body        text NOT NULL DEFAULT '' CHECK (length(body) <= 8000),
  status      text NOT NULL DEFAULT 'open' CHECK (status IN
                ('open','claimed','in_progress','needs_approval','approved','done','rejected')),
  progress    text NOT NULL DEFAULT '',
  lease_owner text,
  lease_until timestamptz,
  approval_note text NOT NULL DEFAULT '',
  decided_by  text,
  decided_at  timestamptz,
  created_by  text NOT NULL,
  created_at  timestamptz NOT NULL DEFAULT now(),
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS hub_tasks_org ON hub_tasks(org_id, status, created_at);
CREATE TABLE IF NOT EXISTS hub_events (
  id      bigserial PRIMARY KEY,
  task_id uuid NOT NULL REFERENCES hub_tasks(id) ON DELETE CASCADE,
  actor   text NOT NULL,
  kind    text NOT NULL,
  note    text NOT NULL DEFAULT '',
  at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS hub_events_task ON hub_events(task_id, id);
