-- Version 1 remains readable for compatibility. Version 2 captures a bounded
-- Incus OCI execution recipe; Go performs the detailed semantic validation.
ALTER TABLE catalog_services
  DROP CONSTRAINT IF EXISTS catalog_services_check;

ALTER TABLE catalog_services
  DROP CONSTRAINT IF EXISTS catalog_services_runtime_contract_check;

ALTER TABLE catalog_services
  ADD CONSTRAINT catalog_services_runtime_contract_check CHECK (
    runtime_contract IS NULL OR (
      deployment_source IS NOT NULL
      AND deployment_source->>'kind' = 'oci'
      AND jsonb_typeof(runtime_contract) = 'object'
      AND runtime_contract ?& ARRAY['version','kind','providers']
      AND runtime_contract->>'kind' = 'incus_oci_container'
      AND jsonb_typeof(runtime_contract->'providers') = 'array'
      AND runtime_contract->'providers' = '["incus"]'::jsonb
      AND octet_length(runtime_contract::text) <= 16384
      AND (
        (runtime_contract->>'version' = '1'
          AND NOT (runtime_contract ?| ARRAY['resources','storage','network','health']))
        OR
        (runtime_contract->>'version' = '2'
          AND runtime_contract ?& ARRAY['resources','storage','network','health']
          AND jsonb_typeof(runtime_contract->'resources') = 'object'
          AND (runtime_contract->'resources') ?& ARRAY['cpu_cores','memory_mib']
          AND jsonb_typeof(runtime_contract->'resources'->'cpu_cores') = 'number'
          AND jsonb_typeof(runtime_contract->'resources'->'memory_mib') = 'number'
          AND jsonb_typeof(runtime_contract->'storage') = 'object'
          AND (runtime_contract->'storage') ?& ARRAY['size_gib','mount_path']
          AND jsonb_typeof(runtime_contract->'storage'->'size_gib') = 'number'
          AND jsonb_typeof(runtime_contract->'storage'->'mount_path') = 'string'
          AND jsonb_typeof(runtime_contract->'network') = 'object'
          AND (runtime_contract->'network') ?& ARRAY['exposure','ports']
          AND jsonb_typeof(runtime_contract->'network'->'exposure') = 'string'
          AND jsonb_typeof(runtime_contract->'network'->'ports') = 'array'
          AND jsonb_typeof(runtime_contract->'health') = 'object'
          AND (runtime_contract->'health') ?& ARRAY['kind','port','interval_seconds','timeout_seconds','failure_threshold']
          AND jsonb_typeof(runtime_contract->'health'->'kind') = 'string'
          AND jsonb_typeof(runtime_contract->'health'->'port') = 'number'
          AND jsonb_typeof(runtime_contract->'health'->'interval_seconds') = 'number'
          AND jsonb_typeof(runtime_contract->'health'->'timeout_seconds') = 'number'
          AND jsonb_typeof(runtime_contract->'health'->'failure_threshold') = 'number')
      )
    )
  );
