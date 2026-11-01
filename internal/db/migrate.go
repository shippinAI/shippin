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

// migrateFS applies every pending file matching glob, in name order. One session holds an
// advisory lock for the whole run, so concurrent starts wait instead of racing; each file and its
// schema_migrations row commit in one transaction, so a failing file leaves no trace.
func (d *DB) migrateFS(ctx context.Context, fsys fs.FS, glob string, hold ...string) error {
	files, err := fs.Glob(fsys, glob)
	if err != nil {
		return err
	}
	sort.Strings(files)
	conn, err := d.Pool.Acquire(ctx)
	if err != nil {
		return err
	}
	defer conn.Release()
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, migrateLock); err != nil {
		return err
	}
	defer func() { _, _ = conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, migrateLock) }()
	for _, f := range files {
		if slices.Contains(hold, path.Base(f)) {
			log.Printf("migrations: holding %s and later files (config migrations.hold)", path.Base(f))
			return nil
		}
		var done bool
		var table bool
		if err := conn.QueryRow(ctx, `SELECT to_regclass('schema_migrations') IS NOT NULL`).Scan(&table); err != nil {
			return err
		}
		if table {
			if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM schema_migrations WHERE version=$1)`, f).Scan(&done); err != nil {
				return err
			}
		}
		if done {
			continue
		}
		sql, err := fs.ReadFile(fsys, f)
		if err != nil {
			return err
		}
		if err := applyOne(ctx, conn.Conn(), f, string(sql)); err != nil {
			return fmt.Errorf("migration %s: %w", f, err)
		}
	}
	return nil
}
