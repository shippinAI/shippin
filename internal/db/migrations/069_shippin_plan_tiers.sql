-- Each plan sets how its organizations' members must sign in, and an installation starts with three tiers
-- it can rename, reprice, change or remove in the panel.
ALTER TABLE plans ADD COLUMN IF NOT EXISTS sign_in text NOT NULL DEFAULT 'password'
  CHECK (sign_in IN ('password','second_factor','identity_provider'));
INSERT INTO plans(id,label,description,rank,price_cents,public,active,sign_in) VALUES
 ('foundational','Foundational','Self-service hosting on shared capacity. Members sign in with a password or better.',10,0,true,true,'password'),
 ('advanced','Advanced','More capacity and services. Members need a passkey or an identity provider.',20,0,true,true,'second_factor'),
 ('managed','Managed','Run for you, with support. Members sign in through the organization''s identity provider.',30,0,true,true,'identity_provider')
ON CONFLICT (id) DO NOTHING;
