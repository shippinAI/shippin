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
	return OpenWith(ctx, url, hold)
}

// OpenWith is OpenHolding, then applies each module's own migrations (internal/ext) after the panel's.
// While a panel migration is held, no module's run: they build on the panel's tables.
func OpenWith(ctx context.Context, url string, hold []string, modules ...Migrations) (*DB, error) {
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		return nil, err
	}
	d := &DB{Pool: pool}
	held, err := d.migrateRun(ctx, migrations, "migrations/*.sql", "", hold)
	if err != nil || held || len(modules) == 0 {
		return d, err
	}
	return d, d.migrateModules(ctx, modules)
}

func (d *DB) migrate(ctx context.Context) error {
	return d.migrateFS(ctx, migrations, "migrations/*.sql")
}
