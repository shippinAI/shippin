package db

import (
	"context"
	"strings"

	"github.com/jackc/pgx/v5"
)

const recordVersion = `INSERT INTO schema_migrations(version) VALUES($1) ON CONFLICT DO NOTHING`

// applyOne runs one migration file and records it: in a single transaction normally, or as two
// steps for a "-- notx" file.
func applyOne(ctx context.Context, conn *pgx.Conn, name, sql string) error {
	if strings.HasPrefix(strings.TrimSpace(sql), notxHeader) {
		if _, err := conn.Exec(ctx, sql); err != nil {
			return err
		}
		_, err := conn.Exec(ctx, recordVersion, name)
		return err
	}
	tx, err := conn.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if _, err := tx.Exec(ctx, sql); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, recordVersion, name); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
