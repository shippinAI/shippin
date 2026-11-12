package db

import (
	"context"
	"time"
)

// PutSession stores a sealed session. userID is its local user, or "" for an identity provider session.
func (d *DB) PutSession(ctx context.Context, id string, data []byte, exp time.Time, userID string) error {
	_, err := d.Pool.Exec(ctx, `INSERT INTO sessions(id, data, expires_at, user_id) VALUES($1,$2,$3,NULLIF($4,''))
		ON CONFLICT (id) DO UPDATE SET data=excluded.data, expires_at=excluded.expires_at, user_id=excluded.user_id`, id, data, exp, userID)
	return err
}

// EndUserSessions deletes every session of a local user except keep, and returns how many ended.
func (d *DB) EndUserSessions(ctx context.Context, userID, keep string) (int64, error) {
	tag, err := d.Pool.Exec(ctx, `DELETE FROM sessions WHERE user_id=$1 AND id<>$2`, userID, keep)
	return tag.RowsAffected(), err
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
