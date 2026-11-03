package db

import (
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

const appRole = "shippin_test_app"

// asApp hardens the test schema for a stand-in app role that owns everything else (like the
// console role on a real install) and returns a DB that logs in as that role (not SET ROLE, which a
// superuser session could undo). pre runs as the superuser before hardening.
func asApp(t *testing.T, pre ...func(*DB)) (*DB, *DB) {
	t.Helper()
	d, ctx := testDB(t)
	for _, f := range pre {
		f(d)
	}
	_, err := d.Pool.Exec(ctx, `DO $$ BEGIN
		IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname='`+appRole+`') THEN CREATE ROLE `+appRole+`; END IF; END $$;
		ALTER ROLE `+appRole+` LOGIN NOSUPERUSER;
		GRANT USAGE, CREATE ON SCHEMA public TO `+appRole+`;
		DO $$ DECLARE r record; BEGIN
		  FOR r IN SELECT tablename FROM pg_tables WHERE schemaname='public' LOOP
		    EXECUTE format('ALTER TABLE %I OWNER TO `+appRole+`', r.tablename); END LOOP; END $$;`)
	if err != nil {
		t.Fatal(err)
	}
	for i := 0; i < 2; i++ { // idempotent
		if err := HardenAudit(ctx, d, appRole); err != nil {
			t.Fatalf("harden run %d: %v", i+1, err)
		}
	}
	cfg := d.Pool.Config().Copy()
	cfg.AfterConnect = nil
	cfg.ConnConfig.User, cfg.ConnConfig.Password = appRole, "" // the test cluster trusts local sockets
	p, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(p.Close)
	return d, &DB{Pool: p}
}

func TestAppRoleCanOnlyAppendAudit(t *testing.T) {
	admin, app := asApp(t)
	ctx := t.Context()
	var who string
	if err := app.Pool.QueryRow(ctx, `SELECT current_user`).Scan(&who); err != nil || who != appRole {
		t.Fatalf("connections must run as %s, got %q (%v)", appRole, who, err)
	}
	// The app still works: grant lifecycle with audit entries, expiry, reads.
	tn, ws := seed(t, app, ctx)
	g := reqGrant(tn, ws.ID, "admin1", "pending", 60)
	if err := app.CreateGrant(ctx, g, AuditEntry{TenantID: tn.ID, GrantID: g.ID, Actor: "admin1", Action: "access.request"}); err != nil {
		t.Fatal("create grant as app:", err)
	}
	for _, op := range []string{"approve", "revoke"} {
		if ok, err := app.TransitionGrant(ctx, g.ID, op, "owner", "Owner", AuditEntry{TenantID: tn.ID, Actor: "owner", Action: "access." + op}); !ok || err != nil {
			t.Fatalf("%s as app: %v %v", op, ok, err)
		}
	}
	g2 := reqGrant(tn, "", "admin2", "approved", 1)
	if err := app.CreateGrant(ctx, g2); err != nil {
		t.Fatal(err)
	}
	time.Sleep(1100 * time.Millisecond)
	exp := func(g Grant) AuditEntry {
		return AuditEntry{TenantID: g.TenantID, GrantID: g.ID, Actor: "shippin", Action: "access.expire"}
	}
	if gs, err := app.ExpireGrants(ctx, exp); err != nil || len(gs) != 1 {
		t.Fatalf("expire as app: %d %v", len(gs), err)
	}
	if es, err := app.AllAudit(ctx, "", 10); err != nil || len(es) != 4 {
		t.Fatalf("audit read as app: %d %v", len(es), err)
	}
	for _, q := range []string{
		`UPDATE audit_log SET detail='x'`, `DELETE FROM audit_log`, `TRUNCATE audit_log`,
		`DELETE FROM access_grants`, `TRUNCATE access_grants CASCADE`, `UPDATE access_grants SET reason='x'`,
		`UPDATE access_grants SET tenant_id=tenant_id`,
		`ALTER TABLE audit_log DISABLE TRIGGER audit_log_append_only`, `ALTER TABLE audit_log DISABLE TRIGGER ALL`,
		`ALTER TABLE access_grants DISABLE TRIGGER access_grants_no_delete`,
		`DROP TRIGGER audit_log_append_only ON audit_log`, `DROP TABLE audit_log`, `DROP TABLE access_grants CASCADE`,
		`ALTER TABLE audit_log ADD COLUMN x int`, `ALTER TABLE audit_log OWNER TO ` + appRole,
		`CREATE OR REPLACE FUNCTION shippin_append_only() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RETURN NULL; END $$`,
		`SET session_replication_role = replica`, `SET ROLE shippin_audit_owner`, `GRANT shippin_audit_owner TO ` + appRole,
	} {
		if _, err := app.Pool.Exec(ctx, q); err == nil {
			t.Errorf("as app: %q succeeded, want refusal", q)
		} else if !strings.Contains(err.Error(), "denied") && !strings.Contains(err.Error(), "must be") &&
			!strings.Contains(err.Error(), "append-only") {
			t.Logf("as app: %q refused: %v", q, err)
		}
	}
	var n int
	if err := admin.Pool.QueryRow(ctx, `SELECT count(*) FROM audit_log`).Scan(&n); err != nil || n != 4 {
		t.Fatalf("entries must survive: %d %v", n, err)
	}
	var login bool
	_ = admin.Pool.QueryRow(ctx, `SELECT rolcanlogin FROM pg_roles WHERE rolname='shippin_audit_owner'`).Scan(&login)
	if login {
		t.Fatal("shippin_audit_owner must be NOLOGIN")
	}
}
