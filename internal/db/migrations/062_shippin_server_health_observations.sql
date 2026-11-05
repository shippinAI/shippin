ALTER TABLE servers
  ADD COLUMN hv_revision bigint NOT NULL DEFAULT 1 CHECK (hv_revision > 0);

CREATE TABLE shippin_server_health_observations (
  server_id uuid PRIMARY KEY REFERENCES servers(id) ON DELETE CASCADE,
  endpoint_revision bigint NOT NULL CHECK (endpoint_revision > 0),
  state text NOT NULL CHECK (state IN ('operational','down','unknown')),
  observed_at timestamptz NOT NULL,
  latency_ms bigint CHECK (latency_ms IS NULL OR latency_ms BETWEEN 0 AND 3600000),
  version text NOT NULL DEFAULT '' CHECK (length(version) <= 120),
  detail text NOT NULL DEFAULT '' CHECK (length(detail) <= 500)
);
