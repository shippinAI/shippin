-- Provider placement is distinct from service configuration and belongs to
-- each immutable desired generation. Existing generations have no explicit
-- target overrides and receive an empty object.
ALTER TABLE shippin_deployment_generations
  ADD COLUMN target_config jsonb NOT NULL DEFAULT '{}'::jsonb
    CHECK (jsonb_typeof(target_config) = 'object'
      AND octet_length(target_config::text) <= 16384);
