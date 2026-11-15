-- Runner machines (RUN-01, docs/runners.md): the single-use machine started for one runner job, where
-- it runs, and how far it got. One machine per job, ever, so a job is never started twice. The
-- organization is carried in the reference to the job, so the database refuses one organization's
-- machine for another's job. The just-in-time runner configuration is never stored here.

CREATE UNIQUE INDEX IF NOT EXISTS shippin_runner_jobs_id_tenant ON shippin_runner_jobs(id, tenant_id);

CREATE TABLE shippin_runner_machines (
  id              uuid PRIMARY KEY,
  tenant_id       uuid NOT NULL REFERENCES tenants(id),
  runner_job_id   uuid NOT NULL UNIQUE,
  -- The job GitHub actually gave this machine's runner, once it says so; GitHub may hand a waiting
  -- runner any queued job of the same repository and labels.
  assigned_job_id uuid,
  server_id       uuid REFERENCES servers(id) ON DELETE SET NULL, -- the host it runs on, once placed
  name            text NOT NULL UNIQUE CHECK (name ~ '^shr-[a-z0-9]{12}$'),
  guest           text NOT NULL DEFAULT '' CHECK (length(guest) <= 100), -- the hypervisor's id; also the GitHub runner's name
  github_runner_id bigint,
  size            text NOT NULL CHECK (length(size) <= 40),
  attempt         integer NOT NULL DEFAULT 0 CHECK (attempt >= 0),
  -- While an attempt to start it runs, it holds the machine until then, so a second run cannot start
  -- it again; an attempt that outlives it has given up.
  busy_until      timestamptz,
  state           text NOT NULL CHECK (state IN ('starting','running','removing','removed','failed')),
  error           text NOT NULL DEFAULT '' CHECK (length(error) <= 500),
  removal_reason  text NOT NULL DEFAULT '' CHECK (length(removal_reason) <= 300),
  created_at      timestamptz NOT NULL DEFAULT now(),
  started_at      timestamptz,
  removed_at      timestamptz,
  updated_at      timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (runner_job_id, tenant_id) REFERENCES shippin_runner_jobs(id, tenant_id),
  FOREIGN KEY (assigned_job_id, tenant_id) REFERENCES shippin_runner_jobs(id, tenant_id) ON DELETE SET NULL (assigned_job_id)
);
CREATE INDEX shippin_runner_machines_tenant ON shippin_runner_machines(tenant_id, created_at DESC);
-- What is not yet removed holds the organization's capacity, and the host's.
CREATE INDEX shippin_runner_machines_live ON shippin_runner_machines(tenant_id, server_id) WHERE state <> 'removed';
