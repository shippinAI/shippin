-- Link the existing registry to identity and infrastructure ownership. Preserve
-- customer IDs and memberships; never reclassify the owner's customer org.
ALTER TABLE orgs ADD COLUMN identity_org_id uuid UNIQUE REFERENCES identity_orgs(id);
ALTER TABLE orgs ADD COLUMN tenant_id uuid UNIQUE REFERENCES tenants(id);
DO $$
DECLARE system_id uuid; internal_id text; directory_id uuid; internal_name text;
BEGIN
  SELECT id INTO STRICT system_id FROM tenants WHERE kind='system';
  SELECT id, name INTO internal_id, internal_name FROM orgs WHERE type='operations';
  IF internal_id IS NULL THEN
    internal_id := 'org_' || upper(substr(replace(gen_random_uuid()::text,'-',''),1,26));
    SELECT name INTO internal_name FROM tenants WHERE id=system_id;
    INSERT INTO orgs(id,slug,type,name)
      VALUES(internal_id,'operations-' || lower(right(internal_id,6)),'operations',internal_name);
  END IF;
  directory_id := gen_random_uuid();
  INSERT INTO identity_orgs(id,name) VALUES(directory_id,internal_name);
  UPDATE orgs SET identity_org_id=directory_id,tenant_id=system_id WHERE id=internal_id;
  INSERT INTO identity_members(org_id,user_id,role)
    SELECT directory_id,u.id,CASE WHEN u.role='owner' THEN 'owner' ELSE 'member' END
    FROM identity_users u
    WHERE u.role='owner' OR EXISTS (SELECT 1 FROM local_staff_roles s WHERE s.user_id=u.id AND cardinality(s.roles)>0)
    ON CONFLICT DO NOTHING;
END $$;
UPDATE orgs o SET tenant_id=t.id FROM tenants t
  WHERE o.type='customer' AND o.nomen_org_id=t.idp_org_id AND t.kind='customer';
UPDATE orgs o SET identity_org_id=i.id FROM identity_orgs i
  WHERE o.type='customer' AND o.nomen_org_id=i.id::text AND o.identity_org_id IS NULL;
