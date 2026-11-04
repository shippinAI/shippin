-- Desired and observed deployment state is separate from catalog metadata and worker leases.
-- Generation, request-key, and operation-event rows are immutable audit inputs/outputs.
CREATE TABLE shippin_deployments (
  id                 uuid PRIMARY KEY,
  tenant_id          uuid NOT NULL REFERENCES tenants(id) ON DELETE RESTRICT,
  service_id         text NOT NULL REFERENCES catalog_services(id) ON DELETE RESTRICT,
  target_system_id   uuid NOT NULL,
  target_provider    text NOT NULL CHECK (target_provider IN ('incus','proxmox')),
  desired_generation bigint NOT NULL CHECK (desired_generation > 0),
  observed_state     text NOT NULL DEFAULT 'unknown'
    CHECK (observed_state IN ('unknown','pending','provisioning','ready','degraded','failed','absent')),
  created_by         text NOT NULL CHECK (length(created_by) BETWEEN 1 AND 255),
  updated_by         text NOT NULL CHECK (length(updated_by) BETWEEN 1 AND 255),
  created_at         timestamptz NOT NULL DEFAULT now(),
  updated_at         timestamptz NOT NULL DEFAULT now(),
  UNIQUE (id, tenant_id),
  FOREIGN KEY (target_system_id, tenant_id)
    REFERENCES server_assignments(server_id, tenant_id) ON DELETE RESTRICT
);
CREATE INDEX shippin_deployments_tenant_updated
  ON shippin_deployments(tenant_id, updated_at DESC, id);
CREATE INDEX shippin_deployments_target
  ON shippin_deployments(target_system_id, observed_state);

CREATE TABLE shippin_deployment_generations (
  deployment_id      uuid NOT NULL,
  tenant_id          uuid NOT NULL,
  generation         bigint NOT NULL CHECK (generation > 0),
  desired_state      text NOT NULL CHECK (desired_state IN ('present','absent')),
  definition_sha256  text NOT NULL CHECK (definition_sha256 ~ '^[a-f0-9]{64}$'),
  definition_snapshot jsonb NOT NULL CHECK (jsonb_typeof(definition_snapshot) = 'object'
    AND octet_length(definition_snapshot::text) <= 262144),
  config             jsonb NOT NULL CHECK (jsonb_typeof(config) = 'object'
    AND octet_length(config::text) <= 131072),
  idempotency_key    uuid NOT NULL,
  request_sha256     text NOT NULL CHECK (request_sha256 ~ '^[a-f0-9]{64}$'),
  created_by         text NOT NULL CHECK (length(created_by) BETWEEN 1 AND 255),
  created_at         timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (deployment_id, tenant_id, generation),
  UNIQUE (tenant_id, idempotency_key),
  FOREIGN KEY (deployment_id, tenant_id)
    REFERENCES shippin_deployments(id, tenant_id) ON DELETE RESTRICT
);
CREATE INDEX shippin_deployment_generations_tenant
  ON shippin_deployment_generations(tenant_id, deployment_id, generation DESC);

CREATE TABLE shippin_deployment_events (
  id                    uuid PRIMARY KEY,
  deployment_id         uuid NOT NULL,
  tenant_id             uuid NOT NULL,
  generation            bigint NOT NULL,
  event_type            text NOT NULL CHECK (event_type IN (
    'requested','review_allowed','review_denied','operation_started',
    'provider_observed','health_observed','operation_failed','operation_cancelled'
  )),
  observed_state        text CHECK (observed_state IS NULL OR observed_state IN (
    'unknown','pending','provisioning','ready','degraded','failed','absent'
  )),
  provider_operation_id text CHECK (provider_operation_id IS NULL OR length(provider_operation_id) <= 255),
  health_status         text CHECK (health_status IS NULL OR health_status IN ('unknown','passing','failing')),
  health_duration_ms    bigint CHECK (health_duration_ms IS NULL OR health_duration_ms BETWEEN 0 AND 3600000),
  failure_code          text CHECK (failure_code IS NULL OR failure_code ~ '^[a-z][a-z0-9_.-]{0,79}$'),
  actor                 text NOT NULL CHECK (length(actor) BETWEEN 1 AND 255),
  recorded_at           timestamptz NOT NULL DEFAULT now(),
  FOREIGN KEY (deployment_id, tenant_id, generation)
    REFERENCES shippin_deployment_generations(deployment_id, tenant_id, generation) ON DELETE RESTRICT
);
CREATE INDEX shippin_deployment_events_tenant
  ON shippin_deployment_events(tenant_id, deployment_id, recorded_at DESC, id);

CREATE FUNCTION shippin_reject_append_only_mutation() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
  RAISE EXCEPTION '% is append-only; % is not permitted', TG_TABLE_NAME, TG_OP
    USING ERRCODE = '55000';
END;
$$;
CREATE TRIGGER shippin_deployment_generations_append_only
  BEFORE UPDATE OR DELETE ON shippin_deployment_generations
  FOR EACH ROW EXECUTE FUNCTION shippin_reject_append_only_mutation();
CREATE TRIGGER shippin_deployment_events_append_only
  BEFORE UPDATE OR DELETE ON shippin_deployment_events
  FOR EACH ROW EXECUTE FUNCTION shippin_reject_append_only_mutation();
