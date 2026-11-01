-- M4 prompt -> project (implementation-plan-3 "M4"). The user's OWN signed-in CLI agent builds the app
-- inside their workspace; Shippin stores no token and no full prompt: only its hash and a preview.
-- prompt_pending holds the text only until hostd has written ~/.shippin/prompt.md, then it is NULLed.
CREATE TABLE IF NOT EXISTS agent_runs (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  project_id      uuid NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
  workspace_id    uuid REFERENCES workspaces(id),
  account_id      uuid NOT NULL REFERENCES accounts(id),
  created_by      text NOT NULL,
  provider        text NOT NULL CHECK (provider ~ '^[a-z][a-z0-9-]{1,31}$'),
  template        text NOT NULL DEFAULT '',
  prompt_sha256   bytea NOT NULL CHECK (length(prompt_sha256) = 32),
  prompt_preview  text NOT NULL DEFAULT '' CHECK (length(prompt_preview) <= 200),
  prompt_pending  text CHECK (prompt_pending IS NULL OR octet_length(prompt_pending) <= 65536),
  state           text NOT NULL DEFAULT 'queued'
                  CHECK (state IN ('queued','provisioning','waiting_ai','starting','running','exited','failed')),
  error           text,
  started_at      timestamptz,
  ended_at        timestamptz,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS agent_runs_project ON agent_runs(project_id, created_at DESC);
CREATE INDEX IF NOT EXISTS agent_runs_waiting ON agent_runs(account_id, tenant_id) WHERE state = 'waiting_ai';
