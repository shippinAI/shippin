package db

import (
	"context"
	"encoding/json"

	"github.com/jackc/pgx/v5"
)

// PlatformSettings returns the ops overrides of config keys (key -> JSON value).
func (d *DB) PlatformSettings(ctx context.Context) (map[string]json.RawMessage, error) {
	rows, err := d.Pool.Query(ctx, `SELECT key, value FROM platform_settings`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]json.RawMessage{}
	for rows.Next() {
		var k string
		var v []byte
		if err := rows.Scan(&k, &v); err != nil {
			return nil, err
		}
		out[k] = v
	}
	return out, rows.Err()
}

// SetPlatformSetting stores (value != nil) or clears (nil: back to the config value) one override,
// audited in the same transaction.
func (d *DB) SetPlatformSetting(ctx context.Context, key string, value json.RawMessage, by string, es ...AuditEntry) error {
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		if value == nil {
			_, err := tx.Exec(ctx, `DELETE FROM platform_settings WHERE key=$1`, key)
			return 1, err
		}
		tag, err := tx.Exec(ctx, `INSERT INTO platform_settings(key,value,updated_by) VALUES($1,$2,$3)
		  ON CONFLICT (key) DO UPDATE SET value=$2, updated_by=$3, updated_at=now()`, key, []byte(value), by)
		return tag.RowsAffected(), err
	})
}
