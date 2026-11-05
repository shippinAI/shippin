-- Bound the local execution window for AgentHub-reviewed guest power actions.
-- AgentHub continues to own ticket state, policy and reviewers.
ALTER TABLE shippin_infra_power_reviews
  ADD COLUMN expires_at timestamptz;

UPDATE shippin_infra_power_reviews
  SET expires_at = created_at + interval '30 minutes'
  WHERE expires_at IS NULL;

ALTER TABLE shippin_infra_power_reviews
  ALTER COLUMN expires_at SET NOT NULL,
  DROP CONSTRAINT shippin_infra_power_reviews_state_check,
  ADD CONSTRAINT shippin_infra_power_reviews_state_check
    CHECK (state IN ('preparing','pending_review','failed','executing','succeeded','failed_execution','unknown','denied','expired')),
  DROP CONSTRAINT shippin_infra_power_reviews_check,
  ADD CONSTRAINT shippin_infra_power_reviews_check
    CHECK ((state='pending_review' AND ticket_sha256 IS NOT NULL AND failure_code IS NULL)
        OR (state IN ('preparing','executing') AND failure_code IS NULL)
        OR (state IN ('failed','failed_execution','unknown','denied') AND failure_code IS NOT NULL)
        OR (state='expired' AND failure_code='review_expired')
        OR (state='succeeded' AND failure_code IS NULL));

ALTER TABLE shippin_infra_power_review_events
  DROP CONSTRAINT shippin_infra_power_review_events_state_check,
  ADD CONSTRAINT shippin_infra_power_review_events_state_check
    CHECK (state IN ('preparing','pending_review','failed','executing','succeeded','failed_execution','unknown','denied','expired'));
