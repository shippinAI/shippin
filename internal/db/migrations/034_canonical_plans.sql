-- Bring catalog rows in line with docs/plans.md. Safe to re-run: plans and plan_services upsert,
-- Same statements as cursor/devspace-foundation-8c87's 028_canonical_plans.sql. Renumbered to 034
-- because 028_hub.sql through 033_tenant_db.sql already occupied those numbers. Do not re-add 028.
-- retired tier ids stay in the table (tenants.plan is free text) but are not sold.
UPDATE plans SET public=false, active=false, description='Retired. See docs/plans.md.'
 WHERE id IN ('micro','single','dual','b1','b2','b4','b8','custom','team','enterprise');

INSERT INTO plans(id,label,description,rank,price_cents,public,boundaries) VALUES
 ('free','Free','Try Shippin: container devspaces on the org pool, shared identity and Vaultix.',0,0,true,0),
 ('starter','Starter','A per-org VM. Cowork is not included at this size.',10,900,true,0),
 ('pro','Pro','Dedicated VM with room for one Cowork disk on the org data volume.',20,2900,true,0),
 ('business','Business','Dedicated VM, plus dedicated mail, Vaultix, and identity.',30,9900,true,1),
 ('founder','Founder','Internal staff and owner plan. Not sold.',5,0,false,1)
ON CONFLICT (id) DO UPDATE SET
  label=EXCLUDED.label,
  description=EXCLUDED.description,
  rank=EXCLUDED.rank,
  price_cents=EXCLUDED.price_cents,
  public=EXCLUDED.public,
  boundaries=EXCLUDED.boundaries,
  active=true,
  updated_at=now();

INSERT INTO catalog_services(id,label,description,kind,docs_url,required,config_schema,default_config) VALUES
 ('identity','Identity','Nomen (Zitadel fork) placement for the org.','identity','',true,
  '{"type":"object","properties":{"placement":{"type":"string","enum":["shared_instance","dedicated_virtual_instance","dedicated_deployment"],"title":"Placement"}}}',
  '{"placement":"shared_instance"}')
ON CONFLICT (id) DO UPDATE SET
  label=EXCLUDED.label, description=EXCLUDED.description, config_schema=EXCLUDED.config_schema,
  default_config=EXCLUDED.default_config, updated_at=now();

UPDATE catalog_services SET
  description='Hosted mailboxes. Limits match shippin-mail PR 1; business is a dedicated deployment.',
  config_schema='{"type":"object","properties":{"placement":{"type":"string","enum":["shared","dedicated"],"title":"Placement"},"mailboxes":{"type":"integer","minimum":0,"title":"Mailboxes"},"domains":{"type":"integer","minimum":0,"title":"Domains"},"quota_gib":{"type":"integer","minimum":0,"title":"Quota GiB"},"per_day":{"type":"integer","minimum":0,"title":"Messages per day"}}}',
  default_config='{"placement":"shared","mailboxes":2,"domains":1,"quota_gib":1,"per_day":50}',
  updated_at=now()
 WHERE id='mail';

UPDATE catalog_services SET
  description='Cloud dev environment created by hostd. Container on free, per-org VM from starter up.',
  docs_url='docs/plans.md',
  config_schema='{"type":"object","properties":{"mode":{"type":"string","enum":["container","vm"],"title":"Mode"},"cowork":{"type":"boolean","title":"Cowork room"},"workspaces":{"type":"integer","minimum":0,"title":"Workspaces"},"cpu":{"type":"integer","minimum":1,"title":"Workspace vCPU"},"memory_gib":{"type":"integer","minimum":1,"title":"Workspace memory GiB"},"home_gib":{"type":"integer","minimum":5,"title":"Home GiB"},"quota_gib":{"type":"integer","minimum":1,"title":"Org quota GiB"},"root_gib":{"type":"integer","minimum":0,"title":"Root GiB"},"data_gib":{"type":"integer","minimum":0,"title":"Data GiB"},"vm_cpu":{"type":"integer","minimum":0,"title":"VM vCPU"},"vm_memory_gib":{"type":"integer","minimum":0,"title":"VM memory GiB"}}}',
  default_config='{"mode":"container","cowork":false,"workspaces":2,"cpu":2,"memory_gib":4,"home_gib":20,"quota_gib":40,"root_gib":20}',
  updated_at=now()
 WHERE id='devspace';

INSERT INTO plan_services(plan_id,service_id,enabled,lock,limits) VALUES
 ('free','devspace',true,'bounded','{"mode":"container","cowork":false,"workspaces":2,"cpu":2,"memory_gib":4,"home_gib":20,"quota_gib":40,"root_gib":20}'),
 ('free','vaultix',true,'bounded','{"placement":"shared","projects":1}'),
 ('free','identity',true,'bounded','{"placement":"shared_instance"}'),
 ('free','mail',true,'bounded','{"placement":"shared","mailboxes":2,"domains":1,"quota_gib":1,"per_day":50}'),
 ('starter','devspace',true,'bounded','{"mode":"vm","cowork":false,"workspaces":3,"cpu":2,"memory_gib":3,"home_gib":20,"quota_gib":80,"root_gib":20,"data_gib":40,"vm_cpu":2,"vm_memory_gib":4}'),
 ('starter','vaultix',true,'bounded','{"placement":"shared","projects":3}'),
 ('starter','identity',true,'bounded','{"placement":"shared_instance"}'),
 ('starter','mail',true,'bounded','{"placement":"shared","mailboxes":10,"domains":3,"quota_gib":10,"per_day":500}'),
 ('pro','devspace',true,'bounded','{"mode":"vm","cowork":true,"workspaces":6,"cpu":4,"memory_gib":8,"home_gib":40,"quota_gib":160,"root_gib":20,"data_gib":100,"vm_cpu":4,"vm_memory_gib":16}'),
 ('pro','vaultix',true,'overridable','{"placement":"dedicated","projects":10}'),
 ('pro','identity',true,'bounded','{"placement":"dedicated_virtual_instance"}'),
 ('pro','mail',true,'bounded','{"placement":"shared","mailboxes":50,"domains":20,"quota_gib":100,"per_day":5000}'),
 ('pro','agent_hub',true,'bounded','{"seats":1,"concurrent_sessions":3}'),
 ('business','devspace',true,'bounded','{"mode":"vm","cowork":true,"workspaces":16,"cpu":8,"memory_gib":16,"home_gib":80,"quota_gib":360,"root_gib":20,"data_gib":280,"vm_cpu":8,"vm_memory_gib":32}'),
 ('business','vaultix',true,'overridable','{"placement":"dedicated"}'),
 ('business','identity',true,'bounded','{"placement":"dedicated_deployment"}'),
 ('business','mail',true,'overridable','{"placement":"dedicated"}'),
 ('business','agent_hub',true,'bounded','{"seats":10,"concurrent_sessions":10}'),
 ('business','chatmaton',true,'bounded','{"seats":25,"retention_days":365}'),
 ('business','dns',true,'bounded','{"zones":3}'),
 ('founder','devspace',true,'bounded','{"mode":"vm","cowork":true,"workspaces":10,"cpu":4,"memory_gib":8,"home_gib":50,"quota_gib":450,"root_gib":20,"data_gib":300,"vm_cpu":16,"vm_memory_gib":32}'),
 ('founder','vaultix',true,'overridable','{"placement":"dedicated"}'),
 ('founder','identity',true,'bounded','{"placement":"dedicated_deployment"}'),
 ('founder','mail',true,'overridable','{"placement":"dedicated"}'),
 ('founder','agent_hub',true,'bounded','{"seats":10,"concurrent_sessions":10}'),
 ('founder','chatmaton',true,'bounded','{"seats":25,"retention_days":365}'),
 ('founder','dns',true,'bounded','{"zones":3}')
ON CONFLICT (plan_id, service_id) DO UPDATE SET
  enabled=EXCLUDED.enabled, lock=EXCLUDED.lock, limits=EXCLUDED.limits, updated_at=now();

DELETE FROM plan_services WHERE plan_id IN ('micro','single','dual','b1','b2','b4','b8','custom','team','enterprise');
DELETE FROM tenant_defaults WHERE layer='plan' AND ref IN ('team','enterprise');

INSERT INTO tenant_defaults(layer,ref,key,value,lock,bounds,note) VALUES
 ('plan','starter','access.egress','["pypi.org","ghcr.io"]','mergeable',NULL,'Starter adds package registries.'),
 ('plan','business','workspace.idle_shutdown_min','240','bounded','{"min":15,"max":1440}','Business keeps devspaces longer.')
ON CONFLICT (layer, ref, key) DO UPDATE SET
  value=EXCLUDED.value, lock=EXCLUDED.lock, bounds=EXCLUDED.bounds, note=EXCLUDED.note, updated_at=now();
