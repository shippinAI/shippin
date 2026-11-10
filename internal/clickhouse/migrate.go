package clickhouse

import (
	"context"
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"
)

//go:embed migrations/*.sql
var migrations embed.FS

// Migrate applies, in name order, every embedded migration the database has not recorded. ClickHouse has
// no transactions, so each file is one idempotent statement (IF NOT EXISTS) and is recorded after it runs.
func (c *Client) Migrate(ctx context.Context) ([]string, error) {
	if err := c.Exec(ctx, `CREATE TABLE IF NOT EXISTS shippin_schema_migrations
		(name String, applied_at DateTime64(3, 'UTC') DEFAULT now64(3))
		ENGINE = ReplacingMergeTree ORDER BY name`, nil); err != nil {
		return nil, err
	}
	var done []struct {
		Name string `json:"name"`
	}
	if err := c.Query(ctx, "SELECT name FROM shippin_schema_migrations FINAL", nil, &done); err != nil {
		return nil, err
	}
	have := map[string]bool{}
	for _, d := range done {
		have[d.Name] = true
	}
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return nil, err
	}
	sort.Strings(names)
	var applied []string
	for _, path := range names {
		name := strings.TrimPrefix(path, "migrations/")
		if have[name] {
			continue
		}
		sql, err := migrations.ReadFile(path)
		if err != nil {
			return applied, err
		}
		if err := c.Exec(ctx, string(sql), nil); err != nil {
			return applied, fmt.Errorf("clickhouse migration %s: %w", name, err)
		}
		if err := c.Insert(ctx, "shippin_schema_migrations", []any{map[string]string{"name": name}}); err != nil {
			return applied, fmt.Errorf("clickhouse migration %s ran but was not recorded: %w", name, err)
		}
		applied = append(applied, name)
	}
	return applied, nil
}
