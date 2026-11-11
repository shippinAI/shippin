-- Coverage is how much help and operation an organization gets. It is sold beside its plan (capacity),
-- the way cloud providers sell support plans beside usage: business hours and tickets at the small end,
-- 24/7 response and reviews in the middle, and run-for-you operations, a named contact, training and
-- professional services at the large end. Every value is a starting default the owner edits in the panel.
-- Response targets are first-response minutes for a new case, counted within the tier's support hours.
CREATE TABLE IF NOT EXISTS coverage_tiers (
  id                 text PRIMARY KEY CHECK (id ~ '^[a-z][a-z0-9_-]{1,39}$'),
  label              text NOT NULL CHECK (length(label) BETWEEN 1 AND 80),
  description        text NOT NULL DEFAULT '',
  rank               int  NOT NULL DEFAULT 0,
  sign_in            text NOT NULL DEFAULT 'password' CHECK (sign_in IN ('password','second_factor','identity_provider')),
  support_hours      text NOT NULL DEFAULT 'business' CHECK (support_hours IN ('business','critical_always','always')),
  channels           text[] NOT NULL DEFAULT '{docs,email}' CHECK (channels <@ ARRAY['docs','community','email','chat','phone','video']),
  critical_minutes   int CHECK (critical_minutes > 0),
  high_minutes       int CHECK (high_minutes > 0),
  normal_minutes     int CHECK (normal_minutes > 0),
  uptime_target      numeric(6,3) CHECK (uptime_target > 0 AND uptime_target < 100),
  operations         text NOT NULL DEFAULT 'self' CHECK (operations IN ('self','assisted','managed')),
  named_contact      boolean NOT NULL DEFAULT false,
  reviews            text NOT NULL DEFAULT 'none' CHECK (reviews IN ('none','quarterly','monthly')),
  training_hours     int  NOT NULL DEFAULT 0 CHECK (training_hours BETWEEN 0 AND 10000),
  consulting         text NOT NULL DEFAULT 'quoted' CHECK (consulting IN ('none','quoted','included')),
  custom_deployments text NOT NULL DEFAULT 'quoted' CHECK (custom_deployments IN ('none','quoted','included')),
  remote_operations  text NOT NULL DEFAULT 'none' CHECK (remote_operations IN ('none','quoted','included')),
  team_training      text NOT NULL DEFAULT 'quoted' CHECK (team_training IN ('none','quoted','included')),
  price_cents        int  NOT NULL DEFAULT 0 CHECK (price_cents >= 0),
  spend_percent      numeric(5,2) NOT NULL DEFAULT 0 CHECK (spend_percent BETWEEN 0 AND 100),
  public             boolean NOT NULL DEFAULT true,
  active             boolean NOT NULL DEFAULT true,
  updated_at         timestamptz NOT NULL DEFAULT now()
);

INSERT INTO coverage_tiers (id, label, description, rank, sign_in, support_hours, channels, critical_minutes, high_minutes, normal_minutes,
  uptime_target, operations, named_contact, reviews, training_hours, consulting, custom_deployments, remote_operations, team_training) VALUES
 ('foundational', 'Foundational', 'For small teams that run their own workloads. Documentation, community and email cases in business hours.',
  10, 'password', 'business', '{docs,community,email}', 240, 480, 960, NULL, 'self', false, 'none', 0, 'quoted', 'quoted', 'none', 'quoted'),
 ('advanced', 'Advanced', 'For production workloads. Round-the-clock response to outages, an uptime target, guidance on changes and quarterly reviews.',
  20, 'second_factor', 'critical_always', '{docs,community,email,chat,phone}', 60, 240, 480, 99.9, 'assisted', false, 'quarterly', 4, 'quoted', 'quoted', 'quoted', 'included'),
 ('managed', 'Managed', 'For larger organizations. We operate the platform for you remotely, with a named contact, monthly reviews, training for your team, and consulting and custom deployments.',
  30, 'identity_provider', 'always', '{docs,community,email,chat,phone,video}', 15, 60, 240, 99.95, 'managed', true, 'monthly', 16, 'included', 'quoted', 'included', 'included')
ON CONFLICT (id) DO NOTHING;

-- Each plan includes a coverage tier; an organization can be moved to another one.
ALTER TABLE plans ADD COLUMN IF NOT EXISTS coverage text REFERENCES coverage_tiers(id) ON UPDATE CASCADE;
UPDATE plans SET coverage = CASE WHEN id = 'business' THEN 'advanced' ELSE 'foundational' END
WHERE coverage IS NULL AND EXISTS (SELECT 1 FROM coverage_tiers WHERE id = 'foundational');
ALTER TABLE tenants ADD COLUMN IF NOT EXISTS coverage text REFERENCES coverage_tiers(id) ON UPDATE CASCADE ON DELETE SET NULL;

-- Migration 069 made the three tiers plans. They are coverage now: organizations on them keep that
-- coverage and move to the lowest-ranked other active plan, and the plan rows go.
UPDATE tenants SET coverage = plan,
  plan = coalesce((SELECT id FROM plans WHERE active AND id NOT IN ('foundational','advanced','managed') ORDER BY rank, id LIMIT 1), plan)
WHERE plan IN ('foundational','advanced','managed');
DELETE FROM plans p WHERE p.id IN ('foundational','advanced','managed') AND NOT EXISTS (SELECT 1 FROM tenants t WHERE t.plan = p.id);
ALTER TABLE plans DROP COLUMN IF EXISTS sign_in;
