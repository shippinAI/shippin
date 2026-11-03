-- Customer catalog: plans (tiers), service catalog, plan->service matrix, layered defaults,
-- tenant service overrides and provision intents. 008 is reserved for the endpoint reconciler.
CREATE TABLE IF NOT EXISTS plans (
  id          text PRIMARY KEY,
  label       text NOT NULL,
  description text NOT NULL DEFAULT '',
  rank        int  NOT NULL DEFAULT 0,
  price_cents int  NOT NULL DEFAULT 0,
  public      boolean NOT NULL DEFAULT true,
  active      boolean NOT NULL DEFAULT true,
  updated_at  timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS catalog_services (
  id             text PRIMARY KEY,
  label          text NOT NULL,
  description    text NOT NULL DEFAULT '',
  kind           text NOT NULL DEFAULT 'app',
  docs_url       text NOT NULL DEFAULT '',
  config_schema  jsonb NOT NULL DEFAULT '{}',
  default_config jsonb NOT NULL DEFAULT '{}',
  required       boolean NOT NULL DEFAULT false,
  active         boolean NOT NULL DEFAULT true,
  updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE TABLE IF NOT EXISTS plan_services (
  plan_id    text NOT NULL REFERENCES plans(id) ON DELETE CASCADE,
  service_id text NOT NULL REFERENCES catalog_services(id) ON DELETE CASCADE,
  enabled    boolean NOT NULL DEFAULT true,
  lock       text NOT NULL DEFAULT 'overridable' CHECK (lock IN ('locked','overridable','mergeable','bounded')),
  limits     jsonb NOT NULL DEFAULT '{}',
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (plan_id, service_id)
);
CREATE TABLE IF NOT EXISTS tenant_defaults (
  id         bigserial PRIMARY KEY,
  layer      text NOT NULL CHECK (layer IN ('system','plan','tenant','workspace')),
  ref        text NOT NULL DEFAULT '',
  key        text NOT NULL,
  value      jsonb NOT NULL,
  lock       text NOT NULL DEFAULT 'overridable' CHECK (lock IN ('locked','overridable','mergeable','bounded')),
  bounds     jsonb,
  note       text NOT NULL DEFAULT '',
  updated_at timestamptz NOT NULL DEFAULT now(),
  UNIQUE (layer, ref, key)
);
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS contacts  jsonb NOT NULL DEFAULT '[]';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS endpoints jsonb NOT NULL DEFAULT '[]';
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS notes     text  NOT NULL DEFAULT '';
CREATE TABLE IF NOT EXISTS tenant_services (
  tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  service_id text NOT NULL REFERENCES catalog_services(id) ON DELETE CASCADE,
  enabled    boolean,
  overrides  jsonb NOT NULL DEFAULT '{}',
  updated_at timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (tenant_id, service_id)
);
CREATE TABLE IF NOT EXISTS provision_intents (
  id         uuid PRIMARY KEY,
  tenant_id  uuid NOT NULL REFERENCES tenants(id) ON DELETE CASCADE,
  desired    jsonb NOT NULL,
  status     text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending','ticketed','applied','failed','cancelled')),
  ticket_ref text NOT NULL DEFAULT '',
  created_by text NOT NULL,
  created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS provision_intents_tenant ON provision_intents(tenant_id, created_at DESC);
