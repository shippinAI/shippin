-- Durable, plan-bound worker intent/outcome. An intent without an outcome is
-- ambiguous after a crash and must be reconciled by provider read-back.
CREATE TABLE shippin_deployment_execution_steps (
  tenant_id uuid NOT NULL,
  deployment_id uuid NOT NULL,
  generation bigint NOT NULL CHECK (generation > 0),
  plan_version smallint NOT NULL CHECK (plan_version > 0),
  plan_sha256 text NOT NULL CHECK (plan_sha256 ~ '^[a-f0-9]{64}$'),
  step_id text NOT NULL CHECK (step_id ~ '^[a-z][a-z0-9_-]{0,39}$'),
  operation text NOT NULL CHECK (operation ~ '^[a-z][a-z0-9_-]{0,79}$'),
  idempotency_key text NOT NULL CHECK (idempotency_key ~ '^[a-f0-9]{64}$'),
  state text NOT NULL CHECK (state IN ('intent','succeeded','failed','unknown')),
  attempt integer NOT NULL DEFAULT 1 CHECK (attempt = 1),
  intent_job_id uuid NOT NULL REFERENCES jobs(id) ON DELETE RESTRICT,
  intent_lease_owner text NOT NULL CHECK (length(intent_lease_owner) BETWEEN 1 AND 255),
  provider_operation_id text CHECK (provider_operation_id IS NULL OR length(provider_operation_id) BETWEEN 1 AND 255),
  failure_code text CHECK (failure_code IS NULL OR failure_code ~ '^[a-z][a-z0-9_.-]{0,79}$'),
  intent_at timestamptz NOT NULL DEFAULT now(),
  completed_at timestamptz,
  PRIMARY KEY (tenant_id, idempotency_key),
  UNIQUE (tenant_id, deployment_id, generation, step_id),
  FOREIGN KEY (deployment_id, tenant_id, generation)
    REFERENCES shippin_deployment_generations(deployment_id, tenant_id, generation) ON DELETE RESTRICT,
  CHECK ((state = 'intent' AND completed_at IS NULL AND failure_code IS NULL)
      OR (state = 'succeeded' AND completed_at IS NOT NULL AND failure_code IS NULL)
      OR (state = 'failed' AND completed_at IS NOT NULL AND failure_code IS NOT NULL)
      OR (state = 'unknown' AND completed_at IS NOT NULL))
);
CREATE INDEX shippin_deployment_execution_steps_recovery
  ON shippin_deployment_execution_steps(tenant_id, deployment_id, generation, state, intent_at);

CREATE TABLE shippin_deployment_execution_step_events (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  idempotency_key text NOT NULL,
  event_type text NOT NULL CHECK (event_type IN ('intent','succeeded','failed','unknown')),
  actor text NOT NULL CHECK (length(actor) BETWEEN 1 AND 255),
  provider_operation_id text CHECK (provider_operation_id IS NULL OR length(provider_operation_id) BETWEEN 1 AND 255),
  failure_code text CHECK (failure_code IS NULL OR failure_code ~ '^[a-z][a-z0-9_.-]{0,79}$'),
  recorded_at timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (tenant_id, idempotency_key)
    REFERENCES shippin_deployment_execution_steps(tenant_id, idempotency_key) ON DELETE RESTRICT
);
CREATE INDEX shippin_deployment_execution_step_events_key
  ON shippin_deployment_execution_step_events(tenant_id, idempotency_key, recorded_at, id);
CREATE TRIGGER shippin_deployment_execution_step_events_append_only
  BEFORE UPDATE OR DELETE ON shippin_deployment_execution_step_events
  FOR EACH ROW EXECUTE FUNCTION shippin_reject_append_only_mutation();
