DROP INDEX IF EXISTS hub_tasks_org_external;
ALTER TABLE hub_tasks DROP COLUMN IF EXISTS external_key;
