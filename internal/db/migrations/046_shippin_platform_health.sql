-- Normalized, operator-managed platform health definitions and append-only observations.
-- Existing JSON definitions are imported once, retaining deterministic IDs for legacy rows.
CREATE TABLE shippin_platform_services (
  id uuid PRIMARY KEY,
  name text NOT NULL CHECK (length(trim(name)) BETWEEN 1 AND 80),
  kind text NOT NULL CHECK (length(trim(kind)) BETWEEN 1 AND 40),
  alias text CHECK (alias IS NULL OR length(alias) <= 80),
  url text,
  host_header text,
  tcp_address text,
  instance text NOT NULL DEFAULT '',
  server_id uuid REFERENCES servers(id) ON DELETE RESTRICT,
  guest_id text,
  timeout_seconds integer NOT NULL DEFAULT 3 CHECK (timeout_seconds BETWEEN 1 AND 10),
  interval_seconds integer NOT NULL DEFAULT 60 CHECK (interval_seconds BETWEEN 30 AND 86400),
  enabled boolean NOT NULL DEFAULT true,
  revision bigint NOT NULL DEFAULT 1 CHECK (revision > 0),
  next_probe_at timestamptz NOT NULL DEFAULT now(),
  lease_until timestamptz,
  created_by text NOT NULL DEFAULT 'system',
  updated_by text NOT NULL DEFAULT 'system',
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now(),
  CHECK ((url IS NOT NULL)::int + (tcp_address IS NOT NULL)::int = 1),
  CHECK ((server_id IS NULL) = (guest_id IS NULL))
);
CREATE UNIQUE INDEX shippin_platform_services_name_active
  ON shippin_platform_services (lower(name)) WHERE enabled;
CREATE INDEX shippin_platform_services_due
  ON shippin_platform_services (enabled, next_probe_at, id) WHERE enabled;

CREATE TABLE shippin_platform_service_observations (
  id uuid PRIMARY KEY,
  service_id uuid NOT NULL REFERENCES shippin_platform_services(id) ON DELETE RESTRICT,
  config_revision bigint NOT NULL CHECK (config_revision > 0),
  observed_at timestamptz NOT NULL DEFAULT now(),
  configured boolean NOT NULL,
  healthy boolean NOT NULL,
  latency_ms bigint CHECK (latency_ms IS NULL OR latency_ms BETWEEN 0 AND 3600000),
  detail text NOT NULL CHECK (length(detail) <= 500),
  resource jsonb CHECK (resource IS NULL OR (jsonb_typeof(resource) = 'object' AND octet_length(resource::text) <= 16384)),
  CHECK (NOT healthy OR configured)
);
CREATE INDEX shippin_platform_service_observations_latest
  ON shippin_platform_service_observations (service_id, observed_at DESC, id);
CREATE INDEX shippin_platform_service_observations_retention
  ON shippin_platform_service_observations (observed_at);

-- Upgrade compatibility: preserve prior UI-authored probes and make re-import impossible.
INSERT INTO shippin_platform_services
  (id, name, kind, url, host_header, tcp_address, instance, server_id, guest_id,
   timeout_seconds, created_by, updated_by)
SELECT
  md5('shippin-platform-health:' || lower(trim(p->>'name')))::uuid,
  trim(p->>'name'), trim(p->>'kind'), nullif(p->>'url',''), nullif(p->>'host',''),
  nullif(p->>'tcp',''), coalesce(p->>'instance',''), srv.id,
  CASE WHEN srv.id IS NULL THEN NULL ELSE nullif(p->>'guest_id','') END, greatest(1, least(10, coalesce(nullif(p->>'timeout_seconds','')::int, 3))),
  'legacy-import', 'legacy-import'
FROM platform_settings ps
CROSS JOIN LATERAL jsonb_array_elements(CASE WHEN jsonb_typeof(ps.value)='array' THEN ps.value ELSE '[]'::jsonb END) p
LEFT JOIN servers srv ON srv.id = CASE
  WHEN p->>'server_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$'
  THEN (p->>'server_id')::uuid END
WHERE ps.key = 'services.probes'
  AND jsonb_typeof(ps.value::jsonb) = 'array'
  AND nullif(trim(p->>'name'),'') IS NOT NULL
  AND nullif(trim(p->>'kind'),'') IS NOT NULL
  AND ((nullif(p->>'url','') IS NOT NULL)::int + (nullif(p->>'tcp','') IS NOT NULL)::int = 1)
  AND (nullif(p->>'url','') IS NULL OR p->>'url' ~ '^https?://[^/@?#]+(/[^?#]*)?$')
  AND (nullif(p->>'server_id','') IS NULL OR p->>'server_id' ~* '^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$')
  AND length(coalesce(p->>'guest_id','')) <= 128
  AND coalesce(p->>'guest_id','') !~ '[[:cntrl:]]'
  AND (nullif(p->>'timeout_seconds','') IS NULL OR p->>'timeout_seconds' ~ '^[0-9]{1,2}$')
ON CONFLICT DO NOTHING;

CREATE TABLE shippin_platform_health_imports (
  source text PRIMARY KEY CHECK (source IN ('services.probes.setting','services.probes.config')),
  imported_at timestamptz NOT NULL DEFAULT now(),
  imported_count integer NOT NULL CHECK (imported_count >= 0)
);
INSERT INTO shippin_platform_health_imports(source, imported_count)
SELECT 'services.probes.setting', count(*)::int FROM shippin_platform_services
WHERE created_by = 'legacy-import';
