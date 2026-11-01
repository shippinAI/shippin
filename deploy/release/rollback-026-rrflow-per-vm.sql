-- Manual down for 026_rrflow_per_vm.sql, only for rolling the console back past provider-admin.
-- Pre-026 code needs UNIQUE(project_id, kind) and NOT NULL project_id. Shared ('vm') instances and
-- their bindings are dropped (their VM containers must be removed with hostd), 'migrating' rows go
-- and 'migrated' rows go back to 'ready' (migrate-to never deletes; their containers and pre-migrate
-- snapshots are intact until a separate retire-into). Run in one transaction, with the console
-- still on the 026 release stopped or switched back first; every provision job is canceled (not
-- deleted: the audit trail stays) and the transaction aborts unless none is left queued or running.
BEGIN;
UPDATE jobs SET status='canceled', error='rollback-026', finished_at=now(), updated_at=now()
  WHERE kind='service.provision' AND status IN ('queued','running');
DO $$ BEGIN
  IF (SELECT count(*) FROM jobs WHERE kind='service.provision' AND status IN ('queued','running')) <> 0 THEN
    RAISE EXCEPTION 'rollback-026: provision jobs still queued or running';
  END IF;
END $$;
UPDATE service_instances SET migrate_to=NULL, state='ready' WHERE state IN ('migrating','migrated');
DELETE FROM service_instances WHERE scope='vm';
DROP TABLE IF EXISTS service_namespaces;
ALTER TABLE service_bindings DROP COLUMN IF EXISTS project_id;
DROP INDEX IF EXISTS service_instances_project_kind, service_instances_vm_kind;
ALTER TABLE service_instances DROP CONSTRAINT IF EXISTS service_instances_scope_shape;
ALTER TABLE service_instances DROP CONSTRAINT IF EXISTS service_instances_state_check;
ALTER TABLE service_instances ADD CONSTRAINT service_instances_state_check
  CHECK (state IN ('pending','provisioning','ready','error','deleting'));
ALTER TABLE service_instances DROP COLUMN IF EXISTS migrate_to, DROP COLUMN IF EXISTS scope;
ALTER TABLE service_instances ALTER COLUMN project_id SET NOT NULL;
ALTER TABLE service_instances ADD CONSTRAINT service_instances_project_id_kind_key UNIQUE (project_id, kind);
DELETE FROM schema_migrations WHERE version LIKE '%026_rrflow_per_vm.sql';
COMMIT;
