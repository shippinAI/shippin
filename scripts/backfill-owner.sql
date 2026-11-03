-- backfill-owner.sql: link one existing tenant to its owner's account by hand, for tenants whose
-- owner has not signed in since P0-3 (sign-in backfills automatically via syncAccount).
-- Usage (on the pg instance): psql -d shippin -v ON_ERROR_STOP=1 -v tenant=<tenant uuid> \
--   -v sub=<Nomen user id of the org owner> -v email=<owner email> -f backfill-owner.sql
BEGIN;
INSERT INTO accounts(id, idp_sub, email) VALUES (gen_random_uuid(), :'sub', :'email')
  ON CONFLICT (idp_sub) DO NOTHING;
INSERT INTO tenant_owners(tenant_id, account_id)
  SELECT :'tenant'::uuid, id FROM accounts WHERE idp_sub = :'sub'
  ON CONFLICT DO NOTHING;
SELECT t.id, t.name, a.idp_sub, a.email FROM tenant_owners o
  JOIN tenants t ON t.id = o.tenant_id JOIN accounts a ON a.id = o.account_id
  WHERE o.tenant_id = :'tenant'::uuid;
COMMIT;
