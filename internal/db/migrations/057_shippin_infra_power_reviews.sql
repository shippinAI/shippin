-- Correlation and execution journal for AgentHub-reviewed guest power actions.
-- AgentHub remains the sole owner of policy, reviewers and approval decisions.
CREATE TABLE shippin_infra_power_reviews (
  id uuid PRIMARY KEY,
  server_id uuid NOT NULL REFERENCES servers(id) ON DELETE RESTRICT,
  guest text NOT NULL CHECK (length(guest) BETWEEN 1 AND 255),
  verb text NOT NULL CHECK (verb IN ('start','stop','shutdown','reboot')),
  provider text NOT NULL CHECK (provider IN ('incus','proxmox')),
  endpoint_sha256 text NOT NULL CHECK (endpoint_sha256 ~ '^[a-f0-9]{64}$'),
  action_sha256 text NOT NULL CHECK (action_sha256 ~ '^[a-f0-9]{64}$'),
  idempotency_key uuid NOT NULL,
  agenthub_ticket_id text NOT NULL UNIQUE CHECK (length(agenthub_ticket_id) BETWEEN 3 AND 64),
  ticket_sha256 text CHECK (ticket_sha256 IS NULL OR ticket_sha256 ~ '^[a-f0-9]{64}$'),
  state text NOT NULL CHECK (state IN ('preparing','pending_review','failed','executing','succeeded','failed_execution','unknown','denied')),
  failure_code text CHECK (failure_code IS NULL OR failure_code ~ '^[a-z][a-z0-9_.-]{0,79}$'),
  provenance jsonb CHECK (provenance IS NULL OR (jsonb_typeof(provenance)='object' AND octet_length(provenance::text)<=32768)),
  actor text NOT NULL CHECK (length(actor) BETWEEN 1 AND 255),
  actor_name text NOT NULL DEFAULT '' CHECK (length(actor_name)<=255),
  change_note text NOT NULL CHECK (length(change_note) BETWEEN 5 AND 300),
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (server_id, idempotency_key),
  CHECK ((state='pending_review' AND ticket_sha256 IS NOT NULL AND failure_code IS NULL)
      OR (state IN ('preparing','executing') AND failure_code IS NULL)
      OR (state IN ('failed','failed_execution','unknown','denied') AND failure_code IS NOT NULL)
      OR (state='succeeded' AND failure_code IS NULL))
);
CREATE INDEX shippin_infra_power_reviews_server_updated
  ON shippin_infra_power_reviews(server_id, updated_at DESC, id);
CREATE INDEX shippin_infra_power_reviews_open
  ON shippin_infra_power_reviews(state) WHERE state IN ('preparing','pending_review','executing');
CREATE TABLE shippin_infra_power_review_events (
  id uuid PRIMARY KEY,
  review_id uuid NOT NULL REFERENCES shippin_infra_power_reviews(id) ON DELETE RESTRICT,
  state text NOT NULL CHECK (state IN ('preparing','pending_review','failed','executing','succeeded','failed_execution','unknown','denied')),
  failure_code text CHECK (failure_code IS NULL OR failure_code ~ '^[a-z][a-z0-9_.-]{0,79}$'),
  provenance jsonb CHECK (provenance IS NULL OR (jsonb_typeof(provenance)='object' AND octet_length(provenance::text)<=32768)),
  actor text NOT NULL CHECK (length(actor) BETWEEN 1 AND 255),
  recorded_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX shippin_infra_power_review_events_review
  ON shippin_infra_power_review_events(review_id,recorded_at,id);
CREATE TRIGGER shippin_infra_power_review_events_append_only
  BEFORE UPDATE OR DELETE ON shippin_infra_power_review_events
  FOR EACH ROW EXECUTE FUNCTION shippin_reject_append_only_mutation();
