-- Starting catalog DATA. docs/plans.md is the source of truth. 028 upserts the same rows
-- so a database that already applied an older copy of this file converges.
-- CPU, RAM, disk, instance, and price figures in here are proposals except where docs/plans.md says otherwise.
INSERT INTO plans(id,label,description,rank,price_cents,public) VALUES
 ('free','Free','Try Shippin: container devspaces on the org pool, shared identity and Vaultix.',0,0,true),
 ('starter','Starter','A per-org VM. Cowork is not included at this size.',10,900,true),
 ('pro','Pro','Dedicated VM with room for one Cowork disk on the org data volume.',20,2900,true),
 ('business','Business','Dedicated VM, plus dedicated mail, Vaultix, and identity.',30,9900,true),
 ('founder','Founder','Internal staff and owner plan. Not sold.',5,0,false)
ON CONFLICT (id) DO NOTHING;

INSERT INTO catalog_services(id,label,description,kind,docs_url,required,config_schema,default_config) VALUES
 ('vaultix','Vaultix','Secrets manager (rebranded Infisical): projects, environments, rotation.','secrets','agent-hub/docs/devagotchi/36-vaultix.md',true,
  '{"type":"object","properties":{"placement":{"type":"string","enum":["shared","dedicated"],"title":"Placement"},"projects":{"type":"integer","minimum":0,"title":"Projects"},"environments":{"type":"array","items":{"type":"string"},"title":"Environments"},"rotation_days":{"type":"integer","minimum":1,"title":"Rotation days"}}}',
  '{"placement":"shared","projects":1,"environments":["dev","prod"],"rotation_days":90}'),
 ('identity','Identity','Nomen (Zitadel fork) placement for the org.','identity','',true,
  '{"type":"object","properties":{"placement":{"type":"string","enum":["shared_instance","dedicated_virtual_instance","dedicated_deployment"],"title":"Placement"}}}',
  '{"placement":"shared_instance"}'),
 ('agent_hub','Agent Hub','Agent workspaces, providers and approvals.','agents','agent-hub/docs/devagotchi/35-tenant-defaults.md',false,
  '{"type":"object","properties":{"seats":{"type":"integer","minimum":0,"title":"Seats"},"concurrent_sessions":{"type":"integer","minimum":0,"title":"Concurrent sessions"},"providers":{"type":"array","items":{"type":"string"},"title":"Allowed providers"}}}',
  '{"seats":1,"concurrent_sessions":1,"providers":["openai","anthropic"]}'),
 ('chatmaton','Chatmaton','Team chat with approvals, audit and PWA.','chat','agent-hub/docs/devagotchi/24-chatmaton-spec.md',false,
  '{"type":"object","properties":{"seats":{"type":"integer","minimum":0,"title":"Seats"},"retention_days":{"type":"integer","minimum":1,"title":"Retention days"}}}',
  '{"seats":5,"retention_days":30}'),
 ('devspace','Devspace','Cloud dev environment created by hostd. Container on free, per-org VM from starter up.','compute','docs/plans.md',true,
  '{"type":"object","properties":{"mode":{"type":"string","enum":["container","vm"],"title":"Mode"},"cowork":{"type":"boolean","title":"Cowork room"},"workspaces":{"type":"integer","minimum":0,"title":"Workspaces"},"cpu":{"type":"integer","minimum":1,"title":"Workspace vCPU"},"memory_gib":{"type":"integer","minimum":1,"title":"Workspace memory GiB"},"home_gib":{"type":"integer","minimum":5,"title":"Home GiB"},"quota_gib":{"type":"integer","minimum":1,"title":"Org quota GiB"},"root_gib":{"type":"integer","minimum":0,"title":"Root GiB"},"data_gib":{"type":"integer","minimum":0,"title":"Data GiB"},"vm_cpu":{"type":"integer","minimum":0,"title":"VM vCPU"},"vm_memory_gib":{"type":"integer","minimum":0,"title":"VM memory GiB"}}}',
  '{"mode":"container","cowork":false,"workspaces":2,"cpu":2,"memory_gib":4,"home_gib":20,"quota_gib":40,"root_gib":20}'),
 ('mail','Mail','Hosted mailboxes. Limits match shippin-mail PR 1; business is a dedicated deployment.','mail','',false,
  '{"type":"object","properties":{"placement":{"type":"string","enum":["shared","dedicated"],"title":"Placement"},"mailboxes":{"type":"integer","minimum":0,"title":"Mailboxes"},"domains":{"type":"integer","minimum":0,"title":"Domains"},"quota_gib":{"type":"integer","minimum":0,"title":"Quota GiB"},"per_day":{"type":"integer","minimum":0,"title":"Messages per day"}}}',
  '{"placement":"shared","mailboxes":2,"domains":1,"quota_gib":1,"per_day":50}'),
 ('dns','DNS','Managed zones on PowerDNS.','dns','',false,
  '{"type":"object","properties":{"zones":{"type":"integer","minimum":0,"title":"Zones"}}}',
  '{"zones":0}')
ON CONFLICT (id) DO NOTHING;

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
ON CONFLICT DO NOTHING;

INSERT INTO tenant_defaults(layer,ref,key,value,lock,bounds,note) VALUES
 ('system','','policy.sandbox','"required"','locked',NULL,'Safety rule: agents always run sandboxed.'),
 ('system','','access.egress','["github.com","registry.npmjs.org","!*.onion"]','mergeable',NULL,'Egress allowlist; entries starting with ! are denies that children cannot remove.'),
 ('system','','workspace.idle_shutdown_min','60','bounded','{"min":15,"max":480}','Idle minutes before a devspace stops.'),
 ('system','','providers.router','"direct"','overridable',NULL,'Model router mode (direct or litellm).'),
 ('system','','secrets.rotation_days','90','overridable',NULL,'Vaultix default rotation.'),
 ('plan','free','workspace.idle_shutdown_min','30','bounded','{"min":15,"max":60}','Free tier stops sooner.'),
 ('plan','starter','access.egress','["pypi.org","ghcr.io"]','mergeable',NULL,'Starter adds package registries.'),
 ('plan','business','workspace.idle_shutdown_min','240','bounded','{"min":15,"max":1440}','Business keeps devspaces longer.')
ON CONFLICT DO NOTHING;
