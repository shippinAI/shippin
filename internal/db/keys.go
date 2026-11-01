package db

import (
	"context"
	"time"
)

type SSHKey struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	PublicKey   string    `json:"public_key"`
	Fingerprint string    `json:"fingerprint"`
	CreatedAt   time.Time `json:"created_at"`
}

func (d *DB) SSHKeys(ctx context.Context, sub string) ([]SSHKey, error) {
	rows, err := d.Pool.Query(ctx, `SELECT id::text, name, public_key, fingerprint, created_at FROM user_ssh_keys
		WHERE user_sub=$1 ORDER BY created_at`, sub)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []SSHKey{}
	for rows.Next() {
		var k SSHKey
		if err := rows.Scan(&k.ID, &k.Name, &k.PublicKey, &k.Fingerprint, &k.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, k)
	}
	return out, nil
}

func (d *DB) AddSSHKey(ctx context.Context, sub string, k SSHKey) error {
	_, err := d.Pool.Exec(ctx, `INSERT INTO user_ssh_keys(id, user_sub, name, public_key, fingerprint) VALUES($1,$2,$3,$4,$5)`,
		k.ID, sub, k.Name, k.PublicKey, k.Fingerprint)
	return err
}

func (d *DB) DeleteSSHKey(ctx context.Context, sub, id string) (bool, error) {
	tag, err := d.Pool.Exec(ctx, `DELETE FROM user_ssh_keys WHERE user_sub=$1 AND id=$2`, sub, id)
	return tag.RowsAffected() > 0, err
}

// WorkspacesByCreator lists live workspaces created by a user (to push key changes).
func (d *DB) WorkspacesByCreator(ctx context.Context, sub string) ([]Workspace, error) {
	return d.queryWS(ctx, `WHERE created_by=$1 AND state<>'deleted'`, sub)
}
