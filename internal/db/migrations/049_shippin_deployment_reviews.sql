-- Shippin stores only the correlation to AgentHub's authoritative ticket.
-- It does not duplicate reviewers, decisions, or policy documents.
CREATE TABLE shippin_deployment_reviews (
  id uuid PRIMARY KEY,
  tenant_id uuid NOT NULL,
  deployment_id uuid NOT NULL,
  generation bigint NOT NULL CHECK (generation > 0),
  idempotency_key uuid NOT NULL,
  action_sha256 text NOT NULL CHECK (action_sha256 ~ '^[a-f0-9]{64}$'),
  agenthub_ticket_id text NOT NULL UNIQUE CHECK (length(agenthub_ticket_id) BETWEEN 3 AND 64),
  state text NOT NULL CHECK (state IN ('preparing','pending_review','failed')),
  ticket_sha256 text CHECK (ticket_sha256 IS NULL OR ticket_sha256 ~ '^[a-f0-9]{64}$'),
  failure_code text CHECK (failure_code IS NULL OR failure_code ~ '^[a-z][a-z0-9_.-]{0,79}$'),
  created_by text NOT NULL CHECK (length(created_by) BETWEEN 1 AND 255),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (tenant_id, idempotency_key),
  UNIQUE (tenant_id, deployment_id, generation),
  FOREIGN KEY (deployment_id, tenant_id, generation)
    REFERENCES shippin_deployment_generations(deployment_id, tenant_id, generation) ON DELETE RESTRICT,
  CHECK ((state = 'pending_review' AND ticket_sha256 IS NOT NULL AND failure_code IS NULL)
      OR (state = 'failed' AND failure_code IS NOT NULL)
      OR (state = 'preparing' AND failure_code IS NULL))
);
CREATE INDEX shippin_deployment_reviews_tenant_updated
  ON shippin_deployment_reviews(tenant_id, updated_at DESC, id);

CREATE TABLE shippin_deployment_review_events (
  id uuid PRIMARY KEY,
  review_id uuid NOT NULL REFERENCES shippin_deployment_reviews(id) ON DELETE RESTRICT,
  state text NOT NULL CHECK (state IN ('preparing','pending_review','failed')),
  ticket_sha256 text CHECK (ticket_sha256 IS NULL OR ticket_sha256 ~ '^[a-f0-9]{64}$'),
  failure_code text CHECK (failure_code IS NULL OR failure_code ~ '^[a-z][a-z0-9_.-]{0,79}$'),
  actor text NOT NULL CHECK (length(actor) BETWEEN 1 AND 255),
  recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX shippin_deployment_review_events_review
  ON shippin_deployment_review_events(review_id, recorded_at, id);
CREATE TRIGGER shippin_deployment_review_events_append_only
  BEFORE UPDATE OR DELETE ON shippin_deployment_review_events
  FOR EACH ROW EXECUTE FUNCTION shippin_reject_append_only_mutation();
