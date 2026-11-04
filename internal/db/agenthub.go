package db

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// AgentHubConnection contains installation-specific review endpoint material.
// TokenCiphertext must only hold ciphertext produced by the installation sealer.
type AgentHubConnection struct {
	BaseURL         string
	TokenCiphertext []byte
	HasCredential   bool
}

func (d *DB) AgentHubConnection(ctx context.Context) (AgentHubConnection, error) {
	var c AgentHubConnection
	err := d.Pool.QueryRow(ctx, `SELECT base_url, token_ciphertext FROM shippin_agenthub_connection WHERE id=1`).Scan(&c.BaseURL, &c.TokenCiphertext)
	if errors.Is(err, pgx.ErrNoRows) {
		return c, ErrNotFound
	}
	if err != nil {
		return c, err
	}
	c.HasCredential = len(c.TokenCiphertext) > 0
	return c, nil
}

// SaveAgentHubConnection updates endpoint and encrypted token with its audit row atomically.
func (d *DB) SaveAgentHubConnection(ctx context.Context, baseURL string, tokenCiphertext []byte, keepCredential bool, by string, es ...AuditEntry) error {
	return d.inTx(ctx, es, func(tx pgx.Tx) (int64, error) {
		var tag pgconn.CommandTag
		var err error
		if keepCredential {
			tag, err = tx.Exec(ctx, `INSERT INTO shippin_agenthub_connection(id,base_url,token_ciphertext,updated_by) VALUES(1,$1,$2,$3)
				ON CONFLICT (id) DO UPDATE SET base_url=$1,updated_by=$3,updated_at=now()`, baseURL, tokenCiphertext, by)
		} else {
			tag, err = tx.Exec(ctx, `INSERT INTO shippin_agenthub_connection(id,base_url,token_ciphertext,updated_by) VALUES(1,$1,$2,$3)
				ON CONFLICT (id) DO UPDATE SET base_url=$1,token_ciphertext=$2,updated_by=$3,updated_at=now()`, baseURL, tokenCiphertext, by)
		}
		return tag.RowsAffected(), err
	})
}
