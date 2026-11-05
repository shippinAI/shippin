-- A replacement deployment worker may claim an ambiguous step intent under
-- its current lease, inspect provider state, and record that observation.
ALTER TABLE shippin_deployment_execution_steps
  ADD COLUMN recovery_job_id uuid REFERENCES jobs(id) ON DELETE RESTRICT,
  ADD COLUMN recovery_lease_owner text,
  ADD COLUMN recovery_attempts integer NOT NULL DEFAULT 0 CHECK (recovery_attempts >= 0),
  ADD CONSTRAINT shippin_deployment_execution_steps_recovery_claim_check
    CHECK ((recovery_job_id IS NULL AND recovery_lease_owner IS NULL)
      OR (recovery_job_id IS NOT NULL AND length(recovery_lease_owner) BETWEEN 1 AND 255));

ALTER TABLE shippin_deployment_execution_step_events
  DROP CONSTRAINT shippin_deployment_execution_step_events_event_type_check,
  ADD CONSTRAINT shippin_deployment_execution_step_events_event_type_check
    CHECK (event_type IN ('intent','recovery_claimed','succeeded','failed','unknown'));
