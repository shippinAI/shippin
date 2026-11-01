-- Granular customer-catalog permissions (cloud-IAM style, deny by default). Only the Owner preset
-- holds them; anyone else needs a direct, audited grant (Team > Direct grants). The routes stopped
-- using tenants.read / tenants.lifecycle / billing.write; billing.* is reserved again.
CREATE TABLE IF NOT EXISTS staff_perm_grants (
  subject     text NOT NULL CHECK (length(subject) BETWEEN 1 AND 200),
  permission  text NOT NULL CHECK (permission ~ '^[a-z][a-z_]*(\.[a-z][a-z_]*){1,2}$'),
  reason      text NOT NULL DEFAULT '',
  granted_by  text NOT NULL,
  granted_at  timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (subject, permission)
);

-- The Owner preset row (if this database already has one) gains the new permissions; a fresh
-- database gets them from the preset seed. No other role row changes.
UPDATE staff_roles SET permissions = (
    SELECT array_agg(DISTINCT p ORDER BY p) FROM unnest(permissions || ARRAY[
      'catalog.plans.read','catalog.prices.read','catalog.plans.write','config.defaults.write',
      'customers.read','customers.contacts.read','customers.notes.read','customers.write',
      'customers.services.write','config.effective.read','provision.intent.create']) p),
  updated_at = now()
WHERE key = 'staff.owner';
