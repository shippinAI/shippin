package hostd

import (
	"os"
	"os/exec"
	"strings"
	"testing"
)

// Runs the starter SQL against a real cluster as a superuser that drops to the provisioner,
// exactly as hostd does. Needs SHIPPIN_TEST_PG_SUPERUSER_PSQL (e.g. "psql -h 127.0.0.1 -p 55433 -U postgres").
func TestStarterSQLOnRealPostgres(t *testing.T) {
	base := os.Getenv("SHIPPIN_TEST_PG_SUPERUSER_PSQL")
	if base == "" {
		t.Skip("SHIPPIN_TEST_PG_SUPERUSER_PSQL not set")
	}
	run := func(db, sql string) string {
		cmd := exec.Command("sh", "-c", base+" -X -q -t -A -v ON_ERROR_STOP=1 -d "+db)
		cmd.Stdin = strings.NewReader("SET ROLE starter_prov_test;\n" + sql)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("%v: %s", err, out)
		}
		return string(out)
	}
	setup := exec.Command("sh", "-c", base+` -X -q -d postgres -c "DROP DATABASE IF EXISTS tdb_00000000000040008000000000000001" -c "DROP ROLE IF EXISTS tn_00000000000040008000000000000001" -c "DROP ROLE IF EXISTS starter_prov_test" -c "CREATE ROLE starter_prov_test NOLOGIN CREATEDB CREATEROLE"`)
	if out, err := setup.CombinedOutput(); err != nil {
		t.Fatalf("%v %s", err, out)
	}
	c := StarterDBConfig{ConnLimit: 5}
	db, role := starterNames("00000000-0000-4000-8000-000000000001")
	run("postgres", starterClusterSQL(c, db, role, "pw-test"))
	run("postgres", starterClusterSQL(c, db, role, "")) // idempotent second run
	out := run(db, starterTablesSQL(role))
	run(db, starterTablesSQL(role))
	if !strings.Contains(out, "dns_records,dns_zones,idp_groups,idp_users,secrets_refs") {
		t.Fatalf("tables: %q", out)
	}
	check := exec.Command("sh", "-c", base+` -X -t -A -d postgres -c "SELECT rolsuper OR rolcreatedb OR rolcreaterole, has_database_privilege('public','`+db+`','CONNECT') FROM pg_roles WHERE rolname='`+role+`'"`)
	o, _ := check.CombinedOutput()
	if strings.TrimSpace(string(o)) != "f|f" {
		t.Fatalf("tenant role too strong or PUBLIC can connect: %q", o)
	}
}
