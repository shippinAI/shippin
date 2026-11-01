-- Demo customers for the local catalog demo only (never applied by the console).
INSERT INTO tenants(id, idp_org_id, name, plan, status, contacts, endpoints, notes) VALUES
 ('01a14149-0000-7000-8000-000000000001', 'demo-org-dev', 'dev', 'team', 'active',
  '[{"name":"the operator H.","email":"operator@shippin.test","role":"owner"}]',
  '[{"kind":"lan","host":"console.shippin.cloud","note":"LAN / tailnet route"}]', 'the operator''s own devspace tenant (t-01a14149).'),
 ('01a14149-0000-7000-8000-000000000002', 'unlinked:01a14149-0000-7000-8000-000000000002', 'Acme Robotics', 'pro', 'draft',
  '[{"name":"Ann Lee","email":"ann@acme.test","role":"billing"}]', '[]', 'Trial, waiting on contract.')
ON CONFLICT (id) DO NOTHING;
INSERT INTO tenant_services(tenant_id, service_id, enabled, overrides) VALUES
 ('01a14149-0000-7000-8000-000000000001', 'chatmaton', NULL, '{"seats":40}'),
 ('01a14149-0000-7000-8000-000000000002', 'agent_hub', NULL, '{"seats":1,"concurrent_sessions":2}')
ON CONFLICT DO NOTHING;
INSERT INTO tenant_defaults(layer, ref, key, value, lock, note) VALUES
 ('tenant', '01a14149-0000-7000-8000-000000000001', 'workspace.idle_shutdown_min', '600', 'overridable', 'the operator wants long sessions.'),
 ('tenant', '01a14149-0000-7000-8000-000000000001', 'access.egress', '["crates.io"]', 'overridable', ''),
 ('tenant', '01a14149-0000-7000-8000-000000000001', 'policy.sandbox', '"off"', 'overridable', 'Tries to turn sandbox off (rejected: locked).')
ON CONFLICT DO NOTHING;
