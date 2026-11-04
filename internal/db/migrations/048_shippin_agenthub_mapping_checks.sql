CREATE TABLE shippin_agenthub_mapping_checks (
  id uuid PRIMARY KEY,
  server_id uuid NOT NULL REFERENCES servers(id) ON DELETE RESTRICT,
  tenant_id text NOT NULL CHECK (length(tenant_id) BETWEEN 3 AND 255),
  machine_id text NOT NULL CHECK (length(machine_id) BETWEEN 3 AND 255),
  outcome text NOT NULL CHECK (outcome IN ('verified', 'failed')),
  policy_version bigint CHECK (policy_version IS NULL OR policy_version > 0),
  policy_sha256 text CHECK (policy_sha256 IS NULL OR policy_sha256 ~ '^[a-f0-9]{64}$'),
  detail text NOT NULL CHECK (length(detail) <= 240),
  checked_by text NOT NULL,
  checked_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((outcome = 'verified' AND policy_version IS NOT NULL AND policy_sha256 IS NOT NULL)
      OR (outcome = 'failed' AND policy_version IS NULL AND policy_sha256 IS NULL))
);
CREATE INDEX shippin_agenthub_mapping_checks_latest
  ON shippin_agenthub_mapping_checks (server_id, checked_at DESC, id);
