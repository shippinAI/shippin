-- Explicit identity facts and AgentHub mapping for the existing Shippin server UUID.
-- Hostname is observed by hostd; aliases/tags and external IDs are operator-maintained metadata.
ALTER TABLE servers
  ADD COLUMN IF NOT EXISTS hostname_source text NOT NULL DEFAULT 'unknown'
    CHECK (hostname_source IN ('unknown','hostd_enrollment')),
  ADD COLUMN IF NOT EXISTS hostname_observed_at timestamptz,
  ADD COLUMN IF NOT EXISTS aliases text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS tags text[] NOT NULL DEFAULT '{}',
  ADD COLUMN IF NOT EXISTS agenthub_tenant_id text,
  ADD COLUMN IF NOT EXISTS agenthub_machine_id text,
  ADD COLUMN IF NOT EXISTS identity_updated_by text NOT NULL DEFAULT '',
  ADD COLUMN IF NOT EXISTS identity_updated_at timestamptz,
  ADD CONSTRAINT servers_agenthub_mapping_pair CHECK (
    (agenthub_tenant_id IS NULL) = (agenthub_machine_id IS NULL)
  );

CREATE UNIQUE INDEX IF NOT EXISTS servers_agenthub_machine_mapping
  ON servers (agenthub_tenant_id, agenthub_machine_id)
  WHERE agenthub_tenant_id IS NOT NULL AND agenthub_machine_id IS NOT NULL;
