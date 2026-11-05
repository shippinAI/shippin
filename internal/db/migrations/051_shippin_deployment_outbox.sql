-- The outbox closes the transaction gap between accepted desired state and
-- waking the separately leased deployment worker.
CREATE TABLE shippin_deployment_outbox (
  id             uuid PRIMARY KEY,
  tenant_id      uuid NOT NULL,
  deployment_id  uuid NOT NULL,
  generation     bigint NOT NULL CHECK (generation > 0),
  created_by     text NOT NULL CHECK (length(created_by) BETWEEN 1 AND 255),
  created_at     timestamptz NOT NULL DEFAULT now(),
  job_id         uuid,
  published_at   timestamptz,
  UNIQUE (tenant_id, deployment_id, generation),
  FOREIGN KEY (deployment_id, tenant_id, generation)
    REFERENCES shippin_deployment_generations(deployment_id, tenant_id, generation) ON DELETE RESTRICT,
  CHECK ((job_id IS NULL AND published_at IS NULL) OR (job_id IS NOT NULL AND published_at IS NOT NULL))
);
CREATE INDEX shippin_deployment_outbox_pending
  ON shippin_deployment_outbox(created_at, id) WHERE published_at IS NULL;
