-- M1 durable jobs: every multi-step operation (workspace/tenant/boundary provisioning) is a row
-- that survives restarts. Workers claim with FOR UPDATE SKIP LOCKED and hold a renewable lease;
-- an expired lease makes the job claimable again. Status values follow the OpenAPI Job schema.
CREATE TABLE IF NOT EXISTS jobs (
  id            uuid PRIMARY KEY,
  tenant_id     uuid NOT NULL REFERENCES tenants(id),
  kind          text NOT NULL,
  target_type   text NOT NULL,
  target_id     text NOT NULL,
  status        text NOT NULL DEFAULT 'queued'
                CHECK (status IN ('queued','running','succeeded','failed','canceling','canceled')),
  step          text NOT NULL DEFAULT '',
  attempts      int  NOT NULL DEFAULT 0,
  max_attempts  int  NOT NULL DEFAULT 5 CHECK (max_attempts > 0),
  payload       jsonb NOT NULL DEFAULT '{}',
  result        jsonb,
  error         text,
  created_by    text NOT NULL,
  lease_owner   text,
  lease_until   timestamptz,
  run_after     timestamptz NOT NULL DEFAULT now(),
  created_at    timestamptz NOT NULL DEFAULT now(),
  updated_at    timestamptz NOT NULL DEFAULT now(),
  finished_at   timestamptz
);
CREATE INDEX IF NOT EXISTS jobs_claim ON jobs(run_after) WHERE status IN ('queued','running');
CREATE INDEX IF NOT EXISTS jobs_tenant ON jobs(tenant_id, created_at DESC);
-- One live job per target and kind: a second enqueue joins the first instead of racing it.
CREATE UNIQUE INDEX IF NOT EXISTS jobs_live_target ON jobs(kind, target_type, target_id)
  WHERE status IN ('queued','running','canceling');

-- Idempotency-Key replay cache (24h): the first response to (scope, key) is stored and replayed;
-- the same key with a different body is refused. status 0 marks a request still in flight.
CREATE TABLE IF NOT EXISTS idempotency_keys (
  scope         text NOT NULL,
  key           uuid NOT NULL,
  request_hash  text NOT NULL,
  status        int  NOT NULL DEFAULT 0,
  body          bytea,
  headers       jsonb NOT NULL DEFAULT '{}',
  created_at    timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (scope, key)
);
CREATE INDEX IF NOT EXISTS idempotency_keys_age ON idempotency_keys(created_at);
