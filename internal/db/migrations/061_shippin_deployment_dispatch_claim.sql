-- Consume an exact provider dispatch permit once. A claimed step is ambiguous
-- after a process crash and must be recovered through provider observation.
ALTER TABLE shippin_deployment_execution_steps
  ADD COLUMN dispatch_claimed_at timestamptz;

ALTER TABLE shippin_deployment_execution_step_events
  DROP CONSTRAINT shippin_deployment_execution_step_events_event_type_check,
  ADD CONSTRAINT shippin_deployment_execution_step_events_event_type_check
    CHECK (event_type IN ('intent','recovery_claimed','dispatch_claimed','succeeded','failed','unknown'));
