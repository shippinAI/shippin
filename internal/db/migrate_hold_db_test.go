package db

import (
	"testing"
	"testing/fstest"
)

// A held migration and every later file stay pending; releasing it applies them in order.
func TestMigrateHold(t *testing.T) {
	d, ctx := testDB(t)
	fsys := fstest.MapFS{
		"h/960_a.sql": {Data: []byte(`CREATE TABLE IF NOT EXISTS hold_a(x int)`)},
		"h/961_b.sql": {Data: []byte(`CREATE TABLE IF NOT EXISTS hold_b(x int)`)},
		"h/962_c.sql": {Data: []byte(`CREATE TABLE IF NOT EXISTS hold_c(x int)`)},
	}
	t.Cleanup(func() {
		_, _ = d.Pool.Exec(ctx, `DROP TABLE IF EXISTS hold_a, hold_b, hold_c; DELETE FROM schema_migrations WHERE version LIKE 'h/96%'`)
	})
	if err := d.migrateFS(ctx, fsys, "h/*.sql", "961_b.sql"); err != nil {
		t.Fatal(err)
	}
	var a, b, c bool
	q := `SELECT to_regclass('hold_a') IS NOT NULL, to_regclass('hold_b') IS NOT NULL, to_regclass('hold_c') IS NOT NULL`
	_ = d.Pool.QueryRow(ctx, q).Scan(&a, &b, &c)
	if !a || b || c {
		t.Fatalf("hold: a=%v b=%v c=%v, want only a", a, b, c)
	}
	if err := d.migrateFS(ctx, fsys, "h/*.sql"); err != nil {
		t.Fatal(err)
	}
	_ = d.Pool.QueryRow(ctx, q).Scan(&a, &b, &c)
	if !a || !b || !c {
		t.Fatal("released files not applied")
	}
}
