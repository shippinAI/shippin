-- Persist the deterministic provider-step plan digest separately from the
-- AgentHub action digest so a future executor can revalidate both fences.
-- Existing reviews remain readable but are intentionally not executable.
ALTER TABLE shippin_deployment_reviews
  ADD COLUMN execution_plan_version smallint,
  ADD COLUMN execution_plan_sha256 text
  CHECK (execution_plan_sha256 IS NULL OR execution_plan_sha256 ~ '^[a-f0-9]{64}$'),
  ADD CONSTRAINT shippin_deployment_reviews_execution_plan_check
    CHECK ((execution_plan_version IS NULL AND execution_plan_sha256 IS NULL)
      OR (execution_plan_version > 0 AND execution_plan_sha256 IS NOT NULL));

