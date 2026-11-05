ALTER TABLE shippin_deployment_events
  ADD COLUMN review_provenance jsonb,
  ADD COLUMN idempotency_key uuid;

ALTER TABLE shippin_deployment_events
  ADD CONSTRAINT shippin_deployment_events_review_decision CHECK (
    (event_type = 'review_allowed'
      AND review_provenance IS NOT NULL
      AND jsonb_typeof(review_provenance) = 'object'
      AND octet_length(review_provenance::text) <= 32768
      AND review_provenance ?& ARRAY['decision_id','tenant_id','machine_id','ticket_id','ticket_sha256','policy_sha256','policy_version','matched_rules','action_sha256']
      AND jsonb_typeof(review_provenance->'decision_id') = 'string'
      AND jsonb_typeof(review_provenance->'tenant_id') = 'string'
      AND jsonb_typeof(review_provenance->'machine_id') = 'string'
      AND jsonb_typeof(review_provenance->'ticket_id') = 'string'
      AND jsonb_typeof(review_provenance->'ticket_sha256') = 'string'
      AND jsonb_typeof(review_provenance->'policy_sha256') = 'string'
      AND jsonb_typeof(review_provenance->'policy_version') = 'number'
      AND (review_provenance->>'policy_version')::bigint > 0
      AND jsonb_typeof(review_provenance->'matched_rules') = 'array'
      AND jsonb_array_length(review_provenance->'matched_rules') > 0
      AND jsonb_array_length(review_provenance->'matched_rules') <= 64
      AND jsonb_typeof(review_provenance->'action_sha256') = 'string'
      AND review_provenance->>'decision_id' ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$'
      AND review_provenance->>'tenant_id' ~ '^[A-Za-z0-9][A-Za-z0-9._-]{2,63}$'
      AND review_provenance->>'machine_id' ~ '^[A-Za-z0-9][A-Za-z0-9._-]{2,63}$'
      AND review_provenance->>'ticket_id' ~ '^shp-[a-z0-9._-]{1,60}$'
      AND review_provenance->>'ticket_sha256' ~ '^[a-f0-9]{64}$'
      AND review_provenance->>'policy_sha256' ~ '^[a-f0-9]{64}$'
      AND review_provenance->>'action_sha256' ~ '^[a-f0-9]{64}$'
      AND failure_code IS NULL
      AND idempotency_key IS NOT NULL)
    OR (event_type = 'review_denied'
      AND review_provenance IS NULL
      AND failure_code IS NOT NULL
      AND idempotency_key IS NOT NULL)
    OR (event_type NOT IN ('review_allowed','review_denied')
      AND review_provenance IS NULL
      AND idempotency_key IS NULL)
  ) NOT VALID;
CREATE UNIQUE INDEX shippin_deployment_review_decision_idempotency
  ON shippin_deployment_events(tenant_id,idempotency_key)
  WHERE idempotency_key IS NOT NULL;
