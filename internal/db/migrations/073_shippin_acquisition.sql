-- Private source imports for acquisition review. Early-access consent is not
-- a verified waitlist membership or customer account.
CREATE TABLE shippin_acquisition_sources (
  id uuid PRIMARY KEY,
  label text NOT NULL CHECK (length(label) BETWEEN 1 AND 100),
  origin text NOT NULL CHECK (length(origin) BETWEEN 1 AND 512),
  dial_ip inet NOT NULL,
  port integer NOT NULL CHECK (port BETWEEN 1 AND 65535),
  source_tenant text NOT NULL CHECK (length(source_tenant) BETWEEN 1 AND 100),
  source_project text NOT NULL CHECK (length(source_project) BETWEEN 1 AND 100),
  issuer text NOT NULL CHECK (length(issuer) BETWEEN 1 AND 512),
  audience text NOT NULL CHECK (length(audience) BETWEEN 1 AND 512),
  ca_pem text NOT NULL CHECK (length(ca_pem) BETWEEN 1 AND 65536),
  sealed_signer bytea NOT NULL CHECK (octet_length(sealed_signer) BETWEEN 1 AND 4096),
  config_generation bigint NOT NULL DEFAULT 1 CHECK (config_generation > 0),
  cursor bigint NOT NULL DEFAULT 0 CHECK (cursor >= 0),
  paused boolean NOT NULL DEFAULT true,
  latest_success_at timestamptz,
  created_at timestamptz NOT NULL DEFAULT now(),
  updated_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE shippin_acquisition_leads (
  source_id uuid NOT NULL REFERENCES shippin_acquisition_sources(id) ON DELETE CASCADE,
  lead_id text NOT NULL CHECK (lead_id ~ '^[a-f0-9]{32}$'),
  revision bigint NOT NULL CHECK (revision > 0),
  expires_at timestamptz,
  deleted boolean NOT NULL DEFAULT false,
  sealed_payload bytea,
  imported_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (source_id, lead_id),
  UNIQUE (source_id, revision),
  CHECK ((deleted AND sealed_payload IS NULL) OR (NOT deleted AND sealed_payload IS NOT NULL)),
  CHECK ((deleted AND expires_at IS NULL) OR (NOT deleted AND expires_at IS NOT NULL))
);

CREATE INDEX shippin_acquisition_leads_active
  ON shippin_acquisition_leads(source_id, revision DESC)
  WHERE NOT deleted;
CREATE INDEX shippin_acquisition_leads_expiry
  ON shippin_acquisition_leads(expires_at, source_id)
  WHERE NOT deleted;

-- Preserve the installation's complete Owner preset; never create a partial one.
-- Fresh installs receive the permissions from the canonical owner seed.
UPDATE staff_roles SET permissions = (
  SELECT array_agg(DISTINCT p ORDER BY p)
  FROM unnest(permissions || ARRAY['acquisition.read','acquisition.sync']) AS p
), updated_at=now()
WHERE key='staff.owner';
