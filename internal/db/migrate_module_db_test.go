package db

import (
	"io/fs"
	"os"
	"path"
	"sort"
	"testing"
	"testing/fstest"
)

func tableExists(t *testing.T, d *DB, name string) bool {
	t.Helper()
	var ok bool
	if err := d.Pool.QueryRow(t.Context(), `SELECT to_regclass($1) IS NOT NULL`, name).Scan(&ok); err != nil {
		t.Fatal(err)
	}
	return ok
}

// A module's migrations run after the panel's, once, recorded under the module's name, so its file
// names never meet the panel's; a failing file leaves no trace.
func TestModuleMigrationsRunAfterThePanels(t *testing.T) {
	d, ctx := testDB(t)
	probe := fstest.MapFS{
		"001_probe.sql": {Data: []byte(`CREATE TABLE shippin_probe_widgets (id int PRIMARY KEY, tenant_id uuid REFERENCES tenants(id))`)},
	}
	for i := 0; i < 2; i++ {
		if err := d.migrateModules(ctx, []Migrations{{Name: "probe", FS: probe}}); err != nil {
			t.Fatal(err)
		}
	}
	if !tableExists(t, d, "shippin_probe_widgets") {
		t.Fatal("the module's table is missing")
	}
	var n int
	if err := d.Pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations WHERE version='module:probe/001_probe.sql'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("recorded %d times: %v", n, err)
	}

	broken := fstest.MapFS{
		"001_probe.sql": probe["001_probe.sql"],
		"002_bad.sql":   {Data: []byte(`CREATE TABLE shippin_probe_half (id int); SELECT no_such_function()`)},
	}
	if err := d.migrateModules(ctx, []Migrations{{Name: "probe", FS: broken}}); err == nil {
		t.Fatal("a failing module migration was accepted")
	}
	if tableExists(t, d, "shippin_probe_half") {
		t.Error("a failing module migration left a table behind")
	}
	for _, bad := range [][]Migrations{{{FS: probe}}, {{Name: "probe"}}, {{Name: "a", FS: probe}, {Name: "a", FS: probe}}} {
		if err := d.migrateModules(ctx, bad); err == nil {
			t.Errorf("accepted %+v", bad)
		}
	}
}

// While a panel migration is held, no module migration runs: they build on the panel's tables.
func TestModuleMigrationsWaitForAHeldPanelMigration(t *testing.T) {
	d, ctx := testDB(t)
	files, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil || len(files) == 0 {
		t.Fatal(files, err)
	}
	sort.Strings(files)
	last := path.Base(files[len(files)-1])
	probe := fstest.MapFS{"001_probe.sql": {Data: []byte(`CREATE TABLE shippin_probe_held (id int)`)}}
	held, err := OpenWith(ctx, os.Getenv("SHIPPIN_TEST_DATABASE_URL"), []string{last}, Migrations{Name: "probe", FS: probe})
	if err != nil {
		t.Fatal(err)
	}
	held.Pool.Close()
	if tableExists(t, d, "shippin_probe_held") {
		t.Fatal("a module migration ran while the panel's were held")
	}
	open, err := OpenWith(ctx, os.Getenv("SHIPPIN_TEST_DATABASE_URL"), nil, Migrations{Name: "probe", FS: probe})
	if err != nil {
		t.Fatal(err)
	}
	open.Pool.Close()
	if !tableExists(t, d, "shippin_probe_held") {
		t.Fatal("the module migration did not run once nothing was held")
	}
}
