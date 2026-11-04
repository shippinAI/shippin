-- Deployment artifacts are catalog configuration. This column stores only a pinned source
-- reference and secret reference; it does not claim a runtime has fetched or deployed anything.
ALTER TABLE catalog_services
  ADD COLUMN IF NOT EXISTS deployment_source jsonb
  CHECK (deployment_source IS NULL OR jsonb_typeof(deployment_source) = 'object');
