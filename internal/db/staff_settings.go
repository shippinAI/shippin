package db

import (
	"context"
	"encoding/json"
	"time"

	"github.com/jackc/pgx/v5"
)

// StaffSettings reads the team settings; defaults when the row is missing.
func (d *DB) StaffSettings(ctx context.Context) (StaffSettings, error) {
	var b []byte
	err := d.Pool.QueryRow(ctx, `SELECT data FROM staff_settings WHERE id=1`).Scan(&b)
	if err == pgx.ErrNoRows {
		return DefaultStaffSettings, nil
	}
	s := DefaultStaffSettings
	if err != nil {
		return s, err
	}
	return s, json.Unmarshal(b, &s)
}

// SaveStaffSettings replaces the settings, audited in the same transaction.
func (d *DB) SaveStaffSettings(ctx context.Context, s StaffSettings, by string, es ...AuditEntry) error {
	b, _ := json.Marshal(s)
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		tag, err := tx.Exec(ctx, `INSERT INTO staff_settings(id, data, updated_by) VALUES(1, $1, $2)
			ON CONFLICT (id) DO UPDATE SET data=$1, updated_by=$2, updated_at=now()`, b, by)
		return tag.RowsAffected(), err
	})
}

// TouchStaff records that a staff user used the console now.
func (d *DB) TouchStaff(ctx context.Context, user string) error {
	_, err := d.Pool.Exec(ctx, `INSERT INTO staff_seen(user_id, at) VALUES($1, now())
		ON CONFLICT (user_id) DO UPDATE SET at=now()`, user)
	return err
}

// StaffSeen maps user id to their last console activity.
func (d *DB) StaffSeen(ctx context.Context) (map[string]time.Time, error) {
	rows, err := d.Pool.Query(ctx, `SELECT user_id, at FROM staff_seen`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	m := map[string]time.Time{}
	for rows.Next() {
		var u string
		var t time.Time
		if err := rows.Scan(&u, &t); err != nil {
			return nil, err
		}
		m[u] = t
	}
	return m, rows.Err()
}
