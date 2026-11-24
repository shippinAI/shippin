package hostd

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log"
	"net/http"
	"strings"
)

// StarterDBConfig: every customer gets a starter Postgres database on the shared platform
// cluster at signup (the operator's ruling), with prefixed tables for platform services acting for
// that customer: secrets_ (Vaultix references, never values), idp_ (Nomen), dns_ (Shippin DNS).
//
// Safety: hostd runs the SQL inside the pg instance through `incus exec` (peer auth, no
// password on the wire) and drops at once to the provisioner role (SET ROLE), which is NOLOGIN
// with only CREATEDB and CREATEROLE. The console's own role is never involved. Each tenant gets
// its own LOGIN role owning only its database; PUBLIC loses CONNECT. The tenant role password is
// generated here and kept in the sops store, never printed or returned.
type StarterDBConfig struct {
	Enabled     bool   `json:"enabled"`
	Instance    string `json:"instance"`    // pg instance in the platform project, e.g. pg
	Provisioner string `json:"provisioner"` // NOLOGIN CREATEDB CREATEROLE role
	Addr        string `json:"addr"`        // host:port platform services use, e.g. 203.0.113.20:5432
	KeyStore    string `json:"key_store"`   // sops file for tenant role passwords
	ConnLimit   int    `json:"conn_limit"`
}

// StarterDB is what callers may know: no password.
type StarterDB struct {
	Database string   `json:"database"`
	Role     string   `json:"role"`
	Addr     string   `json:"addr"`
	Tables   []string `json:"tables"`
}

var starterTables = []string{"secrets_refs", "idp_users", "idp_groups", "dns_zones", "dns_records"}

func starterNames(t string) (string, string) {
	h := strings.ReplaceAll(t, "-", "")
	return "tdb_" + h, "tn_" + h
}

// EnsureStarterDB creates (idempotently) the tenant's starter database, role and tables.
func (s *Server) EnsureStarterDB(ctx context.Context, t string) (*StarterDB, error) {
	c := s.cfg.StarterDB
	if !c.Enabled {
		return nil, fmt.Errorf("starter database is not enabled")
	}
	if err := ValidUUID(t); err != nil {
		return nil, err
	}
	db, role := starterNames(t)
	pw := ""
	if out, _ := s.psql(ctx, "postgres", fmt.Sprintf("SELECT 1 FROM pg_roles WHERE rolname='%s';", role)); strings.TrimSpace(out) != "1" {
		b := make([]byte, 24)
		if _, err := rand.Read(b); err != nil {
			return nil, err
		}
		pw = hex.EncodeToString(b)
		if _, err := run(ctx, `"`+pw+`"`, "sops", "set", "--value-stdin", c.KeyStore, fmt.Sprintf(`["%s"]`, role)); err != nil {
			return nil, fmt.Errorf("store starter db password: %w", err)
		}
	}
	if _, err := s.psql(ctx, "postgres", starterClusterSQL(c, db, role, pw)); err != nil {
		return nil, fmt.Errorf("starter db: %w", err)
	}
	if _, err := s.psql(ctx, db, starterTablesSQL(role)); err != nil {
		return nil, fmt.Errorf("starter tables: %w", err)
	}
	return &StarterDB{Database: db, Role: role, Addr: c.Addr, Tables: starterTables}, nil
}

// psql runs SQL (stdin) in the pg instance as the provisioner role on database db.
func (s *Server) psql(ctx context.Context, db, sql string) (string, error) {
	c := s.cfg.StarterDB
	in := fmt.Sprintf("SET ROLE %s;\n%s", c.Provisioner, sql)
	return run(ctx, in, "incus", "exec", "--project="+s.cfg.Platform, c.Instance, "--", "su", "postgres", "-c",
		"psql -X -q -t -A -v ON_ERROR_STOP=1 -d "+db)
}

func starterClusterSQL(c StarterDBConfig, db, role, pw string) string {
	var b strings.Builder
	b.WriteString("SET createrole_self_grant = 'set, inherit';\n")
	if pw != "" {
		fmt.Fprintf(&b, "CREATE ROLE %s LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOREPLICATION CONNECTION LIMIT %d PASSWORD '%s';\n", role, c.ConnLimit, pw)
	}
	fmt.Fprintf(&b, "SELECT 'CREATE DATABASE %s OWNER %s' WHERE NOT EXISTS (SELECT 1 FROM pg_database WHERE datname='%s')\\gexec\n", db, role, db)
	fmt.Fprintf(&b, "REVOKE ALL ON DATABASE %s FROM PUBLIC;\nGRANT CONNECT, TEMPORARY ON DATABASE %s TO %s;\n", db, db, role)
	return b.String()
}

func starterTablesSQL(role string) string {
	return fmt.Sprintf(`SET ROLE %[1]s;
REVOKE ALL ON SCHEMA public FROM PUBLIC;
CREATE TABLE IF NOT EXISTS secrets_refs (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL UNIQUE,
  vaultix_ref text NOT NULL CHECK (vaultix_ref LIKE 'vaultix://%%'), environment text NOT NULL DEFAULT 'dev',
  created_at timestamptz NOT NULL DEFAULT now(), updated_at timestamptz NOT NULL DEFAULT now());
COMMENT ON TABLE secrets_refs IS 'Vaultix references only; secret values never live here';
CREATE TABLE IF NOT EXISTS idp_users (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), nomen_sub text NOT NULL UNIQUE,
  email text NOT NULL, display_name text, created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS idp_groups (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), name text NOT NULL UNIQUE,
  members text[] NOT NULL DEFAULT '{}', created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS dns_zones (id uuid PRIMARY KEY DEFAULT gen_random_uuid(), zone text NOT NULL UNIQUE,
  created_at timestamptz NOT NULL DEFAULT now());
CREATE TABLE IF NOT EXISTS dns_records (id uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  zone_id uuid NOT NULL REFERENCES dns_zones(id) ON DELETE CASCADE, name text NOT NULL,
  type text NOT NULL CHECK (type IN ('A','AAAA','CNAME','TXT','MX','SRV','CAA','NS')), content text NOT NULL,
  ttl int NOT NULL DEFAULT 300 CHECK (ttl BETWEEN 30 AND 86400), UNIQUE (zone_id, name, type, content));
SELECT string_agg(tablename, ',' ORDER BY tablename) FROM pg_tables WHERE schemaname='public';
`, role)
}

// starterDB (POST) ensures the tenant's starter database (backfill for tenants that predate it).
func (s *Server) starterDB(w http.ResponseWriter, r *http.Request) {
	defer lockWorkspace("starter-db-" + r.PathValue("t"))()
	res, err := s.EnsureStarterDB(r.Context(), r.PathValue("t"))
	if err != nil {
		log.Printf("starter db %s: %v", r.PathValue("t"), err)
		fail(w, http.StatusConflict, "starter database could not be provisioned")
		return
	}
	reply(w, res)
}
