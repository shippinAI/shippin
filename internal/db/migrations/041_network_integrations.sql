-- Installation-scoped network provider connections and observed resources. Credentials are
-- authenticated ciphertext bound to the row id; config and resource metadata never hold secrets.
CREATE TABLE IF NOT EXISTS shippin_network_connections (
  id                    uuid PRIMARY KEY,
  label                 text NOT NULL CHECK (length(label) BETWEEN 1 AND 120),
  provider              text NOT NULL CHECK (provider IN ('cloudflare_tunnel','tailscale','custom')),
  config                jsonb NOT NULL CHECK (jsonb_typeof(config) = 'object'),
  credential_ciphertext bytea,
  sync_interval_seconds int NOT NULL DEFAULT 300 CHECK (sync_interval_seconds BETWEEN 60 AND 86400),
  status                text NOT NULL DEFAULT 'unknown' CHECK (status IN ('unknown','healthy','degraded')),
  last_checked_at       timestamptz,
  lease_until           timestamptz,
  last_error            text NOT NULL DEFAULT '' CHECK (length(last_error) <= 500),
  created_by            text NOT NULL,
  created_at            timestamptz NOT NULL DEFAULT now(),
  updated_at            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS shippin_network_connections_provider ON shippin_network_connections(provider,label);
CREATE INDEX IF NOT EXISTS shippin_network_connections_due ON shippin_network_connections(lease_until,last_checked_at);

CREATE TABLE IF NOT EXISTS shippin_network_resources (
  connection_id uuid NOT NULL REFERENCES shippin_network_connections(id) ON DELETE CASCADE,
  external_id   text NOT NULL CHECK (length(external_id) BETWEEN 1 AND 512),
  provider      text NOT NULL CHECK (provider IN ('cloudflare_tunnel','tailscale','custom')),
  kind          text NOT NULL CHECK (length(kind) BETWEEN 1 AND 80),
  name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 512),
  state         text NOT NULL DEFAULT '',
  address       text NOT NULL DEFAULT '',
  details       jsonb NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(details) = 'object'),
  observed_at   timestamptz NOT NULL,
  PRIMARY KEY (connection_id, external_id)
);
CREATE INDEX IF NOT EXISTS shippin_network_resources_kind ON shippin_network_resources(provider,kind,name);

-- The configurable network integrations are owned by the existing infrastructure operators.
UPDATE staff_roles SET permissions = (
  SELECT array_agg(DISTINCT p ORDER BY p) FROM unnest(permissions || ARRAY['infra.network.write']) AS p
), updated_at=now() WHERE key IN ('staff.owner','staff.infra');
