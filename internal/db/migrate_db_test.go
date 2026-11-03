package db

import (
	"context"
	"io/fs"
	"os"
	"sync"
	"testing"
	"testing/fstest"
)

// Two consoles starting at once on an empty schema: both succeed, every file applied once.
func TestMigrateConcurrentStarts(t *testing.T) {
	d, ctx := testDB(t)
	if _, err := d.Pool.Exec(ctx, `DROP SCHEMA public CASCADE; CREATE SCHEMA public`); err != nil {
		t.Fatal(err)
	}
	url := os.Getenv("SHIPPIN_TEST_DATABASE_URL")
	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i := range errs {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			x, err := Open(context.Background(), url)
			if err == nil {
				x.Pool.Close()
			}
			errs[i] = err
		}(i)
	}
	wg.Wait()
	for i, err := range errs {
		if err != nil {
			t.Fatalf("start %d: %v", i, err)
		}
	}
	files, _ := fs.Glob(migrations, "migrations/*.sql")
	var n int
	_ = d.Pool.QueryRow(ctx, `SELECT count(*) FROM schema_migrations`).Scan(&n)
	if n != len(files) {
		t.Fatalf("%d versions recorded for %d files", n, len(files))
	}
}

// A failing file rolls back with its version row; files before it stay applied; notx runs.
func TestMigrateFailingFileLeavesNoTrace(t *testing.T) {
	d, ctx := testDB(t)
	m := fstest.MapFS{
		"x/900_ok.sql":   {Data: []byte(`CREATE TABLE mig_ok (id int);`)},
		"x/901_bad.sql":  {Data: []byte(`CREATE TABLE mig_bad (id int); SELECT 1/0;`)},
		"x/902_next.sql": {Data: []byte(`CREATE TABLE mig_next (id int);`)},
	}
	if err := d.migrateFS(ctx, m, "x/*.sql"); err == nil {
		t.Fatal("expected the failing migration to error")
	}
	var ok, bad, next, v901 bool
	_ = d.Pool.QueryRow(ctx, `SELECT to_regclass('mig_ok') IS NOT NULL, to_regclass('mig_bad') IS NOT NULL,
	  to_regclass('mig_next') IS NOT NULL, EXISTS (SELECT 1 FROM schema_migrations WHERE version='x/901_bad.sql')`).Scan(&ok, &bad, &next, &v901)
	if !ok || bad || next || v901 {
		t.Fatalf("ok=%v bad=%v next=%v v901=%v", ok, bad, next, v901)
	}
	m["x/901_bad.sql"] = &fstest.MapFile{Data: []byte("-- notx\nCREATE TABLE IF NOT EXISTS mig_bad (id int);")}
	if err := d.migrateFS(ctx, m, "x/*.sql"); err != nil {
		t.Fatalf("rerun after fix: %v", err)
	}
	if err := d.migrateFS(ctx, m, "x/*.sql"); err != nil {
		t.Fatalf("idempotent rerun: %v", err)
	}
}
