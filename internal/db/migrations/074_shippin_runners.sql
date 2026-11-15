-- Build runners (RUN-01, docs/runners.md): the sizes staff offer, the GitHub repositories an
-- organization assigns to its workspaces and projects, and the jobs GitHub sends for them. Every
-- reference to a workspace, project or installation carries the organization, so the database
-- refuses one organization's row pointing at another's.

CREATE UNIQUE INDEX IF NOT EXISTS github_installations_id_tenant ON github_installations(id, tenant_id);

CREATE TABLE shippin_runner_sizes (
  id         text PRIMARY KEY CHECK (id ~ '^[a-z0-9][a-z0-9-]{0,30}$'), -- a job asks for runs-on: shippin-<id>
  label      text NOT NULL CHECK (length(label) BETWEEN 1 AND 80),
  cores      integer NOT NULL CHECK (cores BETWEEN 1 AND 128),
  memory_mib integer NOT NULL CHECK (memory_mib BETWEEN 512 AND 1048576),
  disk_gib   integer NOT NULL CHECK (disk_gib BETWEEN 8 AND 4096),
  image      text NOT NULL DEFAULT '' CHECK (length(image) <= 200), -- the runner image on the provider
  active     boolean NOT NULL DEFAULT true,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE shippin_github_repositories (
  id              bigint PRIMARY KEY CHECK (id > 0), -- GitHub's repository id
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  installation_id bigint NOT NULL,
  full_name       text NOT NULL CHECK (full_name ~ '^[A-Za-z0-9-]{1,39}/[A-Za-z0-9._-]{1,100}$'),
  private         boolean NOT NULL DEFAULT true,
  workspace_id    uuid NOT NULL,
  project_id      uuid,
  runners         boolean NOT NULL DEFAULT false, -- its jobs may ask for Shippin runners
  updated_by      text NOT NULL,
  created_at      timestamptz NOT NULL DEFAULT now(),
  updated_at      timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (installation_id, tenant_id) REFERENCES github_installations(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (workspace_id, tenant_id) REFERENCES shippin_workspaces(id, tenant_id) ON DELETE CASCADE,
  FOREIGN KEY (project_id, tenant_id) REFERENCES projects(id, tenant_id) ON DELETE CASCADE
);
CREATE INDEX shippin_github_repositories_tenant ON shippin_github_repositories(tenant_id);

CREATE TABLE shippin_runner_jobs (
  id            uuid PRIMARY KEY,
  github_job_id bigint NOT NULL UNIQUE CHECK (github_job_id > 0),
  run_id        bigint NOT NULL DEFAULT 0,
  tenant_id     uuid NOT NULL REFERENCES tenants(id),
  repository_id bigint NOT NULL,
  repository    text NOT NULL CHECK (length(repository) <= 140),
  workspace_id  uuid, -- empty for a job refused because its repository is not assigned
  project_id    uuid,
  name          text NOT NULL DEFAULT '' CHECK (length(name) <= 200),
  labels        text[] NOT NULL DEFAULT '{}',
  size          text, -- the runner size the job asked for, once one matched
  status        text NOT NULL CHECK (status IN ('queued','refused','in_progress','completed','canceled')),
  reason        text NOT NULL DEFAULT '' CHECK (length(reason) <= 300), -- why it was refused, in plain words
  conclusion    text NOT NULL DEFAULT '' CHECK (length(conclusion) <= 40),
  runner_name   text NOT NULL DEFAULT '' CHECK (length(runner_name) <= 100),
  queued_at     timestamptz NOT NULL DEFAULT now(),
  started_at    timestamptz,
  completed_at  timestamptz,
  updated_at    timestamptz NOT NULL DEFAULT now(),
  -- History outlives the workspace or project it ran for.
  FOREIGN KEY (workspace_id, tenant_id) REFERENCES shippin_workspaces(id, tenant_id) ON DELETE SET NULL (workspace_id),
  FOREIGN KEY (project_id, tenant_id) REFERENCES projects(id, tenant_id) ON DELETE SET NULL (project_id)
);
CREATE INDEX shippin_runner_jobs_tenant ON shippin_runner_jobs(tenant_id, queued_at DESC);
CREATE INDEX shippin_runner_jobs_waiting ON shippin_runner_jobs(queued_at) WHERE status = 'queued';

-- The service a plan includes and limits like any other: which sizes, how many jobs at once, and the
-- minutes a month. Off on every plan until staff turn it on in the plan matrix.
INSERT INTO catalog_services(id, label, description, kind, required, config_schema, default_config) VALUES
 ('runners', 'Build runners', 'GitHub Actions jobs on single-use runner VMs, for the repositories an organization assigns.', 'runner', false,
  '{"type":"object","properties":{"sizes":{"type":"array","items":{"type":"string"},"title":"Sizes"},"concurrent":{"type":"integer","minimum":0,"title":"Jobs at once"},"minutes":{"type":"integer","minimum":0,"title":"Minutes a month"}}}',
  '{"sizes":[],"concurrent":0,"minutes":0}')
ON CONFLICT (id) DO NOTHING;
