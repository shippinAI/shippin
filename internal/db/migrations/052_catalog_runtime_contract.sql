-- Catalog runtime declarations are validated compatibility metadata. A definition
-- enters tenant deployment intent discovery only with a pinned source and contract.
ALTER TABLE catalog_services
  ADD COLUMN IF NOT EXISTS runtime_contract jsonb
  CHECK (runtime_contract IS NULL OR (
    deployment_source IS NOT NULL
    AND deployment_source->>'kind' = 'oci'
    AND jsonb_typeof(runtime_contract) = 'object'
    AND runtime_contract ?& ARRAY['version','kind','providers']
    AND runtime_contract->>'version' = '1'
    AND runtime_contract->>'kind' = 'incus_oci_container'
    AND jsonb_typeof(runtime_contract->'providers') = 'array'
    AND runtime_contract->'providers' = '["incus"]'::jsonb
    AND octet_length(runtime_contract::text) <= 4096
  ));
