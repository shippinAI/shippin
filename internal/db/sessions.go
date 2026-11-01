package db

import (
	"context"
	"time"
)

func (d *DB) PutSession(ctx context.Context, id string, data []byte, exp time.Time) error {
	_, err := d.Pool.Exec(ctx, `INSERT INTO sessions(id, data, expires_at) VALUES($1,$2,$3)
		ON CONFLICT (id) DO UPDATE SET data=excluded.data, expires_at=excluded.expires_at`, id, data, exp)
	return err
}

func (d *DB) GetSession(ctx context.Context, id string) ([]byte, error) {
	var b []byte
	err := d.Pool.QueryRow(ctx, `SELECT data FROM sessions WHERE id=$1 AND expires_at > now()`, id).Scan(&b)
	return b, err
}

func (d *DB) DeleteSession(ctx context.Context, id string) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM sessions WHERE id=$1 OR expires_at < now()`, id)
	return err
}
