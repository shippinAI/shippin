package db

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"path"
	"slices"
	"sort"
)

// migrateLock is the session advisory-lock key that serialises migration runs across console
// instances; any constant used for nothing else ("SHIPMIG" in ASCII).
const migrateLock int64 = 0x53_4849_504d_4947

// notxHeader marks a migration that cannot run in a transaction (e.g. CREATE INDEX
// CONCURRENTLY); it runs alone, still under the session lock, and must be idempotent.
const notxHeader = "-- notx"

// Migrations are a module's own migration files (internal/ext): every *.sql file at the root of FS,
// applied in name order after the panel's. Each is recorded in schema_migrations as
// module:<Name>/<file>, so a module's numbering never meets the panel's.
type Migrations struct {
	Name string
	FS   fs.FS
}

// migrateFS applies every pending file matching glob, in name order. One session holds an
// advisory lock for the whole run, so concurrent starts wait instead of racing; each file and its
// schema_migrations row commit in one transaction, so a failing file leaves no trace.
func (d *DB) migrateFS(ctx context.Context, fsys fs.FS, glob string, hold ...string) error {
	_, err := d.migrateRun(ctx, fsys, glob, "", hold)
	return err
}

// migrateModules applies each module's pending files after the panel's, in the order given.
func (d *DB) migrateModules(ctx context.Context, ms []Migrations) error {
	seen := map[string]bool{}
	for _, m := range ms {
		if m.Name == "" || m.FS == nil || seen[m.Name] {
			return fmt.Errorf("module migrations need a unique name and files (%q)", m.Name)
		}
		seen[m.Name] = true
		if _, err := d.migrateRun(ctx, m.FS, "*.sql", "module:"+m.Name+"/", nil); err != nil {
			return fmt.Errorf("module %s: %w", m.Name, err)
		}
	}
	return nil
}

// migrateRun is migrateFS with each file recorded under prefix+file. held says it stopped at a file in
// hold, leaving it and every later file pending.
func (d *DB) migrateRun(ctx context.Context, fsys fs.FS, glob, prefix string, hold []string) (held bool, err error) {
	files, err := fs.Glob(fsys, glob)
	if err != nil {
		return false, err
	}
	sort.Strings(files)
	conn, err := d.Pool.Acquire(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLock); err != nil {
		return false, err
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrateLock) }()
	for _, f := range files {
		if slices.Contains(hold, path.Base(f)) {
			log.Printf("migrations: holding %s and later files (config migrations.hold)", path.Base(f))
			return true, nil
		}
		version := prefix + f
		var done bool
		var table bool
		if err := conn.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&table); err != nil {
			return false, err
		}
		if table {
			if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, version).Scan(&done); err != nil {
				return false, err
			}
		}
		if done {
			continue
		}
		sql, err := fs.ReadFile(fsys, f)
		if err != nil {
			return false, err
		}
		if err := applyOne(ctx, conn.Conn(), version, string(sql)); err != nil {
			return false, fmt.Errorf("migration %s: %w", version, err)
		}
	}
	return false, nil
}
