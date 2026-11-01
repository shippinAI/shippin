package db

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

// IdemRecord is a stored Idempotency-Key response (Status 0 = the first request is still running).
type IdemRecord struct {
	Hash    string
	Status  int
	Body    []byte
	Headers map[string]string
}

// ClaimIdemKey reserves (scope, key) for a new request. It returns nil when reserved, or the record
// already there (younger than ttl; an older one is replaced).
func (d *DB) ClaimIdemKey(ctx context.Context, scope, key, hash string, ttl time.Duration) (*IdemRecord, error) {
	tag, err := d.Pool.Exec(ctx, `INSERT INTO idempotency_keys(scope, key, request_hash) VALUES ($1,$2,$3)
	  ON CONFLICT (scope, key) DO UPDATE SET request_hash=EXCLUDED.request_hash, status=0, body=NULL,
	    headers='{}', created_at=now()
	  WHERE idempotency_keys.created_at < now()-make_interval(secs => $4)`, scope, key, hash, ttl.Seconds())
	if err != nil || tag.RowsAffected() == 1 {
		return nil, err
	}
	rec := &IdemRecord{}
	var headers []byte
	err = d.Pool.QueryRow(ctx, `SELECT request_hash, status, coalesce(body,''::bytea), headers
	  FROM idempotency_keys WHERE scope=$1 AND key=$2`, scope, key).Scan(&rec.Hash, &rec.Status, &rec.Body, &headers)
	if errors.Is(err, pgx.ErrNoRows) { // dropped between the two statements: the caller may retry
		return &IdemRecord{Hash: hash}, nil
	}
	if err != nil {
		return nil, err
	}
	return rec, json.Unmarshal(headers, &rec.Headers)
}

// SaveIdemKey stores the response for a reserved key.
func (d *DB) SaveIdemKey(ctx context.Context, scope, key string, status int, body []byte, headers map[string]string) error {
	raw, err := json.Marshal(headers)
	if err != nil {
		return err
	}
	_, err = d.Pool.Exec(ctx, `UPDATE idempotency_keys SET status=$3, body=$4, headers=$5 WHERE scope=$1 AND key=$2`,
		scope, key, status, body, raw)
	return err
}

// DropIdemKey releases a reserved key (the request failed in a way worth retrying).
func (d *DB) DropIdemKey(ctx context.Context, scope, key string) error {
	_, err := d.Pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE scope=$1 AND key=$2 AND status=0`, scope, key)
	return err
}

// PruneIdemKeys deletes keys older than ttl.
func (d *DB) PruneIdemKeys(ctx context.Context, ttl time.Duration) (int64, error) {
	tag, err := d.Pool.Exec(ctx, `DELETE FROM idempotency_keys WHERE created_at < now()-make_interval(secs => $1)`, ttl.Seconds())
	return tag.RowsAffected(), err
}
