-- Lets the factory board address an approval card by a stable key (a Gold ticket, for
-- example) instead of only by the hub's own id. Empty keys stay unconstrained.
ALTER TABLE hub_tasks ADD COLUMN IF NOT EXISTS external_key text NOT NULL DEFAULT '';
CREATE UNIQUE INDEX IF NOT EXISTS hub_tasks_org_external
  ON hub_tasks (org_id, external_key) WHERE external_key <> '';
