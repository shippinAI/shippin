// Package db holds the console's Postgres access (db shippin).
package db

import (
	"context"
	"embed"

	"github.com/jackc/pgx/v5/pgxpool"
)

//go:embed migrations/*.sql
var migrations embed.FS

type DB struct{ Pool *pgxpool.Pool }

// Open connects with SHIPPIN_DATABASE_URL and applies pending migrations.
func Open(ctx context.Context, url string) (*DB, error) { return OpenHolding(ctx, url, nil) }

// OpenHolding is Open, but stops before the first migration named in hold (base file names):
// it and every later file stay pending, so a change ticket can release them separately.
func OpenHolding(ctx context.Context, url string, hold []string) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	d := &DB{Pool: pool}
	return d, d.migrateFS(ctx, migrations, "migrations/*.sql", hold...)
}

func (d *DB) migrate(ctx context.Context) error {
	return d.migrateFS(ctx, migrations, "migrations/*.sql")
}
